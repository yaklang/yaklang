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
func filterSfFrameResultByOnlyReachable(parent *sf.SFFrameResult, sfres *sf.SFFrameResult, prog *Program, targetCfg *CfgCtxValue, opt reachableOptions) {
	if sfres == nil || prog == nil || targetCfg == nil || targetCfg.IsEmpty() {
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
			// Values inherited from parent context should keep default behavior and
			// must not be retroactively filtered by this only_reachable anchor.
			if _, inherited := parentSet[valueIdentity(item)]; inherited {
				out = append(out, item)
				return nil
			}
			if valuePassesCFGReach(item, prog, targetCfg, opt) {
				out = append(out, item)
			}
			return nil
		})
		sfres.SymbolTable.Set(key, out)
	}
}

func buildOnlyReachableBeforeMergeHook(
	frame *sfvm.SFFrame,
	onlyReachVar string,
	mode string,
	inputProg *Program,
	inputProgErr error,
	params *sfvm.NativeCallActualParams,
) func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
	if onlyReachVar == "" || mode != "post" || inputProgErr != nil || inputProg == nil {
		return nil
	}
	if frame == nil {
		return nil
	}
	targetOp, ok := frame.GetSymbolByName(onlyReachVar)
	if !ok || targetOp == nil || targetOp.IsEmpty() {
		return nil
	}
	targetCfg := cfgCtxFromOnlyReachTargetOp(inputProg, targetOp)
	if targetCfg == nil {
		return nil
	}
	icfg := parseBoolParam(params, "icfg", false)
	opt := reachableOptsFromParams(params, icfg)
	opt.skipLoopBackedge = true
	return func(parent *sf.SFFrameResult, child *sf.SFFrameResult) {
		filterSfFrameResultByOnlyReachable(parent, child, inputProg, targetCfg, opt)
	}
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
	// filter the result
	ret = dataFlowFilter(ret, sfResult, config, nil, nil, nil, filterCondition...)
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

	include := params.GetString(0, "code", "include")
	exclude := params.GetString("exclude")
	if len(exclude) == 0 && len(include) == 0 {
		return false, nil, utils.Errorf("exclude and include can't be empty")
	}
	var end sfvm.Values
	endName := params.GetString("end", "dest", "destination")
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

	onlyReachVar := params.GetString("only_reachable", "onlyReachable", "only-reachable")
	onlyReachVar = codec.AnyToString(onlyReachVar)
	onlyReachVar = strings.TrimSpace(onlyReachVar)
	onlyReachVar = strings.TrimPrefix(onlyReachVar, "$")

	// only_reachable_mode: "post" = filter dataflow results after include/exclude; "path" = require each
	// enumerated path step's cfg to reach the anchor (see dataflowPathReachFilter). When mode omitted,
	// strict=true is treated as "path" (alias); otherwise default "post".
	mode := strings.ToLower(strings.TrimSpace(params.GetString("only_reachable_mode", "onlyReachableMode")))
	if mode == "" {
		if parseBoolParam(params, "strict", false) {
			mode = "path"
		} else {
			mode = "post"
		}
	}

	// Program for path-mode CFG: resolve from input values (path steps share the same closure as vs).
	inputProg, inputProgErr := fetchProgram(ToSFVMValues(vs))

	var pathReach *dataflowPathReachFilter
	if onlyReachVar != "" && mode == "path" && inputProgErr == nil && inputProg != nil {
		targetOp, ok := frame.GetSymbolByName(onlyReachVar)
		if ok && targetOp != nil && !targetOp.IsEmpty() {
			if targetCfg := cfgCtxFromOnlyReachTargetOp(inputProg, targetOp); targetCfg != nil {
				icfg := parseBoolParam(params, "icfg", false)
				opt := reachableOptsFromParams(params, icfg)
				opt.skipLoopBackedge = true
				pathReach = &dataflowPathReachFilter{prog: inputProg, targetCfg: targetCfg, opt: opt}
			}
		}
	}

	onlyReachableBeforeMergeHook := buildOnlyReachableBeforeMergeHook(frame, onlyReachVar, mode, inputProg, inputProgErr, params)

	ret = dataFlowFilter(ret, contextResult, frame.GetVM().GetConfig(), end, pathReach, onlyReachableBeforeMergeHook, condition...)

	// Phase-3/4: post-filter only in default post mode; path mode applies CFG during path enumeration.
	// Post-filter must resolve Program from filtered results: dataflow leaves may carry ParentProgram
	// where the native-call input chain does not, so do not reuse inputProg here.
	if onlyReachVar != "" && mode == "post" {
		targetOp, ok := frame.GetSymbolByName(onlyReachVar)
		if ok && targetOp != nil && !targetOp.IsEmpty() {
			prog, err := fetchProgram(ToSFVMValues(ret))
			if err == nil && prog != nil {
				targetCfg := cfgCtxFromOnlyReachTargetOp(prog, targetOp)
				if targetCfg != nil {
					icfg := parseBoolParam(params, "icfg", false)
					opt := reachableOptsFromParams(params, icfg)
					opt.skipLoopBackedge = true

					filtered := make(Values, 0, len(ret))
					for _, item := range ret {
						if item == nil || item.IsEmpty() {
							continue
						}
						if valuePassesCFGReach(item, prog, targetCfg, opt) {
							filtered = append(filtered, item)
						}
					}
					ret = filtered
				}
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
