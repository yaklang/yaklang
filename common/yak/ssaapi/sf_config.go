package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

type RecursiveConfig struct {
	contextResult *sf.SFFrameResult
	config        *sf.Config
	configItems   []*sf.RecursiveConfigItem
	vm            *sf.SyntaxFlowVirtualMachine
	frame         []*sfvm.SFFrame
}

func clearSymbolTable(res *sf.SFFrameResult) {
	res.AlertSymbolTable.Delete(sfvm.RecursiveMagicVariable)
	res.SymbolTable.Delete(sfvm.RecursiveMagicVariable)
}

func CreateRecursiveConfigFromItems(
	contextResult *sf.SFFrameResult,
	config *sf.Config,
	configItems ...*sf.RecursiveConfigItem,
) *RecursiveConfig {
	res := &RecursiveConfig{
		contextResult: contextResult,
		config:        config,
		configItems:   configItems,
		vm:            sf.NewSyntaxFlowVirtualMachine(),
	}
	clearSymbolTable(contextResult)
	res.vm.SetConfig(config)
	res.frame = make([]*sf.SFFrame, len(configItems))
	for index, item := range configItems {
		frame, err := res.vm.Compile(item.Value)
		if err == nil {
			res.frame[index] = frame
		}
	}
	return res
}

func CreateRecursiveConfigFromNativeCallParams(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	params *sf.NativeCallActualParams,
) (*RecursiveConfig, bool) {
	var opts []*sf.RecursiveConfigItem
	var hasInclude bool
	if depth := params.GetString("depth"); depth != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Hook, Value: depth, SyntaxFlowRule: false}
		opts = append(opts, configItem)
	}
	if rule := params.GetString("hook"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Hook, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}
	if rule := params.GetString("exclude"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Exclude, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}
	if rule := params.GetString("include"); rule != "" {
		hasInclude = true
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Include, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}
	if rule := params.GetString("until"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Until, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}

	return CreateRecursiveConfigFromItems(sfResult, config, opts...), hasInclude
}

// handler:用于根据RecursiveConfig配置项对每个Value行为进行处理
// 其中RecursiveConfig_Include在匹配到符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Exclude在匹配到不符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Until会沿着数据流匹配每个Value，知道匹配到符合配置项的Value的时候，数据流停止流动。
// RecursiveConfig_Hook会对匹配到的每个Value执行配置项的sfRule，但是不会影响最终结果，其数据流会持续流动。
func (r *RecursiveConfig) compileAndRun(value sf.ValueOperator) map[sf.RecursiveConfigKey]struct{} {
	isMatch := func(result *SyntaxFlowResult) bool {
		if result.GetVariableNum() == 0 {
			// check un-name value
			if len(result.GetUnNameValues()) != 0 {
				return true
			}
		} else if result.GetVariableNum() == 1 {
			match := false
			// if only one variable, check its value
			if ret := result.GetAllVariable(); ret.Len() == 1 {
				ret.ForEach(func(key string, value any) {
					num := value.(int)
					if num != 0 {
						match = true
					}
				})
			}
			return match
		} else {
			// multiple variable, check magic variable
			if len(result.GetValues(sfvm.RecursiveMagicVariable)) != 0 {
				return true
			}
		}
		return false
	}
	ret := make(map[sfvm.RecursiveConfigKey]struct{})
	for index, item := range r.configItems {
		_ = index
		if r.frame[index] == nil {
			continue
		}
		var res *SyntaxFlowResult
		var err error
		res, err = QuerySyntaxflow(
			QueryWithVM(r.vm),
			QueryWithInitVar(r.contextResult.SymbolTable),
			QueryWithValue(value),
			QueryWithFrame(r.frame[index]),
		)
		if err != nil {
			log.Errorf("syntaxflow rule exec fail: %v", err)
			continue
		}
		sfres := res.GetSFResult()
		s := CreateResultFromQuery(sfres)
		if isMatch(s) {
			ret[sf.RecursiveConfigKey(item.Key)] = struct{}{}
		}
		clearSymbolTable(sfres)
		r.contextResult.MergeByResult(sfres)
	}
	return ret
}
