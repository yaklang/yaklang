package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func WithSyntaxFlowConfig(opts ...sf.RecursiveConfigItem) OperationOption {
	var results []OperationOption
	var exec = func(newOpts ...OperationOption) OperationOption {
		return func(operationConfig *OperationConfig) {
			for _, p := range newOpts {
				p(operationConfig)
			}
		}
	}
	for _, op := range opts {
		key := sf.FormatRecursiveConfigKey(op.Key)
		switch key {
		case sf.RecursiveConfig_Depth:
			if ret := codec.Atoi(op.Value); ret > 0 {
				results = append(results, WithDepthLimit(ret))
			}
		case sf.RecursiveConfig_DepthMin:
			if ret := codec.Atoi(op.Value); ret > 0 {
				results = append(results, WithMinDepth(ret))
			}
		case sf.RecursiveConfig_DepthMax:
			if ret := codec.Atoi(op.Value); ret > 0 {
				results = append(results, WithMaxDepth(ret))
			}
		case sf.RecursiveConfig_Exclude:
			if op.Value != "" {
				vm := sf.NewSyntaxFlowVirtualMachine()
				err := vm.Compile(op.Value)
				if err != nil {
					log.Warnf("SyntaxFlow compile %#v failed: %v", op.Value, err)
					continue
				}
				results = append(results, WithHookEveryNode(func(value *Value) error {
					if !op.SyntaxFlowRule {
						return utils.Error("exclude value must be a syntaxflow rule")
					}
					find := false
					//vm.Feed(value).ForEach(func(s string, operator sf.ValueOperator) bool {
					//	err := operator.Recursive(func(o sf.ValueOperator) error {
					//		raw, ok := o.(*Value)
					//		if !ok {
					//			return nil
					//		}
					//		if raw.GetId() == value.GetId() {
					//			find = true
					//			return utils.Error("abort")
					//		}
					//		return nil
					//	})
					//	if err != nil {
					//		return false
					//	}
					//	return true
					//})
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					log.Warn("TBD for RecursiveConfig_Exclude")
					if find {
						return utils.Error("abort")
					}
					return nil
				}))
			}
		}
	}
	return exec(results...)
}
