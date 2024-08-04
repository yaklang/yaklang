package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var nativeCallString = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {

	var vals []sfvm.ValueOperator
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		results := val.NewValue(ssa.NewConst(val.String()))
		results.AppendPredecessor(val, frame.WithPredecessorContext("string"))
		vals = append(vals, results)
		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
}
