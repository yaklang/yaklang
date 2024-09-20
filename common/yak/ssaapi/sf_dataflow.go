package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func WithSyntaxFlowConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	dataflowRecursiveFunc func(...OperationOption) Values,
	opts ...*sf.RecursiveConfigItem,
) sf.ValueOperator {

	var handlerResult func(v Values) Values
	resultModeErr := utils.Errorf("exclude and include can't be used at the same time")
	handlerResult = nil

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
		case sf.RecursiveConfig_Hook:
			configItems = append(configItems, item)

		case sf.RecursiveConfig_Exclude:
			if handlerResult == nil {
				// exist include
				log.Errorf("config: %v", resultModeErr)
			}

			handlerResult = func(v Values) Values {
				return dataFlowFilter(
					sf.RecursiveConfig_Exclude, item.Value, v,
					sfResult, config,
				)
			}
		case sf.RecursiveConfig_Include:
			if handlerResult != nil {
				// exist exclude
				log.Errorf("config: %v", resultModeErr)
			}
			handlerResult = func(v Values) Values {
				return dataFlowFilter(
					sf.RecursiveConfig_Include, item.Value, v,
					sfResult, config,
				)
			}
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

	result := dataflowRecursiveFunc(options...)
	if handlerResult != nil {
		result = handlerResult(result)
	}
	return result
}

var nativeCallDataFlow sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	contextResult, err := frame.GetSFResult()
	if err != nil {
		return false, nil, err
	}

	exclude := params.GetString(0, "code", "exclude")
	include := params.GetString("include")
	if len(exclude) != 0 && len(include) != 0 {
		return false, nil, utils.Errorf("exclude and include can't be used at the same time")
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
		)
	}
	if len(include) != 0 {
		ret = dataFlowFilter(
			sf.RecursiveConfig_Include, include, vs,
			contextResult, frame.GetVM().GetConfig(),
		)
	}

	if len(ret) > 0 {
		return true, ret, nil
	}
	return false, sfvm.NewValues(nil), nil
}

func dataFlowFilter(
	configKey sf.RecursiveConfigKey, code string,
	vs Values,
	contextResult *sf.SFFrameResult, config *sf.Config,
) Values {
	if configKey != sf.RecursiveConfig_Exclude && configKey != sf.RecursiveConfig_Include {
		return vs
	}

	var ret []*Value
	all := make(map[*Value]struct{})
	for _, v := range vs {
		all[v] = struct{}{}
	}
	recursiveConfig := CreateRecursiveConfigFromItems(contextResult, config, &sf.RecursiveConfigItem{
		Key:            string(configKey),
		Value:          code,
		SyntaxFlowRule: true,
	})
	for _, v := range vs {
		dataPath := v.GetDataFlowPath()
		// log.Infof("v: %v", v)
		// log.Infof("dataPath: %v", dataPath)
		matchedConfigs := recursiveConfig.compileAndRun(dataPath)
		if _, ok := matchedConfigs[sf.RecursiveConfig_Exclude]; ok {
			delete(all, v)
		}

		if _, ok := matchedConfigs[sf.RecursiveConfig_Include]; ok {
			ret = append(ret, v)
		}
	}

	switch configKey {
	case sf.RecursiveConfig_Exclude:
		return Values(lo.Keys(all))
	case sf.RecursiveConfig_Include:
		return Values(ret)
	default:
		// log.Info("")
		return vs
	}
}
