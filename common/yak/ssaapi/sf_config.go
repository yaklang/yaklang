package ssaapi

import (
	"strings"
	"sync"
	"sync/atomic"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

type sfCheck struct {
	contextResult *sf.SFFrameResult
	config        *sf.Config
	vm            *sf.SyntaxFlowVirtualMachine

	matchItem []*checkItem
	untilItem []*checkItem

	// compiledFrames memoizes sub-rule text -> compiled frame within this
	// sfCheck (see compiledFrame). dataflow include=/exclude= sub-rules are
	// recompiled once per sfCheck without this; the same text appears across
	// every rule in a scan, so this avoids the regexp-compile allocation that
	// showed up at ~10GB on large projects.
	compiledFrames map[string]*sf.SFFrame

	// beforeMergeHook runs on a child SFFrameResult before MergeByResult (e.g. dataflow
	// only_reachable post-mode pruning include-rule captures). parent is the accumulated
	// context result before this child merge.
	beforeMergeHook func(parent *sf.SFFrameResult, child *sf.SFFrameResult)

	// originalSnapshot captures the contextResult's named-symbol state at
	// CreateCheck time (before any path/query runs). clearup uses it to decide
	// whether a child result produced NEW named output (merge) or only re-
	// contains inherited parent vars + magic vars (skip — the Opt A 463GB
	// saving). Snapshotting once (not per-path) is what makes the across-path
	// case correct: a key merged by path 1 is NOT in this snapshot, so path 2's
	// re-bind is correctly seen as new and merged. See sfvm.SymbolSnapshot.
	originalSnapshot *sf.SymbolSnapshot
}

type checkItem struct {
	*sf.RecursiveConfigItem
	frame *sf.SFFrame
	// matchCache memoizes per-node until/hook match results (keyed by SSA
	// node ID) so the same node is not re-queried via a full SF sub-query
	// when it appears in multiple descent paths. CheckUntil is the hot
	// path: 156 sources each with deep descents call CheckUntil per node,
	// and the same node recurs across sources. Without this cache every
	// node visit ran a full QuerySyntaxflow, consuming 24.79% of CPU.
	matchCache sync.Map
}

// appendSubRuleFromNativeParam adds one include/exclude/hook/until sub-rule from a native-call param if non-empty.
func appendSubRuleFromNativeParam(check *sfCheck, params *sf.NativeCallActualParams, paramKey string, cfgKey sf.RecursiveConfigKey) {
	if check == nil || params == nil {
		return
	}
	if rule := params.GetString(paramKey); rule != "" {
		check.AppendItems(&sf.RecursiveConfigItem{
			Key:            string(cfgKey),
			Value:          rule,
			SyntaxFlowRule: true,
		})
	}
}

func recursiveCheckKeyLabel(key string) string {
	switch key {
	case sf.RecursiveConfig_Include, sf.RecursiveConfig_Exclude, sf.RecursiveConfig_Until, sf.RecursiveConfig_Hook:
		return string(key)
	default:
		return "unknown"
	}
}

func CreateCheck(
	contextResult *sf.SFFrameResult,
	config *sf.Config,
	configItems ...*sf.RecursiveConfigItem,
) *sfCheck {
	res := &sfCheck{
		contextResult: contextResult,
		config:        config,
		vm:            sf.NewSyntaxFlowVirtualMachine(),
		matchItem:     make([]*checkItem, 0, len(configItems)),
	}
	contextResult.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	contextResult.SymbolTable.Delete(sf.RecursiveMagicVariable)
	// Snapshot the parent's named-symbol state ONCE here (before any path/query
	// runs) so clearup can detect child NEW named output without re-merging
	// inherited parent vars. Taken under the config mutex (contextResult is
	// mutated under it during merges).
	config.Mutex.Lock()
	res.originalSnapshot = sf.TakeSymbolSnapshot(contextResult.SymbolTable)
	config.Mutex.Unlock()
	res.vm.SetConfig(config)
	res.AppendItems(configItems...)
	return res
}

func (c *sfCheck) Empty() bool {
	return len(c.matchItem) == 0 && len(c.untilItem) == 0
}

func (c *sfCheck) SetBeforeMergeHook(f func(parent *sf.SFFrameResult, child *sf.SFFrameResult)) {
	c.beforeMergeHook = f
}

func (c *sfCheck) AppendItems(items ...*sf.RecursiveConfigItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		frame, err := c.compiledFrame(item.Value)
		if err != nil {
			keyName := recursiveCheckKeyLabel(item.Key)
			// 暴露编译错误，添加到结果中以便前端可以获取
			errorMsg := utils.Errorf("SyntaxFlow compile error for %s rule [%s]: %v", keyName, item.Value, err).Error()
			log.Errorf(errorMsg)
			if c.contextResult != nil {
				c.contextResult.Errors = append(c.contextResult.Errors, errorMsg)
			}
			continue
		}
		checkItem := &checkItem{
			RecursiveConfigItem: item,
			frame:               frame,
		}
		switch item.Key {
		case sf.RecursiveConfig_Include, sf.RecursiveConfig_Exclude:
			c.matchItem = append(c.matchItem, checkItem)
		case sf.RecursiveConfig_Until, sf.RecursiveConfig_Hook:
			c.untilItem = append(c.untilItem, checkItem)
		}
	}
}

// compiledFrame returns a compiled *sf.SFFrame for the given sub-rule text,
// memoized within this sfCheck. dataflow(include=/exclude=) sub-rules are
// appended once per native-call sfCheck but the SAME text (e.g.
// `* & $params as $__next__`) is recompiled across every rule in a scan —
// driving regexp compile to ~10GB on large projects. Memoizing per-sfCheck
// (a) dedupes when the same text is appended more than once, and
// (b) bounds the VM's `s.frames` slice (vm.go Compile appends without limit).
// Lifetime is the sfCheck's (one dataflow native call), so no unbounded
// growth and no cross-rule contention.
func (c *sfCheck) compiledFrame(text string) (*sf.SFFrame, error) {
	if c.compiledFrames != nil {
		if f, ok := c.compiledFrames[text]; ok {
			return f, nil
		}
	}
	frame, err := c.vm.Compile(text)
	if err != nil {
		return nil, err
	}
	if c.compiledFrames == nil {
		c.compiledFrames = make(map[string]*sf.SFFrame, 4)
	}
	c.compiledFrames[text] = frame
	return frame, nil
}

func CreateCheckFromNativeCallParam(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	params *sf.NativeCallActualParams,
) *sfCheck {

	check := CreateCheck(sfResult, config)
	// Order preserved: exclude / include (path match) before hook / until (walk).
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamExclude, sf.RecursiveConfig_Exclude)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamInclude, sf.RecursiveConfig_Include)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamHook, sf.RecursiveConfig_Hook)
	appendSubRuleFromNativeParam(check, params, NativeCall_DataflowParamUntil, sf.RecursiveConfig_Until)

	return check
}

func (r *sfCheck) CheckMatch(path sf.Values) bool {
	if r.Empty() {
		return true
	}
	ret := true
	r.check(path, r.matchItem, func(key string, match bool) bool {
		switch key {
		case sf.RecursiveConfig_Include:
			if !match {
				// RecursiveConfig_Include: If match, continue; if not, stop.
				ret = false
				return false // stop
			}
		case sf.RecursiveConfig_Exclude:
			if match {
				// RecursiveConfig_Exclude: If match, stop; if not, continue.
				ret = false
				return false // stop
			}
		}
		return true // continue
	})
	return ret
}
func (r *sfCheck) CheckUntil(path sf.Values) bool {
	if r.Empty() {
		return false
	}
	until := false
	r.check(path, r.untilItem, func(key string, match bool) bool {
		switch key {
		case sf.RecursiveConfig_Until:
			if match {
				// RecursiveConfig_Until: If match, stop; if not, continue.
				until = true
				return false // stop
			}
		case sf.RecursiveConfig_Hook:
			// RecursiveConfig_Hook: Always continue.
		}
		return true // continue
	})
	return until
}
func (r *sfCheck) check(
	path sf.Values,
	items []*checkItem,
	fn func(string, bool) bool,
) {
	opt := []QueryOption{
		QueryWithVM(r.vm),
		QueryWithInitVar(r.contextResult.SymbolTable),
		QueryWithValues(path),
	}
	// Propagate the per-rule total-work budget into the sub-query so dataflow
	// include/exclude/hook/until sub-rules share the parent's fanout budget
	// (cumulative across the whole rule, not just the top-level opcodes). The
	// budget is on r.config (the frame's sfvm.Config); nil when no budget is
	// set (QueryWithWorkBudget is a no-op then).
	if budget := r.config.GetWorkBudget(); budget != nil {
		opt = append(opt, QueryWithWorkBudget(budget))
	}
	// The parent named-symbol state was snapshotted once at CreateCheck
	// (r.originalSnapshot); clearup uses it to detect the child's NEW named
	// output. No per-path re-snapshot — re-snapshotting per path is what made
	// Opt A over-skip (a key merged by an earlier path looked "inherited" to a
	// later path, so the later path's re-bind of it was skipped).
	nodeId := extractCheckNodeId(path)
	for _, it := range items {
		if nodeId > 0 {
			if cached, ok := it.matchCache.Load(nodeId); ok {
				if !fn(it.Key, cached.(bool)) {
					return
				}
				continue
			}
		}
		res, err := it.check(path, opt...)
		if err != nil {
			log.Errorf("check path value %v fail: %v", path.String(), err)
			continue
		}

		match := isMatch(res)
		r.clearup(res.GetSFResult())
		if nodeId > 0 {
			it.matchCache.Store(nodeId, match)
		}
		if !fn(it.Key, match) {
			return
		}
	}
}

// extractCheckNodeId gets a stable SSA node ID from the path values for
// cache keying. Uses the first value's GetId() which is the SSA instruction
// ID -- stable across different descent paths within the same program.
func extractCheckNodeId(path sf.Values) int64 {
	var id int64
	path.Recursive(func(op sf.ValueOperator) error {
		if v, ok := op.(interface{ GetId() int64 }); ok {
			if i := v.GetId(); i > 0 {
				id = i
				return utils.Error("done")
			}
		}
		return nil
	})
	return id
}

func (item *checkItem) check(value sf.Values, opt ...QueryOption) (*SyntaxFlowResult, error) {
	if item.frame == nil {
		return nil, utils.Errorf("syntaxflow frame is nil")
	}

	var res *SyntaxFlowResult
	var err error
	opt = append(opt, QueryWithFrame(item.frame))
	res, err = QuerySyntaxflow(opt...)
	if err != nil {
		return nil, utils.Errorf("syntaxflow rule exec fail: %v", err)
	}
	return res, nil
}

func (r *sfCheck) clearup(sfres *sf.SFFrameResult) {
	if sfres == nil {
		return
	}
	r.sanitizeChildResult(sfres)
	r.runBeforeMergeHook(sfres)
	// CheckMatch/CheckUntil only consume isMatch(res) (a bool); the merge into
	// the shared contextResult is a side effect. Child queries inherit the parent
	// SymbolTable (QueryWithInitVar), so the child result re-contains every
	// parent var; merging re-runs MergeValues on those inherited keys once per
	// path × per source — the #1 alloc driver on large projects (MergeValues
	// ~463GB / 27% of alloc_space).
	//
	// Skip the symbol/alert merge when the child produced no NEW named output:
	// no new named key, AND no new value (by dedupKey) for an existing key.
	// r.originalSnapshot was taken ONCE at CreateCheck (before any path), so a
	// key/value merged by an earlier path is NOT in the snapshot — a later path's
	// re-bind of it is correctly seen as NEW and merged (this is the Opt A
	// over-skip fix; the per-path re-snapshot treated earlier paths' merges as
	// "inherited" and skipped later paths' re-binds, losing them). A child that
	// only re-contains inherited parent vars (unchanged) + magic ($__) vars has
	// no new named output → skip (the 463GB saving). Errors/CheckParams always
	// propagate.
	r.config.Mutex.Lock()
	mergeable := r.originalSnapshot.HasNewNamedValue(sfres)
	if mergeable {
		clearupMergeSkipCounter.addMerge()
		r.contextResult.MergeByResultLocked(sfres)
	} else {
		clearupMergeSkipCounter.addSkip()
		r.contextResult.MergeByResultMetaLocked(sfres)
	}
	r.config.Mutex.Unlock()
}

// clearupMergeSkipCounter is a test-only counter for how many clearup calls
// skipped vs performed the symbol merge. Production code never reads it; the
// atomic ops are the only cost (one per clearup, negligible vs the merge work).
// Tests read it to assert Opt A actually skips the useless merges deterministically
// (alloc-profile-based assertions are too noisy on small synthetic inputs).
var clearupMergeSkipCounter mergeSkipCounter

type mergeSkipCounter struct {
	skip  int64
	merge int64
}

func (m *mergeSkipCounter) addSkip()  { atomic.AddInt64(&m.skip, 1) }
func (m *mergeSkipCounter) addMerge() { atomic.AddInt64(&m.merge, 1) }

// ResetClearupMergeCounters zeros the test-only counters. Returns previous values.
func ResetClearupMergeCounters() (skip, merge int64) {
	prevSkip := atomic.SwapInt64(&clearupMergeSkipCounter.skip, 0)
	prevMerge := atomic.SwapInt64(&clearupMergeSkipCounter.merge, 0)
	return prevSkip, prevMerge
}

// ClearupMergeCounters returns the current test-only (skip, merge) counts.
func ClearupMergeCounters() (skip, merge int64) {
	return atomic.LoadInt64(&clearupMergeSkipCounter.skip), atomic.LoadInt64(&clearupMergeSkipCounter.merge)
}

func (r *sfCheck) sanitizeChildResult(sfres *sf.SFFrameResult) {
	r.config.Mutex.Lock()
	sfres.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	sfres.SymbolTable.Delete(sf.RecursiveMagicVariable)
	r.config.Mutex.Unlock()
}

func (r *sfCheck) runBeforeMergeHook(sfres *sf.SFFrameResult) {
	if r.beforeMergeHook == nil {
		return
	}
	// Run hook outside config lock:
	// hook logic may trigger CFG/reachability helpers that re-enter VM/config paths and
	// attempt to lock the same mutex again. Holding the lock here can deadlock (non-reentrant mutex).
	r.beforeMergeHook(r.contextResult, sfres)
}

func (r *sfCheck) mergeChildResult(sfres *sf.SFFrameResult) {
	r.config.Mutex.Lock()
	r.contextResult.MergeByResultLocked(sfres)
	r.config.Mutex.Unlock()
}

func isMatch(result *SyntaxFlowResult) bool {
	if result == nil {
		return false
	}

	effectiveVarNum := 0
	matchedSingle := false
	if vars := result.GetAllVariable(); vars != nil {
		vars.ForEach(func(key string, value any) {
			// Ignore VM internal temporary symbols when deciding include/exclude/until match.
			if strings.HasPrefix(key, "__") && key != sf.RecursiveMagicVariable {
				return
			}
			effectiveVarNum++
			if num, ok := value.(int); ok && num != 0 {
				matchedSingle = true
			}
		})
	}

	if effectiveVarNum == 0 {
		// check un-name value
		if len(result.GetUnNameValues()) != 0 {
			return true
		}
	} else if effectiveVarNum == 1 {
		return matchedSingle
	} else {
		// multiple variable, check magic variable
		if len(result.GetValues(sf.RecursiveMagicVariable)) != 0 {
			return true
		}
	}
	return false
}
