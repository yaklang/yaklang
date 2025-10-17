package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func getValueNames(val *Value) []string {
	name := val.GetVerboseName()
	//过滤掉前面的ID号
	index := strings.Index(name, ":")
	if index != -1 {
		name = name[index+1:]
	}
	var names = []string{
		val.GetName(),
		name,
		val.ShortString(),
		val.GetSSAInst().GetVerboseName(),
	}
	if val.IsMember() {
		constVal, ok := ssa.ToConstInst(val.GetKey().GetSSAInst())
		if ok {
			names = append(names, constVal.VarString())
		}
	}

	if udef, ok := ssa.ToFunction(val.GetSSAInst()); ok {
		names = append(names, udef.GetShortVerboseName())
		names = append(names, udef.GetMethodName())
	}
	if call, b := ssa.ToCall(val.GetSSAInst()); b {
		method, ok := call.GetValueById(call.Method)
		if ok && method != nil {
			names = append(names, method.GetName())
		}
		//todo: args?
	}
	return names
}

var nativeCallName sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		names := getValueNames(val)
		filter := make(map[string]struct{})
		for _, name := range names {
			if name == "" {
				continue
			}
			_, existed := filter[name]
			if !existed {
				filter[name] = struct{}{}
				results := val.NewConstValue(name, val.GetRange())
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
