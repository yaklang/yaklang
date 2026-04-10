package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	Predecessors_TopDefLabel    = "dataflow_topdef"
	Predecessors_BottomUseLabel = "dataflow_bottomuse"
)

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
	ret = dataFlowFilter(ret, sfResult, config, nil, nil, filterCondition...)
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
		condition = append(condition, withFilterIncludeCondition(include))
	}
	if len(exclude) != 0 {
		condition = append(condition, withFilterExcludeCondition(exclude))
	}

	onlyReachVar := params.GetString("only_reachable", "onlyReachable", "only-reachable")
	onlyReachVar = codec.AnyToString(onlyReachVar)
	onlyReachVar = strings.TrimSpace(onlyReachVar)
	onlyReachVar = strings.TrimPrefix(onlyReachVar, "$")

	mode := strings.ToLower(strings.TrimSpace(params.GetString("only_reachable_mode", "onlyReachableMode")))
	if mode == "" {
		if parseBoolParam(params, "strict", false) {
			mode = "path"
		} else {
			mode = "post"
		}
	}

	var pathReach *dataflowPathReachFilter
	if onlyReachVar != "" && mode == "path" {
		targetOp, ok := frame.GetSymbolByName(onlyReachVar)
		if ok && targetOp != nil {
			var targetCfg *CfgCtxValue
			_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
				if c, ok := operator.(*CfgCtxValue); ok && c != nil && !c.IsEmpty() {
					targetCfg = c
					return utils.Error("abort")
				}
				return nil
			})
			if targetCfg != nil {
				if prog, err := fetchProgram(ToSFVMValues(vs)); err == nil && prog != nil {
					icfg := parseBoolParam(params, "icfg", false)
					opt := reachableOptsFromParams(params, icfg)
					pathReach = &dataflowPathReachFilter{prog: prog, targetCfg: targetCfg, opt: opt}
				}
			}
		}
	}

	ret = dataFlowFilter(ret, contextResult, frame.GetVM().GetConfig(), end, pathReach, condition...)

	// Phase-3/4: post-filter only in default post mode; path mode applies CFG during path enumeration.
	if onlyReachVar != "" && mode == "post" {
		targetOp, ok := frame.GetSymbolByName(onlyReachVar)
		if ok && targetOp != nil {
			var targetCfg *CfgCtxValue
			_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
				if c, ok := operator.(*CfgCtxValue); ok && c != nil && !c.IsEmpty() {
					targetCfg = c
					return utils.Error("abort")
				}
				return nil
			})
			if targetCfg != nil {
				prog, err := fetchProgram(ToSFVMValues(ret))
				if err == nil && prog != nil {
					icfg := parseBoolParam(params, "icfg", false)
					opt := reachableOptsFromParams(params, icfg)

					filtered := make(Values, 0, len(ret))
					for _, item := range ret {
						if item == nil || item.IsEmpty() {
							continue
						}
						ok, got, _ := nativeCallGetCFG(sfvm.ValuesOf(item), frame, sfvm.NewNativeCallActualParams())
						if !ok || got == nil || got.IsEmpty() {
							continue
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
							continue
						}
						if reachableWithOptions(prog, candCfg, targetCfg, opt) {
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

func withFilterIncludeCondition(code string) *filterCondition {
	return &filterCondition{
		configKey: sf.RecursiveConfig_Include,
		code:      code,
	}
}
func withFilterExcludeCondition(code string) *filterCondition {
	return &filterCondition{
		configKey: sf.RecursiveConfig_Exclude,
		code:      code,
	}
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
		edgeFilter := func(from, to *Value) bool {
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
	all := make(map[*Value]struct{})
	for _, v := range vs {
		all[v] = struct{}{}
	}
	if end != nil {
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
	} else {
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
	}
	return ret
}
