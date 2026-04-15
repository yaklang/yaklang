package ssaapi

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

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
	ok, got, _ := nativeCallGetCFG(sfvm.ValuesOf(v), nil, sfvm.NewNativeCallActualParams())
	if !ok || got == nil || got.IsEmpty() {
		return false
	}
	var candCfg *CfgCtxValue
	_ = got.Recursive(func(op sfvm.ValueOperator) error {
		if c, ok := op.(*CfgCtxValue); ok && c != nil && !c.IsEmpty() {
			candCfg = c
			return utils.Error("abort")
		}
		return nil
	})
	if candCfg == nil {
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

// parseTopDefReachSymbolFromItem reads the value from a RecursiveConfigItem (plain identifier or `` `$cfg` `` filter text).
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

	for _, opt := range config.RuntimeOptions {
		if item, ok := opt.(OperationOption); ok {
			options = append(options, item)
		}
	}

	for _, item := range opts {
		switch item.Key {
		case sf.RecursiveConfig_DepthMin:
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithMinDepth(ret))
			}
		case sf.RecursiveConfig_Depth:
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithDepthLimit(ret))
			}
		case sf.RecursiveConfig_DepthMax:
			if ret := codec.Atoi(item.Value); ret > 0 {
				options = append(options, WithMaxDepth(ret))
			}
		case sf.RecursiveConfig_Until:
			untilCheck.AppendItems(item)
		case sf.RecursiveConfig_Hook:
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
	var dataflowRecursiveFunc func(options ...OperationOption) Values
	if analysisType == TopDefAnalysis {
		dataflowRecursiveFunc = value.GetTopDefs
	} else if analysisType == BottomUseAnalysis {
		dataflowRecursiveFunc = value.GetBottomUses
	}

	// dataflow analysis
	ret := dataflowRecursiveFunc(options...)

	inputProg, inputProgErr := fetchProgram(ToSFVMValues(ret))
	reachOpt := reachableOptions{icfg: false, maxDepth: 0, maxNodes: 0, skipLoopBackedge: true}
	resolvedReach := resolveTopDefReachAnchors(sfResult, inputProg, topDefReachParsed)
	reachMergeHook := buildTopDefReachMergeHook(sfResult, inputProg, inputProgErr, resolvedReach, reachOpt)

	ret = dataFlowFilter(ret, sfResult, config, nil, nil, reachMergeHook, filterCondition...)
	if len(resolvedReach) > 0 {
		ret = filterTopDefResultsByReachAnchors(ret, resolvedReach, reachOpt)
	}

	// set predecessor label on the explicit sfvm.Values container
	retValue := ToSFVMValues(ret)
	retValue.AppendPredecessor(value, sf.WithAnalysisContext_Label(DataFlowLabel(analysisType)))
	return retValue
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

	ret = dataFlowFilter(ret, contextResult, frame.GetVM().GetConfig(), end, pathReach, reachMergeHook, condition...)

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

	var enumeratePaths func(v *Value) []Values
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
		enumeratePaths = func(v *Value) []Values {
			if end != nil {
				return v.GetDataflowPathWithEdgeFilter(edgeFilter, FromSFVMValues(end)...)
			}
			return v.GetDataflowPathWithEdgeFilter(edgeFilter)
		}
	} else {
		enumeratePaths = func(v *Value) []Values {
			if end != nil {
				return v.GetDataflowPath(FromSFVMValues(end)...)
			}
			return v.GetDataflowPath()
		}
	}

	var ret []*Value
	for _, v := range vs {
		flag := false
		for _, path := range enumeratePaths(v) {
			if checkMatch(path) {
				flag = true
				break
			}
		}
		if flag {
			ret = append(ret, v)
		}
	}
	return ret
}
