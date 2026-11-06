package ssaapi

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// dataflowTimeoutEnv is the env var for the per-dataflow-call wall-clock budget
// (seconds). 0 disables. Default 5min. This is a temporary guard so a
// dataflow() rule on a large project (thousands of sources × deep recursion)
// cannot hang the scan; the proper total-work fix is on another branch.
const dataflowTimeoutEnv = "YAK_SSA_DATAFLOW_TIMEOUT"

func loadDataflowTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(dataflowTimeoutEnv))
	if raw == "" {
		return 5 * time.Minute
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 5 * time.Minute
	}
	return time.Duration(n) * time.Second
}

const (
	Predecessors_TopDefLabel    = "dataflow_topdef"
	Predecessors_BottomUseLabel = "dataflow_bottomuse"
)

// cfgCtxFromOnlyReachTargetOp resolves only_reachable="$var" to a *CfgCtxValue. The symbol may hold
// raw *CfgCtxValue from <getCfg>, or *ssaapi.Value (e.g. the sink SSA value) which we map via cfgCtxForValueMemo.
func cfgCtxFromOnlyReachTargetOp(prog *Program, targetOp sfvm.Values) *CfgCtxValue {
	if targetOp == nil || targetOp.IsEmpty() {
		return nil
	}
	cfgCtxWithProgram := func(c *CfgCtxValue) *CfgCtxValue {
		if c == nil || prog == nil || c.prog != nil {
			return c
		}
		if c.FuncID <= 0 || c.BlockID <= 0 {
			return c
		}
		out := *c
		out.prog = prog
		return &out
	}
	var targetCfg *CfgCtxValue
	// Prefer *Value → cfgCtxForValueMemo: the same symbol bag may carry stripped *CfgCtxValue (nil prog);
	// filling prog would make those non-empty and would wrongly beat the SSA sink anchor.
	_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
		if v, ok := operator.(*Value); ok && v != nil && !v.IsNil() {
			p := prog
			if p == nil {
				if pp, err := fetchProgram(sfvm.ValuesOf(v)); err == nil && pp != nil {
					p = pp
				}
			}
			if p != nil {
				if ctx := cfgCtxForValueMemo(p, v, nil); ctx != nil && !ctx.IsEmpty() {
					targetCfg = ctx
					return utils.Error("abort")
				}
			}
		}
		return nil
	})
	if targetCfg != nil {
		return targetCfg
	}
	_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
		if c, ok := operator.(*CfgCtxValue); ok && c != nil {
			c = cfgCtxWithProgram(c)
			if c != nil && !c.IsEmpty() {
				targetCfg = c
				return utils.Error("abort")
			}
		}
		return nil
	})
	return targetCfg
}

// valuePassesCFGReach returns whether the SSA instruction site of v can reach targetCfg
// in the intraprocedural CFG (same semantics as only_reachable post-filter on dataflow roots).
func valuePassesCFGReach(v *Value, prog *Program, targetCfg *CfgCtxValue, opt reachableOptions) bool {
	if v == nil || v.IsEmpty() || prog == nil || targetCfg == nil || targetCfg.IsEmpty() {
		return false
	}
	candCfg := cfgCtxFromExpandedSSAValue(prog, v)
	if candCfg == nil || candCfg.IsEmpty() {
		return false
	}
	// Same condition-inst but different block usually means opposite branches of one control split
	// (e.g. then vs else). Exclude it for only_reachable anchor filtering.
	if candSummary, err := getBlockConditionSummaryByCfgCtx(prog, candCfg); err == nil && candSummary != nil {
		if targetSummary, err2 := getBlockConditionSummaryByCfgCtx(prog, targetCfg); err2 == nil && targetSummary != nil {
			if candSummary.FuncID == targetSummary.FuncID &&
				candSummary.CondInstID > 0 &&
				candSummary.CondInstID == targetSummary.CondInstID &&
				candSummary.BlockID != targetSummary.BlockID {
				return false
			}
		}
	}
	return reachableWithOptions(prog, candCfg, targetCfg, opt)
}

func valueIdentity(v *Value) string {
	if v == nil || v.IsNil() {
		return ""
	}
	if id := v.GetId(); id > 0 {
		return fmt.Sprintf("id:%d", id)
	}
	return "str:" + v.String()
}

// filterSfFrameResultByOnlyReachable drops SymbolTable *ssaapi.Value entries whose CFG site
// cannot reach the only_reachable anchor. Include-rule captures (e.g. `as $x` in dataflow CODE)
// are merged via clearup; without this, post-mode only_reachable would only filter dataflow
// roots, not the captured path values.
// topDefReachConstraint is one CFG anchor clause inside `# { ... } ->` (only_reachable / include_reachable / exclude_reachable).
type topDefReachConstraint struct {
	symbol  string
	exclude bool // exclude_reachable: drop values whose site *can* reach the anchor
}

func normalizeTopDefReachSymbol(raw string) string {
	s := strings.TrimSpace(codec.AnyToString(raw))
	s = strings.TrimPrefix(s, "$")
	return s
}

// parseTopDefReachSymbolFromItem reads the value from a RecursiveConfigItem (plain identifier or “ `$cfg` “ filter text).
func parseTopDefReachSymbolFromItem(item *sf.RecursiveConfigItem) string {
	if item == nil {
		return ""
	}
	s := strings.TrimSpace(item.Value)
	if i := strings.IndexAny(s, ";\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return normalizeTopDefReachSymbol(s)
}

type resolvedTopDefReach struct {
	cfg     *CfgCtxValue
	exclude bool
}

// --- dataflow() native: only_reachable / mode / icfg (aligned with include|exclude param style) ---

func dataflowNativeReachMode(params *sfvm.NativeCallActualParams) string {
	if params == nil {
		return "post"
	}
	mode := strings.ToLower(strings.TrimSpace(params.GetString(
		NativeCall_DataflowParamOnlyReachableMode,
		NativeCall_DataflowParamOnlyReachableModeCamel,
	)))
	if mode != "" {
		return mode
	}
	if parseBoolParam(params, NativeCall_DataflowParamStrict, false) {
		return "path"
	}
	return "post"
}

func dataflowNativeReachOpts(params *sfvm.NativeCallActualParams) reachableOptions {
	icfg := parseBoolParam(params, NativeCall_DataflowParamIcfg, false)
	opt := reachableOptsFromParams(params, icfg)
	opt.skipLoopBackedge = true
	return opt
}

// resolveDataflowOnlyReachTarget resolves a frame symbol (e.g. from <getCfg>) to a CFG anchor for dataflow().
func resolveDataflowOnlyReachTarget(frame *sfvm.SFFrame, prog *Program, symbol string) *CfgCtxValue {
	if frame == nil || prog == nil || symbol == "" {
		return nil
	}
	targetOp, ok := frame.GetSymbolByName(symbol)
	if !ok || targetOp == nil || targetOp.IsEmpty() {
		return nil
	}
	cfg := cfgCtxFromOnlyReachTargetOp(prog, targetOp)
	if cfg == nil || cfg.IsEmpty() {
		return nil
	}
	return cfg
}

// buildOnlyReachableMergeHookForResolved is the merge hook for dataflow(..., only_reachable=..., mode=post).
func buildOnlyReachableMergeHookForResolved(
	inputProg *Program,
	inputProgErr error,
	anchor *CfgCtxValue,
	opt reachableOptions,
) func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
	if inputProgErr != nil || inputProg == nil || anchor == nil || anchor.IsEmpty() {
		return nil
	}
	return func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
		filterSfFrameResultByOnlyReachable(parent, child, inputProg, anchor, opt)
	}
}

func resolveTopDefReachAnchors(sfResult *sf.SFFrameResult, prog *Program, parsed []topDefReachConstraint) []resolvedTopDefReach {
	if sfResult == nil || prog == nil || len(parsed) == 0 {
		return nil
	}
	var out []resolvedTopDefReach
	for _, p := range parsed {
		if p.symbol == "" {
			continue
		}
		vals, ok := sfResult.SymbolTable.Get(p.symbol)
		if !ok || vals == nil || vals.IsEmpty() {
			continue
		}
		cfg := cfgCtxFromOnlyReachTargetOp(prog, vals)
		if cfg == nil || cfg.IsEmpty() {
			continue
		}
		out = append(out, resolvedTopDefReach{cfg: cfg, exclude: p.exclude})
	}
	return out
}

func valueSatisfiesTopDefReachAnchors(v *Value, prog *Program, resolved []resolvedTopDefReach, opt reachableOptions) bool {
	if v == nil || v.IsEmpty() || prog == nil {
		return len(resolved) == 0
	}
	for _, r := range resolved {
		pass := valuePassesCFGReach(v, prog, r.cfg, opt)
		if r.exclude {
			if pass {
				return false
			}
		} else {
			if !pass {
				return false
			}
		}
	}
	return true
}

func filterSfFrameResultByTopDefReachAnchors(
	parent *sf.SFFrameResult, sfres *sf.SFFrameResult, prog *Program, resolved []resolvedTopDefReach, opt reachableOptions,
) {
	if sfres == nil || prog == nil || len(resolved) == 0 {
		return
	}
	var keys []string
	sfres.SymbolTable.ForEach(func(key string, _ sfvm.Values) bool {
		if !strings.HasPrefix(key, "__") {
			keys = append(keys, key)
		}
		return true
	})
	for _, key := range keys {
		vals, ok := sfres.SymbolTable.Get(key)
		if !ok {
			continue
		}
		var parentVals sfvm.Values
		if parent != nil {
			parentVals, _ = parent.SymbolTable.Get(key)
		}
		parentSet := make(map[string]struct{}, len(parentVals))
		_ = parentVals.Recursive(func(op sfvm.ValueOperator) error {
			if vv, ok := op.(*Value); ok && vv != nil && !vv.IsNil() {
				parentSet[valueIdentity(vv)] = struct{}{}
			}
			return nil
		})
		var out sfvm.Values
		_ = vals.Recursive(func(op sfvm.ValueOperator) error {
			item, ok := op.(*Value)
			if !ok {
				out = append(out, op)
				return nil
			}
			if _, inherited := parentSet[valueIdentity(item)]; inherited {
				out = append(out, item)
				return nil
			}
			if valueSatisfiesTopDefReachAnchors(item, prog, resolved, opt) {
				out = append(out, item)
			}
			return nil
		})
		sfres.SymbolTable.Set(key, out)
	}
}

func buildTopDefReachMergeHook(
	sfResult *sf.SFFrameResult,
	inputProg *Program,
	inputProgErr error,
	resolved []resolvedTopDefReach,
	opt reachableOptions,
) func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
	if len(resolved) == 0 || inputProgErr != nil || inputProg == nil || sfResult == nil {
		return nil
	}
	opt.skipLoopBackedge = true
	return func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
		filterSfFrameResultByTopDefReachAnchors(parent, child, inputProg, resolved, opt)
	}
}

func filterTopDefResultsByReachAnchors(ret Values, resolved []resolvedTopDefReach, opt reachableOptions) Values {
	if len(ret) == 0 || len(resolved) == 0 {
		return ret
	}
	prog, err := fetchProgram(ToSFVMValues(ret))
	if err != nil || prog == nil {
		return ret
	}
	filtered := make(Values, 0, len(ret))
	for _, item := range ret {
		if item == nil || item.IsEmpty() {
			continue
		}
		if valueSatisfiesTopDefReachAnchors(item, prog, resolved, opt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterSfFrameResultByOnlyReachable(parent *sf.SFFrameResult, sfres *sf.SFFrameResult, prog *Program, targetCfg *CfgCtxValue, opt reachableOptions) {
	if sfres == nil || prog == nil || targetCfg == nil || targetCfg.IsEmpty() {
		return
	}
	filterSfFrameResultByTopDefReachAnchors(parent, sfres, prog, []resolvedTopDefReach{{cfg: targetCfg, exclude: false}}, opt)
}

func DataFlowLabel(analysisType AnalysisType) string {
	switch analysisType {
	case TopDefAnalysis:
		return Predecessors_TopDefLabel
	case BottomUseAnalysis:
		return Predecessors_BottomUseLabel
	}
	return ""
}

func IsDataFlowLabel(label string) bool {
	return label == Predecessors_TopDefLabel || label == Predecessors_BottomUseLabel
}

func DataFlowWithSFConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	value *Value,
	analysisType AnalysisType,
	opts ...*sf.RecursiveConfigItem,
) sfvm.Values {
	filterCondition := make([]*filterCondition, 0)
	addHandler := func(key sf.RecursiveConfigKey, code string) {
		filterCondition = append(filterCondition, withFilterCondition(key, code))
	}
	options := make([]OperationOption, 0)
	untilCheck := CreateCheck(sfResult, config)
	hookRunner := CreateCheck(sfResult, config)

	var topDefReachParsed []topDefReachConstraint
	descentCustomized := len(config.RuntimeOptions) > 0

	for _, opt := range config.RuntimeOptions {
		if item, ok := opt.(OperationOption); ok {
			options = append(options, item)
		}
	}

	for _, item := range opts {
		switch item.Key {
		case sf.RecursiveConfig_DepthMin:
			descentCustomized = true
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithMinDepth(ret))
			}
		case sf.RecursiveConfig_Depth:
			descentCustomized = true
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithDepthLimit(ret))
			}
		case sf.RecursiveConfig_DepthMax:
			descentCustomized = true
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithMaxDepth(ret))
			}
		case sf.RecursiveConfig_Until:
			descentCustomized = true
			untilCheck.AppendItems(item)
		case sf.RecursiveConfig_Hook:
			descentCustomized = true
			hookRunner.AppendItems(item)
		case sf.RecursiveConfig_Exclude:
			addHandler(sf.RecursiveConfig_Exclude, item.Value)
		case sf.RecursiveConfig_Include:
			addHandler(sf.RecursiveConfig_Include, item.Value)
		case sf.RecursiveConfig_OnlyReachable, sf.RecursiveConfig_IncludeReachable:
			if sym := parseTopDefReachSymbolFromItem(item); sym != "" {
				topDefReachParsed = append(topDefReachParsed, topDefReachConstraint{symbol: sym, exclude: false})
			}
		case sf.RecursiveConfig_ExcludeReachable:
			if sym := parseTopDefReachSymbolFromItem(item); sym != "" {
				topDefReachParsed = append(topDefReachParsed, topDefReachConstraint{symbol: sym, exclude: true})
			}
		}
	}

	options = append(options,
		WithHookEveryNode(func(value *Value) error {
			hookRunner.CheckUntil(sfvm.ValuesOf(value))
			return nil
		}),
	)
	if !untilCheck.Empty() {
		options = append(options,
			WithUntilNode(func(v *Value) bool {
				return untilCheck.CheckUntil(sfvm.ValuesOf(v))
			}),
		)
	}

	options = append(options, WithExclusiveContext(config.GetContext()))
	// Propagate the per-rule total-work budget into the recursive descent so
	// AnalyzeContext.check() increments it per node across all sources (the
	// per-source recursiveCounter cap resets each GetTopDefs call; this shared
	// budget is what bounds N-sources × depth total work).
	if budget := config.GetWorkBudget(); budget != nil {
		options = append(options, WithExclusiveWorkBudget(budget))
	}
	var dataflowRecursiveFunc func(options ...OperationOption) Values
	if analysisType == TopDefAnalysis {
		dataflowRecursiveFunc = value.GetTopDefs
	} else if analysisType == BottomUseAnalysis {
		dataflowRecursiveFunc = value.GetBottomUses
	}

	// Reuse one descent across repeated dataflow opcodes on the same input
	// value when only the post-descent filters differ. The servlet XSS rule
	// evaluates `$sink#->` twice (include+exclude, then include-only); both
	// would otherwise run the identical, expensive GetTopDefs descent. Depth /
	// until / hook / runtime options change the descent itself, so those keep
	// the old behavior.
	cacheKey := ""
	if sfResult != nil && !descentCustomized && sfResult.DataflowDescentReuse() {
		cacheKey = dataflowDescentCacheKey(analysisType, value)
	}
	if os.Getenv("YAK_DF_CACHE_DEBUG") != "" {
		fmt.Printf("[df-cache] type=%s uid=%d reuse=%v result=%p key=%q\n", analysisType, value.GetUID(), sfResult.DataflowDescentReuse(), sfResult, cacheKey)
	}
	// dataflow analysis
	ret := Values(nil)
	cacheHit := false
	if cacheKey != "" {
		if cached, ok := sfResult.TakeDataflowResult(cacheKey); ok {
			if values, ok := cached.(Values); ok {
				ret = values
				cacheHit = true
				dataflowDescentCacheCounter.addHit()
				if os.Getenv("YAK_DF_CACHE_DEBUG") != "" {
					fmt.Printf("[df-cache] HIT key=%q values=%d\n", cacheKey, len(values))
				}
			}
		}
	}
	if !cacheHit {
		if cacheKey != "" {
			dataflowDescentCacheCounter.addMiss()
		}
		ret = dataflowRecursiveFunc(options...)
		if cacheKey != "" {
			ctxDone := false
			select {
			case <-config.GetContext().Done():
				ctxDone = true
			default:
			}
			if !ctxDone {
				if budget := config.GetWorkBudget(); budget == nil || !budget.Exceeded() {
					sfResult.SetDataflowResult(cacheKey, ret)
					if os.Getenv("YAK_DF_CACHE_DEBUG") != "" {
						fmt.Printf("[df-cache] STORE key=%q values=%d\n", cacheKey, len(ret))
					}
				}
			}
		}
	}

	inputProg, inputProgErr := fetchProgram(ToSFVMValues(ret))
	reachOpt := reachableOptions{icfg: false, maxDepth: 0, maxNodes: 0, skipLoopBackedge: true}
	resolvedReach := resolveTopDefReachAnchors(sfResult, inputProg, topDefReachParsed)
	reachMergeHook := buildTopDefReachMergeHook(sfResult, inputProg, inputProgErr, resolvedReach, reachOpt)

	ret = dataFlowFilter(config.GetContext(), ret, sfResult, config, nil, nil, reachMergeHook, filterCondition...)
	if len(resolvedReach) > 0 {
		ret = filterTopDefResultsByReachAnchors(ret, resolvedReach, reachOpt)
	}

	// set predecessor label on the explicit sfvm.Values container
	retValue := ToSFVMValues(ret)
	retValue.AppendPredecessor(value, sf.WithAnalysisContext_Label(DataFlowLabel(analysisType)))
	return retValue
}

// dataflowDescentCacheCounter is a test-only hit/miss counter. Production never
// reads it; the atomics are negligible per dataflow opcode and let tests prove
// repeated `#->` opcodes actually reuse the descent.
type dataflowDescentCacheStats struct {
	hits   int64
	misses int64
}

var dataflowDescentCacheCounter dataflowDescentCacheStats

func (c *dataflowDescentCacheStats) addHit()  { atomic.AddInt64(&c.hits, 1) }
func (c *dataflowDescentCacheStats) addMiss() { atomic.AddInt64(&c.misses, 1) }

// DataflowDescentCacheStats returns process-wide hits/misses for the repeated
// dataflow descent cache.
func DataflowDescentCacheStats() (hits, misses int64) {
	return atomic.LoadInt64(&dataflowDescentCacheCounter.hits),
		atomic.LoadInt64(&dataflowDescentCacheCounter.misses)
}

// dataflowDescentCacheKey identifies one input value's raw descent result.
// GetUID is unique per *Value instance; returning "" for a zero uid disables
// caching rather than risking a collision.
func dataflowDescentCacheKey(analysisType AnalysisType, value *Value) string {
	if value == nil {
		return ""
	}
	uid := value.GetUID()
	if uid == 0 {
		return ""
	}
	return "dataflow:" + string(analysisType) + ":" + strconv.FormatInt(uid, 10)
}

var nativeCallDataFlow sfvm.NativeCallFunc = func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	contextResult, err := frame.GetSFResult()
	if err != nil {
		return false, nil, err
	}

	include := params.GetString(0, NativeCall_DataflowParamCode, NativeCall_DataflowParamInclude)
	exclude := params.GetString(NativeCall_DataflowParamExclude)
	if len(exclude) == 0 && len(include) == 0 {
		return false, nil, utils.Errorf("exclude and include can't be empty")
	}
	var end sfvm.Values
	endName := params.GetString(
		NativeCall_DataflowParamEnd,
		NativeCall_DataflowParamDest,
		NativeCall_DataflowParamDestination,
	)
	if endName != "" {
		var ok bool
		end, ok = frame.GetSymbolByName(endName)
		if !ok {
			return false, nil, utils.Errorf("destination valueOperator %s not found", endName)
		}
	}

	vs := make(Values, 0)
	v.Recursive(func(vo sfvm.ValueOperator) error {
		v, ok := vo.(*Value)
		if !ok {
			return nil
		}
		vs = append(vs, v)
		return nil
	})
	// NOTE(large-project): on big targets (e.g. moodle, 7733 PHP files) an
	// `include` like `php-tp-all-extern-variable-param-source` can match >11k
	// sources here, and the recursive getTopDefs dataflow below runs on each →
	// heavy rules can run for 20+ min and the scan never finishes. The
	// cross-process rollback fix (restoring emptyStackHash) removed the implicit
	// early-abort depth cap that used to bound this; dataflowValueLimit/MaxDepth
	// only bound per-branch depth, not total work.
	//
	// This is now bounded by a per-rule wall-clock budget in the scan runner
	// (syntaxflow_scan/runtime.go Query -> context.WithTimeout, propagated via
	// QueryWithContext -> sfvm.Config.ctx -> config.GetContext() here). Two heavy
	// paths now honor the rule ctx:
	//   - getTopDefs/getBottomUses (DataFlowWithSFConfig, line ~414
	//     WithExclusiveContext) -> AnalyzeContext.check() ctx.Done() (analyze_context.go).
	//   - the dataflow path enumeration here (dataFlowFilter -> enumeratePaths ->
	//     GetDataflowPathWithContext -> getPathWithDirectionWithContext ->
	//     graph.GraphPathEx, which checks ctx.Done() at every DFS node). The old
	//     GraphPathWithKey used context.Background(), so the deadline never fired
	//     here — that was the real reason heavy rules ran for hours even after the
	//     per-rule timeout was added.
	// `yak code-scan` defaults the budget to 5m (--rule-timeout, 0 disables); a
	// rule that exceeds it is bailed (partial results) instead of hanging.

	var ret = vs
	var condition []*filterCondition
	if len(include) != 0 {
		condition = append(condition, withFilterCondition(sf.RecursiveConfig_Include, include))
	}
	if len(exclude) != 0 {
		condition = append(condition, withFilterCondition(sf.RecursiveConfig_Exclude, exclude))
	}

	reachSym := normalizeTopDefReachSymbol(params.GetString(
		NativeCall_DataflowParamOnlyReachable,
		NativeCall_DataflowParamOnlyReachableCamel,
		NativeCall_DataflowParamOnlyReachableKebab,
	))
	reachMode := dataflowNativeReachMode(params)
	reachOpt := dataflowNativeReachOpts(params)

	inputProg, inputProgErr := fetchProgram(ToSFVMValues(vs))

	var pathReach *dataflowPathReachFilter
	var reachMergeHook func(parent *sf.SFFrameResult, child *sf.SFFrameResult)
	if len(reachSym) > 0 && inputProgErr == nil && inputProg != nil {
		if anchor := resolveDataflowOnlyReachTarget(frame, inputProg, reachSym); anchor != nil {
			switch reachMode {
			case "path":
				pathReach = &dataflowPathReachFilter{prog: inputProg, targetCfg: anchor, opt: reachOpt}
			case "post":
				reachMergeHook = buildOnlyReachableMergeHookForResolved(inputProg, inputProgErr, anchor, reachOpt)
			}
		}
	}

	// Per-dataflow wall-clock budget: a dataflow() rule on a large project
	// can match thousands of sources and the for-each-source loop in
	// dataFlowFilter has no total-work cap. Wrap the call context with a
	// deadline; dataFlowFilter checks ctx.Done() on each source so the loop
	// unwinds cleanly and returns partial results instead of hanging.
	dataflowTimeout := loadDataflowTimeout()
	dataflowCtx := frame.GetVM().GetConfig().GetContext()
	var dataflowCancel context.CancelFunc
	if dataflowTimeout > 0 {
		dataflowCtx, dataflowCancel = context.WithTimeout(dataflowCtx, dataflowTimeout)
	}
	defer func() {
		if dataflowCancel != nil {
			dataflowCancel()
		}
	}()

	ret = dataFlowFilter(dataflowCtx, ret, contextResult, frame.GetVM().GetConfig(), end, pathReach, reachMergeHook, condition...)

	// Post-mode: same anchor pipeline as # { only_reachable: `$cfg` } -> (resolveTopDefReachAnchors + filterTopDefResultsByReachAnchors).
	if len(reachSym) > 0 && reachMode == "post" {
		if prog, err := fetchProgram(ToSFVMValues(ret)); err == nil && prog != nil {
			resolved := resolveTopDefReachAnchors(contextResult, prog, []topDefReachConstraint{{symbol: reachSym, exclude: false}})
			if len(resolved) > 0 {
				ret = filterTopDefResultsByReachAnchors(ret, resolved, reachOpt)
			}
		}
	}

	if len(ret) > 0 {
		return true, ToSFVMValues(ret), nil
	}
	return false, sfvm.NewEmptyValues(), nil
}

type filterCondition struct {
	configKey sf.RecursiveConfigKey
	code      string
}

func withFilterCondition(key sfvm.RecursiveConfigKey, code string) *filterCondition {
	return &filterCondition{
		configKey: key,
		code:      code,
	}
}

// dataflowPathReachFilter applies only_reachable during path enumeration (Phase 4 "path" mode).
type dataflowPathReachFilter struct {
	prog      *Program
	targetCfg *CfgCtxValue
	opt       reachableOptions
}

func dataFlowFilter(
	ctx context.Context,
	vs Values,
	contextResult *sf.SFFrameResult, config *sf.Config,
	end sfvm.Values,
	pathReach *dataflowPathReachFilter,
	beforeMergeHook func(parent *sf.SFFrameResult, child *sf.SFFrameResult),
	condition ...*filterCondition,
) Values {
	// for _, f := range condition {
	// 	if f.configKey != sf.RecursiveConfig_Include && f.configKey != sf.RecursiveConfig_Exclude {
	// 		return vs
	// 	}
	// }
	if len(vs) == 0 || len(condition) == 0 {
		return vs
	}

	pathCheck := CreateCheck(contextResult, config)
	if beforeMergeHook != nil {
		pathCheck.SetBeforeMergeHook(beforeMergeHook)
	}

	for _, f := range condition {
		item := &sf.RecursiveConfigItem{
			Key:            string(f.configKey),
			Value:          f.code,
			SyntaxFlowRule: true,
		}
		pathCheck.AppendItems(item)
	}

	//foreach every path,A-> B-> C-> D-> E
	//if E start dataflow. include: A && exclude:D this path is not match
	checkMatch := func(path Values) bool {
		return pathCheck.CheckMatch(ToSFVMValues(path))
	}

	// Drive the path-enumeration DFS with the rule ctx so the per-rule
	// wall-clock budget (syntaxflow_scan/runtime.go Query -> QueryWithContext
	// -> sfvm.Config.ctx, read here via config.GetContext()) can bail it. The
	// DFS (graph.GraphPathEx / DeepFirstPath.deepFirst) checks ctx.Done() at
	// every node; without passing ctx it ran under context.Background() and the
	// per-rule deadline never fired — heavy rules ran for hours on large projects.
	pathCtx := config.GetContext()
	// Exclude branch pruning: `*<getCallee> & $filter` is monotone, so a call
	// whose callee is a filter value rejects every longer path. Drop that
	// subtree instead of enumerating it only to fail CheckMatch. The pattern is
	// gated in sfCheck; unsupported exclude shapes leave prune nil.
	var prune func(*Value) bool
	if pathCheck.ExcludePrunable() {
		prune = pathCheck.CheckExcludeNode
	}

	// Include reachability prefilter: for simple `* & $symbol` includes, a
	// candidate whose effect/depend component cannot reach any bound symbol
	// value cannot produce a matching path. Reachability is linear in the saved
	// edge graph, while path enumeration can be exponential; this skips the
	// latter entirely for disconnected candidates. Unknown/complex includes
	// leave includeReach nil.
	var includeReach *dataflowIncludeReach
	if sourceIDs, ok := pathCheck.SimpleIncludeSourceIDs(); ok {
		includeReach = newDataflowIncludeReach(pathCtx, sourceIDs)
	}

	// walkPaths streams paths and reports false as soon as the current source is
	// accepted, so the old eager effect×depend product is never materialized. A
	// heavy dataflow(include=...) rule only needs the existence of one accepted
	// path per source; enumerating and allocating every path first was pure
	// overhead (and could explode combinatorially on large projects).
	var walkPaths func(v *Value, visit func(Values) bool) bool
	if pathReach != nil && pathReach.prog != nil && pathReach.targetCfg != nil && !pathReach.targetCfg.IsEmpty() {
		memo := make(map[int64]*CfgCtxValue)
		condMemo := make(map[int64]*ssa.BlockConditionSummary)
		edgeFilter := func(from, to *Value) bool {
			// Prefer block-level condition summary extraction on path nodes.
			_ = cfgConditionForValueMemo(pathReach.prog, to, memo, condMemo)
			cfgTo := cfgCtxForValueMemo(pathReach.prog, to, memo)
			if cfgTo == nil || cfgTo.IsEmpty() {
				return false
			}
			return reachableWithOptions(pathReach.prog, cfgTo, pathReach.targetCfg, pathReach.opt)
		}
		walkPaths = func(v *Value, visit func(Values) bool) bool {
			if end != nil {
				return v.walkDataflowPathWithContext(pathCtx, edgeFilter, prune, visit, FromSFVMValues(end)...)
			}
			return v.walkDataflowPathWithContext(pathCtx, edgeFilter, prune, visit)
		}
	} else {
		walkPaths = func(v *Value, visit func(Values) bool) bool {
			if end != nil {
				return v.walkDataflowPathWithContext(pathCtx, nil, prune, visit, FromSFVMValues(end)...)
			}
			return v.walkDataflowPathWithContext(pathCtx, nil, prune, visit)
		}
	}

	var ret []*Value
	for _, v := range vs {
		// Honor the rule ctx + total-work budget in the outer per-source loop. On
		// a large project `vs` can hold tens of thousands of sources (e.g. every
		// servlet/spring param for an include, or every call site reaching a sink
		// for an exclude-only dataflow); walkPaths is fast per-source but the
		// outer loop itself never re-checked the per-rule deadline, so a heavy
		// rule kept feeding sources long after its budget fired. Bailing here
		// returns the partial matches found so far.
		select {
		case <-pathCtx.Done():
			return ret
		default:
		}
		if config.EnterWork() {
			return ret
		}
		// The center value is part of every enumerated path. If it alone
		// matches the monotone exclude, skip the walk entirely.
		if prune != nil && prune(v) {
			continue
		}
		if includeReach != nil && !includeReach.CanReach(v) {
			continue
		}
		flag := false
		walkPaths(v, func(path Values) bool {
			if checkMatch(path) {
				flag = true
				return false
			}
			return true
		})
		if flag {
			ret = append(ret, v)
		}
	}
	return ret
}

// dataflowIncludeReach pre-filters top-def candidates for simple
// `* & $symbol` includes. A path emitted by GetDataflowPath is a V shape:
// EffectOn* <-> center <-> DependOn*. If none of the center's effect/depend
// reachable values is a symbol value, no enumerated path can satisfy the
// include, so walking that candidate's paths is pointless.
//
// states memoizes per Value instance (uid; pointer fallback) whether it can
// reach a source. When a BFS succeeds we may safely over-mark visited nodes as
// reachable (it only disables future pruning). When it exhausts the reachable
// set without a source, every visited node is provably unreachable: a path
// from any visited node to a source would also be a path from the start.
type dataflowIncludeReach struct {
	sourceIDs map[int64]struct{}
	states    map[dataflowReachKey]uint8
	ctx       context.Context
	steps     int
}

type dataflowReachKey struct {
	uid int64
	ptr *Value
}

const (
	includeReachUnknown uint8 = iota
	includeReachNo
	includeReachYes
)

func newDataflowIncludeReach(ctx context.Context, sourceIDs map[int64]struct{}) *dataflowIncludeReach {
	if ctx == nil {
		ctx = context.Background()
	}
	return &dataflowIncludeReach{
		sourceIDs: sourceIDs,
		states:    make(map[dataflowReachKey]uint8),
		ctx:       ctx,
	}
}

func dataflowReachKeyFor(v *Value) dataflowReachKey {
	if uid := v.GetUID(); uid != 0 {
		return dataflowReachKey{uid: uid}
	}
	return dataflowReachKey{ptr: v}
}

func (r *dataflowIncludeReach) markVisited(visited map[dataflowReachKey]struct{}, state uint8) {
	for key := range visited {
		if state == includeReachYes && r.states[key] == includeReachNo {
			continue
		}
		r.states[key] = state
	}
}

// CanReach reports whether any source is reachable from v through the saved
// EffectOn/DependOn graph (including v itself). It returns true on uncertainty
// (nil state, cancelled ctx) so the caller never skips a potentially valid
// candidate.
func (r *dataflowIncludeReach) CanReach(v *Value) bool {
	if r == nil || v == nil {
		return true
	}
	if _, ok := r.sourceIDs[v.GetId()]; ok {
		r.states[dataflowReachKeyFor(v)] = includeReachYes
		return true
	}
	startKey := dataflowReachKeyFor(v)
	switch r.states[startKey] {
	case includeReachYes:
		return true
	case includeReachNo:
		return false
	}

	visited := map[dataflowReachKey]struct{}{startKey: {}}
	queue := []*Value{v}
	hit := false
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		r.steps++
		if r.steps%4096 == 0 {
			select {
			case <-r.ctx.Done():
				return true
			default:
			}
		}
		if node == nil {
			continue
		}
		if node != v {
			if _, ok := r.sourceIDs[node.GetId()]; ok {
				hit = true
				break
			}
			switch r.states[dataflowReachKeyFor(node)] {
			case includeReachYes:
				hit = true
			case includeReachNo:
				continue
			}
			if hit {
				break
			}
		}
		enqueue := func(child *Value) {
			if child == nil {
				return
			}
			key := dataflowReachKeyFor(child)
			if _, ok := visited[key]; ok {
				return
			}
			visited[key] = struct{}{}
			queue = append(queue, child)
		}
		for _, child := range node.GetEffectOn() {
			enqueue(child)
		}
		for _, child := range node.GetDependOn() {
			enqueue(child)
		}
	}
	if hit {
		r.markVisited(visited, includeReachYes)
		return true
	}
	r.markVisited(visited, includeReachNo)
	return false
}
