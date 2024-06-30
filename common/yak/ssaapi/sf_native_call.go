package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const (
	NativaCall_FormalParamToCall = "formalParamToCall"
)

func init() {
	sfvm.RegisterNativeCall("formalParamToCall", func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			if val, ok := operator.(*Value); ok {
				switch ins := val.getOpcode(); ins {
				case ssa.SSAOpcodeParameterMember:
					param, ok := ssa.ToParameterMember(val.node)
					if ok {
						funcName := param.GetFunc().GetName()
						if val.ParentProgram == nil {
							return utils.Error("ParentProgram is nil")
						}
						ok, next, _ := val.ParentProgram.ExactMatch(sfvm.BothMatch, funcName)
						if ok {
							vals = append(vals, next)
						}
					}
				case ssa.SSAOpcodeParameter:
					param, ok := ssa.ToParameter(val.node)
					if ok {
						funcIns := param.GetFunc()
						funcName := funcIns.GetName()
						if m := funcIns.GetMethodName(); m != "" {
							funcName = m
						}
						if val.ParentProgram == nil {
							return utils.Error("ParentProgram is nil")
						}
						ok, next, _ := val.ParentProgram.ExactMatch(sfvm.BothMatch, funcName)
						if ok {
							vals = append(vals, next)
						}
					}
				}
			}
			return nil
		})

		if len(vals) == 0 {
			return false, new(Values), utils.Errorf("no value found")
		}
		return true, sfvm.NewValues(vals), nil
	})
}
