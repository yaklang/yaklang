package ssaapi

import (
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func WithSyntaxFlowConfig(
	sfResult *sf.SFFrameResult,
	config *sf.Config,
	cb func(...OperationOption) Values,
	opts ...*sf.RecursiveConfigItem,
) Values {
	var options []OperationOption
	var result []*Value
	useResult := false

	runSyntaxFlow := func(
		op *sf.RecursiveConfigItem,
		cb func(map[string]Values, *Value) error,
	) {
		if op.Value == "" {
			return
		}
		options = append(options, WithHookEveryNode(func(value *Value) error {
			if !op.SyntaxFlowRule {
				return utils.Error("exclude value must be a syntaxFlow rule")
			}
			//res,err := SyntaxFlowWithError(value,op.Value)
			res, err := SyntaxFlowWithOldEnv(value, op.Value, sfResult, config)
			if err != nil {
				return err
			}
			return cb(res.GetAllValues(), value)
		}))
	}

	for _, op := range opts {
		// key := sf.FormatRecursiveConfigKey(op.Key)
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
			runSyntaxFlow(op, func(m map[string]Values, v *Value) error {
				for _, results := range m {
					for _, result := range results {
						if ValueCompare(result, v) {
							return utils.Error("abort")
						}
					}
				}
				return nil
			})

		case sf.RecursiveConfig_Until:
			runSyntaxFlow(op, func(m map[string]Values, v *Value) error {
				for _, sfDatas := range m {
					for _, sfData := range sfDatas {
						if ValueCompare(sfData, v) {
							result = append(result, sfData)
							return utils.Error("abort")
						}
					}
				}
				return nil
			})
			useResult = true
		case sf.RecursiveConfig_Hook:
			runSyntaxFlow(op, func(m map[string]Values, v *Value) error {
				return nil
			})
		}
	}

	if useResult {
		cb(options...)
		return result
	}
	return cb(options...)
}
