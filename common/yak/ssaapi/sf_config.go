package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type RecursiveConfig struct {
	sfResult    *sf.SFFrameResult
	config      *sf.Config
	configItems []*sf.RecursiveConfigItem
	// depth limit
	depth int
}

// RecursiveConfigOption
// SytaxFlow的Config语法通过WithHookEveryNode选项实现在每次Filter的时候进行回调
// RecursiveConfigOption则决定回调过程中对相关Value行为的处理，具体行为如下:
// ContinueMatch: 匹配对应Value，数据流继续流动
// ContinueSkip: 不匹配对应Value，数据流继续流动
// StopMatch: 匹配对应Value，数据流停止流动
// StopNoMatch: 不匹配对应Value，数据流停止流动
// Nothing: 不处理对应Value,一般用于hook，避免hook执行的结果影响最终结果
type RecursiveConfigOption int

const (
	ContinueMatch RecursiveConfigOption = iota
	ContinueSkip
	StopMatch
	StopNoMatch
	Nothing
)

func CreateRecursiveConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	opts ...*sf.RecursiveConfigItem,
) *RecursiveConfig {
	res := &RecursiveConfig{
		sfResult:    sfResult,
		config:      config,
		configItems: opts,
	}

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

// handler:用于根据RecursiveConfig配置项对每个Value行为进行处理
// 其中RecursiveConfig_Include在匹配到符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Exclude在匹配到不符合配置项的Value后，数据流继续流动，以匹配其它Value。
// RecursiveConfig_Until会沿着数据流匹配每个Value，知道匹配到符合配置项的Value的时候，数据流停止流动。
// RecursiveConfig_Hook会对匹配到的每个Value执行配置项的sfRule，但是不会影响最终结果，其数据流会持续流动。
func (r *RecursiveConfig) handler(value *Value) RecursiveConfigOption {
	for _, op := range r.configItems {
		if !op.SyntaxFlowRule {
			continue
		}
		res, err := SyntaxFlowWithVMContext(value, op.Value, r.sfResult, r.config)
		if err != nil {
			log.Errorf("SyntaxFlowWithVMContext error: %v", err)
			continue
		}

		switch op.Key {
		case sf.RecursiveConfig_Exclude:
			isInclude := false
			for _, sfDatas := range res.GetAllValues() {
				for _, sfData := range sfDatas {
					if ValueCompare(sfData, value) {
						isInclude = true
					}
				}
			}
			if !isInclude {
				return ContinueMatch
			} else {
				return ContinueSkip
			}
		case sf.RecursiveConfig_Include:
			isInclude := false
			for _, sfDatas := range res.GetAllValues() {
				for _, sfData := range sfDatas {
					if ValueCompare(sfData, value) {
						isInclude = true
					}
				}
			}

			if isInclude {
				return ContinueMatch
			} else {
				return ContinueSkip
			}
		case sf.RecursiveConfig_Hook:
			// do nothing for outer
			return Nothing
		case sf.RecursiveConfig_Until:
			for _, sfDatas := range res.GetAllValues() {
				for _, sfData := range sfDatas {
					if ValueCompare(sfData, value) {
						return StopMatch
					} else {
						return ContinueMatch
					}
				}
			}
			return Nothing
		}

	}
	return ContinueMatch
}

func WithSyntaxFlowConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	cb func(...OperationOption) Values,
	opts ...*sf.RecursiveConfigItem,
) Values {
	var options []OperationOption
	var result []*Value
	var useResult bool

	rc := CreateRecursiveConfig(sfResult, config, opts...)
	handlerValue := func(rc *RecursiveConfig) {
		options = append(options, WithHookEveryNode(func(value *Value) error {
			configOption := rc.handler(value)
			switch configOption {
			case ContinueSkip:
				return nil
			case ContinueMatch:
				result = append(result, value)
				return nil
			case StopMatch:
				result = append(result, value)
				return utils.Error("abort")
			case StopNoMatch:
				return utils.Error("abort")
			case Nothing:
				return nil	
			default:
				return nil
			}
		}))
	}

	for _, op := range opts {
		switch op.Key {
		case sf.RecursiveConfig_Depth:
			if ret := codec.Atoi(op.Value); ret > 0 {
				options = append(options, WithDepthLimit(ret))
			}
		case sf.RecursiveConfig_DepthMin:
			if ret := codec.Atoi(op.Value); ret > 0 {
				options = append(options, WithMinDepth(ret))
			}
		case sf.RecursiveConfig_DepthMax:
			if ret := codec.Atoi(op.Value); ret > 0 {
				options = append(options, WithMaxDepth(ret))
			}
		case sf.RecursiveConfig_Exclude:
			handlerValue(rc)
			useResult = true
		case sf.RecursiveConfig_Until:
			handlerValue(rc)
			useResult = true
		case sf.RecursiveConfig_Hook:
			handlerValue(rc)
		case sf.RecursiveConfig_Include:
			handlerValue(rc)
			useResult = true
		}
	}

	if useResult {
		cb(options...)
		return result
	}
	return cb(options...)
}
