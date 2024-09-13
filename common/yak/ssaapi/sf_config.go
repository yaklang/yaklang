package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type RecursiveConfig struct {
	sfResult    *sf.SFFrameResult
	config      *sf.Config
	configItems []*sf.RecursiveConfigItem
	vm          *sf.SyntaxFlowVirtualMachine
	// depth limit
	depth int
}

type RecursiveKind int

// comparePriority 比较指令优先级并且替换

func (r RecursiveKind) comparePriority(kind RecursiveKind) RecursiveKind {
	if kind > r {
		return kind
	}
	return r
}

const (
	//有默认的优先级顺序

	// ContinueMatch 匹配对应Value，数据流继续流动
	ContinueSkip RecursiveKind = iota
	// ContinueSkip 不匹配对应Value，数据流继续流动
	ContinueMatch
	// StopMatch 匹配对应Value，数据流停止流动
	StopMatch
	// StopNoMatch StopNoMatch:不匹配对应Value，数据流停止流动
	StopNoMatch
)

func CreateRecursiveConfigFromItems(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	opts ...*sf.RecursiveConfigItem,
) *RecursiveConfig {
	res := &RecursiveConfig{
		sfResult:    sfResult,
		config:      config,
		configItems: opts,
		vm:          sf.NewSyntaxFlowVirtualMachine(),
	}
	res.vm.SetConfig(config)

	// handler other config
	for _, op := range opts {
		switch op.Key {
		case sf.RecursiveConfig_Depth:
			if ret := codec.Atoi(op.Value); ret > 0 {
				res.depth = ret
			}

		}
	}

	return res
}

func CreateRecursiveConfigFromNativeCallParams(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	params *sf.NativeCallActualParams,
) *RecursiveConfig {
	var opts []*sf.RecursiveConfigItem
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
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Include, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}
	if rule := params.GetString("until"); rule != "" {
		configItem := &sf.RecursiveConfigItem{Key: sf.RecursiveConfig_Until, Value: rule, SyntaxFlowRule: true}
		opts = append(opts, configItem)
	}

	return CreateRecursiveConfigFromItems(sfResult, config, opts...)
}

// handler:用于根据RecursiveConfig配置项对每个Value行为进行处理
// 其中RecursiveConfig_Include在匹配到符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Exclude在匹配到不符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Until会沿着数据流匹配每个Value，知道匹配到符合配置项的Value的时候，数据流停止流动。
// RecursiveConfig_Hook会对匹配到的每个Value执行配置项的sfRule，但是不会影响最终结果，其数据流会持续流动。
func (r *RecursiveConfig) compileAndRun(value *Value) RecursiveKind {
	var resultKind = ContinueSkip
	matchConfig := func(key string, result Values) {
		switch key {
		case sf.RecursiveConfig_Exclude:
			if len(result) == 0 {
				resultKind = resultKind.comparePriority(ContinueMatch)
			}
			for _, v := range result {
				if v == nil {
					resultKind = resultKind.comparePriority(ContinueMatch)
				}
				if !ValueCompare(value, v) {
					resultKind = resultKind.comparePriority(ContinueMatch)
				}
			}
		case sf.RecursiveConfig_Include:
			for _, v := range result {
				if ValueCompare(value, v) {
					resultKind = resultKind.comparePriority(ContinueMatch)
				} else {
					resultKind = resultKind.comparePriority(ContinueSkip)
				}
			}
		case sf.RecursiveConfig_Hook:
			resultKind = resultKind.comparePriority(ContinueMatch)
		case sf.RecursiveConfig_Until:
			for _, v := range result {
				if ValueCompare(value, v) {
					resultKind = resultKind.comparePriority(StopMatch)
				} else {
					resultKind = resultKind.comparePriority(ContinueMatch)
				}
			}
		}
	}
	lo.ForEach(r.configItems, func(item *sf.RecursiveConfigItem, index int) {
		if !item.SyntaxFlowRule {
			return
		}
		frame, err := r.vm.Compile(item.Value)
		if err != nil {
			log.Errorf("syntaxflow rule compile fail: %v", utils.Errorf("SyntaxFlow compile %#v failed: %v", item.Value, err))
			return
		}
		frame.WithContext(r.sfResult)
		res, err := frame.Feed(value)
		if err != nil {
			log.Errorf("frame exec opcode fail: %s", err)
			return
		}
		s := CreateResultFromQuery(res)
		r.sfResult.MergeByResult(res)
		matchConfig(item.Key, s.GetAllValuesChain())
	})
	return resultKind
}

func WithSyntaxFlowConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	cb func(...OperationOption) Values,
	opts ...*sf.RecursiveConfigItem,
) Values {
	var (
		use        bool
		options    []OperationOption
		_recursive = CreateRecursiveConfigFromItems(sfResult, config, opts...)
		results    Values
	)
	handler := func() {
		options = append(options, WithHookEveryNode(func(value *Value) error {
			switch _recursive.compileAndRun(value) {
			case ContinueMatch:
				results = append(results, value)
				return nil
			case ContinueSkip:
				return nil
			case StopMatch:
				results = append(results, value)
				return utils.Error("abort")
			case StopNoMatch:
				return utils.Error("abort")
			default:
				return nil
			}
		}))
	}
	lo.ForEach(opts, func(item *sf.RecursiveConfigItem, index int) {
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
		case sf.RecursiveConfig_Until, sf.RecursiveConfig_Exclude, sf.RecursiveConfig_Include:
			use = true
		default:
			if !use {
				use = false
			}
		}
	})
	handler()
	if use {
		cb(options...)
		return results
	}
	return cb(options...)
}
