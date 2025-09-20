package ssaapi

import (
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
	filterCondition := make([]*filterCondition, 0)
	addHandler := func(key sf.RecursiveConfigKey, code string) {
		filterCondition = append(filterCondition, withFilterCondition(key, code))
	}
	options := make([]OperationOption, 0)
	untilConfig := make([]*sf.RecursiveConfigItem, 0)
	untilCheck := false

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
			untilConfig = append(untilConfig, item)
			untilCheck = true
		case sf.RecursiveConfig_Hook:
			untilConfig = append(untilConfig, item)
		case sf.RecursiveConfig_Exclude:
			addHandler(sf.RecursiveConfig_Exclude, item.Value)
		case sf.RecursiveConfig_Include:
			addHandler(sf.RecursiveConfig_Include, item.Value)
		}
	}

	untilMatch := make(map[*Value]struct{})
	{
		untilCheck := CreateCheck(sfResult, config, untilConfig...)
		options = append(options, WithHookEveryNode(func(value *Value) error {
			if untilCheck.Empty() {
				return nil
			}
			if untilCheck.CheckUntil(value) {
				untilMatch[value] = struct{}{}
				return utils.Error("abort")
			}
			return nil
		}))
	}

	options = append(options, WithExclusiveContext(config.GetContext()))
	var dataflowRecursiveFunc func(options ...OperationOption) Values
	if analysisType == TopDefAnalysis {
		dataflowRecursiveFunc = value.GetTopDefs
	} else if analysisType == BottomUseAnalysis {
		dataflowRecursiveFunc = value.GetBottomUses
	}

	// dataflow analysis
	result := dataflowRecursiveFunc(options...)
	ret := make(Values, 0, len(result))
	if untilCheck {
		for v := range untilMatch {
			ret = append(ret, v)
		}
	} else {
		ret = result
	}
	// filter the result
	ret = dataFlowFilter(ret, sfResult, config, nil, filterCondition...)
	// set predecessor label
	ret.AppendPredecessor(value, sf.WithAnalysisContext_Label(DataFlowLabel(analysisType)))
	return ret
}

var nativeCallDataFlow sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	contextResult, err := frame.GetSFResult()
	if err != nil {
		return false, nil, err
	}

	include := params.GetString(0, "code", "include")
	exclude := params.GetString("exclude")
	if len(exclude) == 0 && len(include) == 0 {
		return false, nil, utils.Errorf("exclude and include can't be empty")
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

	var ret = vs
	var condition []*filterCondition
	if len(include) != 0 {
		condition = append(condition, withFilterIncludeCondition(include))
	}
	if len(exclude) != 0 {
		condition = append(condition, withFilterExcludeCondition(exclude))
	}
	ret = dataFlowFilter(ret, contextResult, frame.GetVM().GetConfig(), end, condition...)

	if len(ret) > 0 {
		return true, ret, nil
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
func dataFlowFilter(
	vs Values,
	contextResult *sf.SFFrameResult, config *sf.Config,
	end sf.ValueOperator,
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
		return pathCheck.CheckMatch(path)
	}
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
			flag := false
			paths := v.GetDataflowPath(endValues...)
			for _, path := range paths {
				if checkMatch(path) {
					flag = true
					break
				}
				continue
			}
			if flag {
				ret = append(ret, v)
			}
		}
	} else {
		for _, v := range vs {
			flag := false
			dataflowPaths := v.GetDataflowPath()
			for _, path := range dataflowPaths {
				//if match one dataflowPath break
				if checkMatch(path) {
					flag = true
					break
				}
				continue
			}
			if flag {
				ret = append(ret, v)
			}
		}
	}
	return ret
}
