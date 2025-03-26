package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
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
) Values {

	handlerResult := make([]func(v Values) Values, 0)
	addHandler := func(key sf.RecursiveConfigKey, code string) {
		handlerResult = append(handlerResult, func(v Values) Values {
			return dataFlowFilter(
				key, code, v,
				sfResult, config,
				nil,
			)
		})
	}
	options := make([]OperationOption, 0)
	configItems := make([]*sf.RecursiveConfigItem, 0)

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
			configItems = append(configItems, item)
			addHandler(sf.RecursiveConfig_Include, item.Value)
		case sf.RecursiveConfig_Hook:
			configItems = append(configItems, item)

		case sf.RecursiveConfig_Exclude:
			addHandler(sf.RecursiveConfig_Exclude, item.Value)
		case sf.RecursiveConfig_Include:
			addHandler(sf.RecursiveConfig_Include, item.Value)
		}
	}

	{
		recursiveConfig := CreateRecursiveConfigFromItems(sfResult, config, configItems...)
		options = append(options, WithHookEveryNode(func(value *Value) error {
			matchedConfigs := recursiveConfig.compileAndRun(value)
			if _, ok := matchedConfigs[sf.RecursiveConfig_Until]; ok {
				return utils.Error("abort")
			}
			return nil
		}))
	}
	var dataflowRecursiveFunc func(options ...OperationOption) Values
	if analysisType == TopDefAnalysis {
		dataflowRecursiveFunc = value.GetTopDefs
	} else if analysisType == BottomUseAnalysis {
		dataflowRecursiveFunc = value.GetBottomUses
	}

	result := dataflowRecursiveFunc(options...)
	for _, handler := range handlerResult {
		result = handler(result)
	}
	result.AppendPredecessor(value, sf.WithAnalysisContext_Label(DataFlowLabel(analysisType)))
	return result
}

var nativeCallDataFlow sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	contextResult, err := frame.GetSFResult()
	if err != nil {
		return false, nil, err
	}

	include := params.GetString(0, "code", "include")
	exclude := params.GetString("exclude")
	if len(exclude) != 0 && len(include) != 0 {
		return false, nil, utils.Errorf("exclude and include can't be used at the same time")
	}
	var end sf.ValueOperator
	endName := params.GetString("end", "dest", "destination")
	if endName != "" {
		var ok bool
		end, ok = frame.GetSymbolByName(endName)
		if !ok {
			return false, nil, utils.Errorf("destination valueOperator %s not found", endName)
		}
	}

	vs := make(Values, 0)
	v.Recursive(func(vo sf.ValueOperator) error {
		v, ok := vo.(*Value)
		if !ok {
			return nil
		}
		vs = append(vs, v)
		return nil
	})

	var ret Values
	if len(exclude) != 0 {
		ret = dataFlowFilter(
			sf.RecursiveConfig_Exclude, exclude, vs,
			contextResult, frame.GetVM().GetConfig(),
			end,
		)
	}
	if len(include) != 0 {
		ret = dataFlowFilter(
			sf.RecursiveConfig_Include, include, vs,
			contextResult, frame.GetVM().GetConfig(),
			end,
		)
	}

	if len(ret) > 0 {
		return true, ret, nil
	}
	return false, sfvm.NewEmptyValues(), nil
}

func dataFlowFilter(
	configKey sf.RecursiveConfigKey, code string,
	vs Values,
	contextResult *sf.SFFrameResult, config *sf.Config,
	end sf.ValueOperator,
) Values {
	if configKey != sf.RecursiveConfig_Exclude && configKey != sf.RecursiveConfig_Include {
		return vs
	}
	if len(vs) == 0 {
		return vs
	}

	recursiveConfig := CreateRecursiveConfigFromItems(contextResult, config, &sf.RecursiveConfigItem{
		Key:            string(configKey),
		Value:          code,
		SyntaxFlowRule: true,
	})
	var ret []*Value
	all := make(map[*Value]struct{})
	for _, v := range vs {
		all[v] = struct{}{}
	}

	if end != nil {
		var endValues Values
		switch i := end.(type) {
		case Values:
			endValues = i
		case *Value:
			endValues = Values{i}
		case *sf.ValueList:
			values, err := SFValueListToValues(i)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", i, err)
			} else {
				endValues = append(endValues, values...)
			}
		default:
			log.Warnf("dataFlowFilter: end type is not supported: %T", end)
		}
		for _, v := range vs {
			paths := v.GetDataflowPath(endValues...)
			pathNum := len(paths)
			for _, path := range paths {
				matchedConfigs := recursiveConfig.compileAndRun(path)
				if _, ok := matchedConfigs[sf.RecursiveConfig_Exclude]; ok {
					pathNum -= 1
				}
				if _, ok := matchedConfigs[sf.RecursiveConfig_Include]; ok {
					ret = append(ret, v)
				}
			}
			if pathNum == 0 {
				delete(all, v)
			}
		}
	} else {
		for _, v := range vs {
			dataPaths := v.GetDataflowPath()
			var excludeFlag = 0
			for index, dataPath := range dataPaths {
				_ = index
				matchedConfigs := recursiveConfig.compileAndRun(dataPath)

				if _, ok := matchedConfigs[sf.RecursiveConfig_Include]; ok {
					ret = append(ret, v)
					break
				}
				if _, ok := matchedConfigs[sf.RecursiveConfig_Exclude]; ok {
					excludeFlag++
				}
			}
			if excludeFlag == len(dataPaths) {
				delete(all, v)
			}
		}
	}
	switch configKey {
	case sf.RecursiveConfig_Exclude:
		return lo.Keys(all)
	case sf.RecursiveConfig_Include:
		return ret
	default:
		return vs
	}
}
