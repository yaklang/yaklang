package ssaapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

// dataflowFilterKind identifies include/exclude sub-rules that are provably
// equivalent to a membership test on one already-bound symbol, so the dataflow
// fast path can skip compiling the sub-rule and evaluate it from an id set.
type dataflowFilterKind uint8

const (
	dataflowFilterUnknown dataflowFilterKind = iota
	// dataflowFilterAny is `* & $sym`: the path contains a value bound to $sym.
	// Repeated `& $sym` items are idempotent and fold into this shape.
	dataflowFilterAny
	// dataflowFilterSelf is `<self> & $sym`: `<self>` is identity over the
	// input values, so on a dataflow path it carries the same membership
	// meaning as dataflowFilterAny.
	dataflowFilterSelf
	// dataflowFilterCallee is `*<getCallee> & $sym`: a call on the path has a
	// callee bound to $sym. This shape is monotone, which is what makes branch
	// pruning sound: once a path contains such a call, longer paths still do.
	dataflowFilterCallee
)

type dataflowFilterPlan struct {
	kind   dataflowFilterKind
	symbol string
}

var (
	// simpleDataflowInclude matches the subject and first symbol of
	// `* & $sym` / `<self> & $sym`. The tail is validated separately so a
	// trailing `as $alias` or repeated `& $sym` can be accepted.
	simpleDataflowInclude = regexp.MustCompile(`^\s*(?:\*|<self>)\s*&\s*\$([A-Za-z_][A-Za-z0-9_]*)`)
	// simpleGetCalleeExclude is the same for `*<getCallee> & $sym`.
	simpleGetCalleeExclude = regexp.MustCompile(`^\s*\*\s*<getCallee>\s*&\s*\$([A-Za-z_][A-Za-z0-9_]*)`)
	// simpleDataflowFilterAlias is the optional `as $alias` tail. It only names
	// the sub-rule result; include/exclude is evaluated on whether the result is
	// non-empty, so the alias does not change semantics.
	simpleDataflowFilterAlias = regexp.MustCompile(`^as\s+\$[A-Za-z_][A-Za-z0-9_]*$`)
)

// dataflowFilterShapeCache memoizes dataflowFilterShape. Classification runs per
// sfCheck (one per sink) and fastPathMatch runs per enumerated path, so the
// cached lookup keeps the hot path to a single map access. Negative entries are
// cached too, so complex sub-rules are not re-matched on every path.
var dataflowFilterShapeCache sync.Map // string -> dataflowFilterShapeEntry

type dataflowFilterShapeEntry struct {
	plan dataflowFilterPlan
	ok   bool
}

// dataflowFilterShape classifies a dataflow include/exclude sub-rule. The
// accepted shapes are exactly those whose include/exclude predicate is a
// membership test on one bound symbol, so callers may evaluate them from a
// symbol id set and, for the monotone Callee exclude, prune path branches.
func dataflowFilterShape(text string) (dataflowFilterPlan, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return dataflowFilterPlan{}, false
	}
	if cached, ok := dataflowFilterShapeCache.Load(text); ok {
		entry := cached.(dataflowFilterShapeEntry)
		return entry.plan, entry.ok
	}
	plan, ok := parseDataflowFilterShape(text)
	dataflowFilterShapeCache.Store(text, dataflowFilterShapeEntry{plan: plan, ok: ok})
	return plan, ok
}

func parseDataflowFilterShape(text string) (dataflowFilterPlan, bool) {
	if loc := simpleDataflowInclude.FindStringSubmatchIndex(text); loc != nil {
		sym := text[loc[2]:loc[3]]
		kind := dataflowFilterAny
		if strings.HasPrefix(text, "<self>") {
			kind = dataflowFilterSelf
		}
		if matchDataflowFilterTail(text[loc[1]:], sym) {
			return dataflowFilterPlan{kind: kind, symbol: sym}, true
		}
	}
	if loc := simpleGetCalleeExclude.FindStringSubmatchIndex(text); loc != nil {
		sym := text[loc[2]:loc[3]]
		if matchDataflowFilterTail(text[loc[1]:], sym) {
			return dataflowFilterPlan{kind: dataflowFilterCallee, symbol: sym}, true
		}
	}
	return dataflowFilterPlan{}, false
}

// matchDataflowFilterTail accepts only more `& $sym` items for the same symbol
// plus an optional trailing `as $alias`. Anything else keeps the full sub-query.
func matchDataflowFilterTail(tail, sym string) bool {
	for {
		tail = strings.TrimSpace(tail)
		if tail == "" {
			return true
		}
		if simpleDataflowFilterAlias.MatchString(tail) {
			return true
		}
		if !strings.HasPrefix(tail, "&") {
			return false
		}
		tail = strings.TrimSpace(tail[1:])
		if !strings.HasPrefix(tail, "$") {
			return false
		}
		tail = tail[1:]
		end := 0
		for end < len(tail) && isDataflowFilterIdentByte(tail[end]) {
			end++
		}
		if end == 0 || tail[:end] != sym {
			return false
		}
		tail = tail[end:]
	}
}

func isDataflowFilterIdentByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

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

	// originalSnapshot captures the contextResult's named-symbol state at the
	// first AppendItems that adds a real item (before any path/query runs for
	// this check). clearup uses it to decide whether a child result produced NEW
	// named output (merge) or only re-contains inherited parent vars + magic
	// vars (skip — the Opt A 463GB saving). Snapshotting once (not per-path) is
	// what makes the across-path case correct: a key merged by path 1 is NOT in
	// this snapshot, so path 2's re-bind is correctly seen as new and merged.
	//
	// O3: the snapshot is built lazily on the first AppendItems that adds an
	// item, NOT eagerly at CreateCheck. Empty checks (no include/exclude/until/
	// hook sub-rules, e.g. the unconditionally-created untilCheck/hookRunner in
	// DataFlowWithSFConfig) never build a snapshot — they never call check()/
	// clearup, so the eager build was pure waste (43M TakeSymbolSnapshot calls /
	// 42GB on hadoop). Building at first AppendItems is safe because AppendItems
	// always runs before any descent for this check, and the sibling checks
	// (untilCheck/hookRunner) are each given their items before the descent
	// starts, so each captures its own pre-descent parent state.
	originalSnapshot     *sf.SymbolSnapshot
	originalSnapshotOnce sync.Once

	// fastMatchMu guards fastMatchIDs, which memoizes symbol -> id set for the
	// simple `* & $source` / `<self> & $source` fast path.
	fastMatchMu  sync.Mutex
	fastMatchIDs map[string]map[int64]struct{}

	// excludeItems is the exclude-only subset of matchItem. excludePrunable is
	// true only when every exclude uses the monotone `*<getCallee> & $symbol`
	// shape; then a call node whose callee is in that symbol's value set rejects
	// every path through it, so the DFS can drop the branch.
	excludeItems      []*checkItem
	excludePrunable   bool
	excludePruneOnce  sync.Once
	excludePruneIDs   map[int64]struct{}
	excludePruneReady bool

	// needSymbolSnapshot is set when at least one item must run the full
	// sub-query/clearup path. Fast-path-only checks skip TakeSymbolSnapshot,
	// which otherwise copies the (large) $source/$filter symbol table once per
	// dataflow filter on every sink.
	needSymbolSnapshot bool
}

type checkItem struct {
	*sf.RecursiveConfigItem
	frame *sf.SFFrame
	// fastPathOnly marks a simple include/exclude that fastPathMatch can always
	// evaluate from symbol ids. Such an item needs no compiled frame and never
	// runs the sub-query/clearup path, so it also needs no symbol snapshot. On a
	// large project the same two dataflow filters are otherwise recompiled for
	// every sink (tens of thousands of times).
	fastPathOnly bool
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
		contextResult:   contextResult,
		config:          config,
		vm:              sf.NewSyntaxFlowVirtualMachine(),
		matchItem:       make([]*checkItem, 0, len(configItems)),
		excludePrunable: true,
	}
	contextResult.AlertSymbolTable.Delete(sf.RecursiveMagicVariable)
	contextResult.SymbolTable.Delete(sf.RecursiveMagicVariable)
	// Snapshot is built lazily on the first AppendItems that adds a real item
	// (see originalSnapshot field comment); it must reflect the parent state
	// before this check's own descent, which AppendItems precedes.
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

// ensureOriginalSnapshot builds the parent-symbol snapshot once, on the first
// call that actually adds a sub-rule item. It runs under config.Mutex so the
// snapshot reflects a consistent parent state. Empty checks that never get a
// real item never build a snapshot (O3: 43M wasted TakeSymbolSnapshot calls on
// hadoop came from eagerly snapshoting every CreateCheck, including empty ones).
func (c *sfCheck) ensureOriginalSnapshot() {
	if c == nil || c.contextResult == nil || c.config == nil {
		return
	}
	c.originalSnapshotOnce.Do(func() {
		c.config.Mutex.Lock()
		defer c.config.Mutex.Unlock()
		c.originalSnapshot = sf.TakeSymbolSnapshot(c.contextResult.SymbolTable)
	})
}

func (c *sfCheck) AppendItems(items ...*sf.RecursiveConfigItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		fastPathOnly := c.isSimpleFastPathItem(item)
		var frame *sf.SFFrame
		if !fastPathOnly {
			var err error
			frame, err = c.compiledFrame(item.Value)
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
		}
		checkItem := &checkItem{
			RecursiveConfigItem: item,
			frame:               frame,
			fastPathOnly:        fastPathOnly,
		}
		switch item.Key {
		case sf.RecursiveConfig_Include, sf.RecursiveConfig_Exclude:
			c.matchItem = append(c.matchItem, checkItem)
			if item.Key == sf.RecursiveConfig_Exclude {
				c.excludeItems = append(c.excludeItems, checkItem)
				if shape, ok := dataflowFilterShape(item.Value); !ok || shape.kind != dataflowFilterCallee {
					c.excludePrunable = false
				}
			}
		case sf.RecursiveConfig_Until, sf.RecursiveConfig_Hook:
			c.untilItem = append(c.untilItem, checkItem)
		}
		// A non-fast item can run check()/clearup(), which needs the parent
		// snapshot. Fast-path-only items never do, so skip the (large) copy.
		if !fastPathOnly {
			c.needSymbolSnapshot = true
			c.ensureOriginalSnapshot()
		}
	}
}

// isSimpleFastPathItem reports whether Appending this item can skip compilation
// entirely: the text is one of the simple include/exclude forms fastPathMatch
// evaluates from a bound symbol id set. The symbol must already be bound;
// otherwise the full query stays authoritative.
func (c *sfCheck) isSimpleFastPathItem(item *sf.RecursiveConfigItem) bool {
	if c == nil || item == nil {
		return false
	}
	shape, ok := dataflowFilterShape(item.Value)
	if !ok {
		return false
	}
	switch item.Key {
	case sf.RecursiveConfig_Include:
		if shape.kind != dataflowFilterAny && shape.kind != dataflowFilterSelf {
			return false
		}
	case sf.RecursiveConfig_Exclude:
		// Only the monotone `*<getCallee> & $sym` exclude is safe to prune on;
		// an include-style exclude stays on the full query.
		if shape.kind != dataflowFilterCallee {
			return false
		}
	default:
		return false
	}
	return len(c.fastMatchSymbolIDs(shape.symbol)) > 0
}

// compiledFrame returns a compiled *sf.SFFrame for the given sub-rule text,
// memoized within this sfCheck. dataflow(include=/exclude=) sub-rules are
// appended once per native-call sfCheck but the SAME text (e.g.
// `*?{opcode: call} as $__next__`) is recompiled across every rule in a scan —
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
	cacheKey := extractCheckNodeId(path)
	for _, it := range items {
		if match, ok := r.fastPathMatch(it, path); ok {
			if !fn(it.Key, match) {
				return
			}
			continue
		}
		if it.frame == nil {
			// Fast-path-only item whose symbol disappeared after AppendItems.
			// There is no compiled sub-rule to fall back to; an unbound symbol
			// cannot match, so treat it as a non-match.
			if !fn(it.Key, false) {
				return
			}
			continue
		}
		if cacheKey != "" {
			if cached, ok := it.matchCache.Load(cacheKey); ok {
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
		if cacheKey != "" {
			it.matchCache.Store(cacheKey, match)
		}
		if !fn(it.Key, match) {
			return
		}
	}
}

// fastPathMatch short-circuits the common dataflow include/exclude sub-rules
// (`* & $source`, `<self> & $source`, `*<getCallee> & $filter` and their
// `as $alias` forms) by checking path-node ids against the already-bound symbol
// set. This avoids a full SyntaxFlow sub-query per enumerated path, which
// dominated the heavy log-forging / path-traversal rules. Returns ok=false for
// anything it cannot prove equivalent, so semantics always fall back to the
// full query.
func (r *sfCheck) fastPathMatch(item *checkItem, path sf.Values) (bool, bool) {
	if r == nil || item == nil || item.Value == "" {
		atomic.AddInt64(&fastPathMatchCounter.fallback, 1)
		return false, false
	}
	text := strings.TrimSpace(item.Value)

	if shape, ok := dataflowFilterShape(text); ok {
		if ids := r.fastMatchSymbolIDs(shape.symbol); len(ids) > 0 {
			switch shape.kind {
			case dataflowFilterAny, dataflowFilterSelf:
				// `* & $symbol` / `<self> & $symbol`: path intersects the
				// symbol's ids.
				atomic.AddInt64(&fastPathMatchCounter.hit, 1)
				return pathContainsAnyID(path, ids), true
			case dataflowFilterCallee:
				// `*<getCallee> & $symbol`: a call on the path has a callee in
				// the symbol.
				atomic.AddInt64(&fastPathMatchCounter.hit, 1)
				return pathCalleeMatchesAnyID(path, ids), true
			}
		}
	}

	atomic.AddInt64(&fastPathMatchCounter.fallback, 1)
	if os.Getenv("YAK_SSA_FAST_MATCH_DEBUG") != "" && atomic.AddInt64(&fastPathFallbackLogCount, 1) <= 20 {
		log.Infof("fast-path fallback text: %q", text)
	}
	return false, false
}

func pathContainsAnyID(path sf.Values, ids map[int64]struct{}) bool {
	matched := false
	path.Recursive(func(op sf.ValueOperator) error {
		if id, ok := op.(interface{ GetId() int64 }); ok {
			if _, ok := ids[id.GetId()]; ok {
				matched = true
				return utils.Error("abort")
			}
		}
		return nil
	})
	return matched
}

func pathCalleeMatchesAnyID(path sf.Values, ids map[int64]struct{}) bool {
	matched := false
	path.Recursive(func(op sf.ValueOperator) error {
		value, ok := op.(*Value)
		if !ok || !value.IsCall() {
			return nil
		}
		callee := value.GetCallee()
		if callee == nil {
			return nil
		}
		if _, ok := ids[callee.GetId()]; ok {
			matched = true
			return utils.Error("abort")
		}
		return nil
	})
	return matched
}

// ExcludePrunable reports whether every exclude sub-rule is the monotone
// `*<getCallee> & $symbol` shape, so a single call node can reject a branch.
func (r *sfCheck) ExcludePrunable() bool {
	return r != nil && r.excludePrunable && len(r.excludeItems) > 0
}

// CheckExcludeNode reports whether node alone is enough to reject every path
// containing it: the node is a call whose callee id is bound to an exclude
// filter symbol. Because the exclude pattern is monotone, extending the path
// cannot make it stop matching, so the caller may prune the entire subtree.
func (r *sfCheck) CheckExcludeNode(node *Value) bool {
	if r == nil || !r.ExcludePrunable() || node == nil || !node.IsCall() {
		return false
	}
	if !r.prepareExcludePruneIDs() {
		return false
	}
	callee := node.GetCallee()
	if callee == nil {
		return false
	}
	_, ok := r.excludePruneIDs[callee.GetId()]
	return ok
}

// SimpleIncludeSourceIDs returns the union of the bound value ids referenced
// by every `* & $symbol` include item. ok=false when there is no include, any
// include has a shape we cannot prove (combinations, other patterns), or a
// referenced symbol is unbound/empty. The reachability prefilter is only an
// optimization; the full path check remains authoritative.
func (r *sfCheck) SimpleIncludeSourceIDs() (map[int64]struct{}, bool) {
	if r == nil {
		return nil, false
	}
	ids := make(map[int64]struct{})
	found := false
	for _, item := range r.matchItem {
		if item == nil || item.Key != string(sf.RecursiveConfig_Include) {
			continue
		}
		found = true
		shape, ok := dataflowFilterShape(item.Value)
		if !ok || (shape.kind != dataflowFilterAny && shape.kind != dataflowFilterSelf) {
			return nil, false
		}
		symbolIDs := r.fastMatchSymbolIDs(shape.symbol)
		if len(symbolIDs) == 0 {
			return nil, false
		}
		for id := range symbolIDs {
			ids[id] = struct{}{}
		}
	}
	if !found || len(ids) == 0 {
		return nil, false
	}
	return ids, true
}

// prepareExcludePruneIDs resolves the exclude filter symbol(s) once. Any
// unresolved/empty symbol leaves pruning disabled (the full query is still
// authoritative; this is only an optional branch cut).
func (r *sfCheck) prepareExcludePruneIDs() bool {
	r.excludePruneOnce.Do(func() {
		ids := make(map[int64]struct{})
		for _, item := range r.excludeItems {
			shape, ok := dataflowFilterShape(item.Value)
			if !ok || shape.kind != dataflowFilterCallee {
				r.excludePrunable = false
				return
			}
			for id := range r.fastMatchSymbolIDs(shape.symbol) {
				ids[id] = struct{}{}
			}
		}
		if len(ids) == 0 {
			return
		}
		r.excludePruneIDs = ids
		r.excludePruneReady = true
	})
	return r.excludePruneReady
}

func (r *sfCheck) fastMatchSymbolIDs(name string) map[int64]struct{} {
	if r == nil || r.contextResult == nil || name == "" {
		return nil
	}
	r.fastMatchMu.Lock()
	defer r.fastMatchMu.Unlock()
	if r.fastMatchIDs == nil {
		r.fastMatchIDs = make(map[string]map[int64]struct{})
	}
	if set, ok := r.fastMatchIDs[name]; ok {
		return set
	}
	vals, ok := r.contextResult.SymbolTable.Get(name)
	if !ok {
		r.fastMatchIDs[name] = nil
		return nil
	}
	if cached, ok := r.contextResult.GetSymbolIDSet(name, len(vals)); ok {
		r.fastMatchIDs[name] = cached
		return cached
	}
	set := make(map[int64]struct{}, len(vals))
	for _, op := range vals {
		if id, ok := op.(interface{ GetId() int64 }); ok {
			if id.GetId() > 0 {
				set[id.GetId()] = struct{}{}
			}
		}
	}
	r.fastMatchIDs[name] = set
	// Share the set across the per-sink sfChecks of this rule execution; the
	// symbol values are append-merged, so the count is a safe invalidation key.
	r.contextResult.SetSymbolIDSet(name, len(vals), set)
	return set
}

// extractCheckNodeId builds a cache key from the path values.
// The path is a sequence of SSA nodes (e.g. A -> B -> C -> D -> E); using
// only the first node's ID as the cache key is incorrect because two
// different paths sharing the same first node but diverging later would
// collide and return stale results. Instead, we collect the IDs of ALL
// nodes in the path and join them into a string key, so the cache is
// keyed by the full path identity.
func extractCheckNodeId(path sf.Values) string {
	var sb strings.Builder
	path.Recursive(func(op sf.ValueOperator) error {
		if v, ok := op.(interface{ GetId() int64 }); ok {
			if i := v.GetId(); i > 0 {
				if sb.Len() > 0 {
					sb.WriteByte(',')
				}
				sb.Write(strconv.AppendInt(nil, i, 10))
			}
		}
		return nil
	})
	return sb.String()
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

// fastPathMatchCounter counts fast-path hits vs fallbacks so tests and the
// scan summary can prove the simple include/exclude short-circuit is active.
type fastPathMatchStats struct {
	hit      int64
	fallback int64
}

var fastPathMatchCounter fastPathMatchStats
var fastPathFallbackLogCount int64

// FastPathMatchStats returns the process-wide fast-path hit/fallback counts.
// Exposed for tests and optional scan-summary telemetry.
func FastPathMatchStats() (hits, fallbacks int64) {
	return atomic.LoadInt64(&fastPathMatchCounter.hit),
		atomic.LoadInt64(&fastPathMatchCounter.fallback)
}

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
