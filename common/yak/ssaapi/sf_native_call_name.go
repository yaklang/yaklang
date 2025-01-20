package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var nativeCallName sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}

		var names = []string{
			val.GetName(),
			val.GetVerboseName(),
			val.ShortString(),
			val.GetSSAValue().GetVerboseName(),
			val.GetSSAValue().GetShortVerboseName(),
		}

		if val.IsMember() {
			constVal, ok := ssa.ToConstInst(val.GetKey().GetSSAValue())
			if ok {
				names = append(names, constVal.VarString())
			}
		}

		if udef, ok := ssa.ToFunction(val.GetSSAValue()); ok {
			names = append(names, udef.GetShortVerboseName())
			names = append(names, udef.GetMethodName())
		}
		if call, ok := ssa.ToCall(val.GetSSAValue()); ok {
			method := call.GetValueById(call.Method)
			names = append(names, method.GetName())
			//todo: args?
		}

		filter := make(map[string]struct{})
		for _, name := range names {
			if name == "" {
				continue
			}
			_, existed := filter[name]
			if !existed {
				filter[name] = struct{}{}
				results := val.NewValue(ssa.NewConst(name))
				results.AppendPredecessor(val, frame.WithPredecessorContext("getFuncName"))
				vals = append(vals, results)
			}
		}

		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
}
