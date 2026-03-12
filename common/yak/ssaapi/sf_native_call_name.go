package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
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

var nativeCallName sfvm.NativeCallFunc = sfvm.ValueSingleNativeCall(func(operator sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (sfvm.Values, error) {
	val, ok := operator.(*Value)
	if !ok {
		return sfvm.NewEmptyValues(), nil
	}
	names := getValueNames(val)
	filter := make(map[string]struct{})
	results := make([]sfvm.ValueOperator, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, existed := filter[name]; existed {
			continue
		}
		filter[name] = struct{}{}
		ret := val.NewConstValue(name, val.GetRange())
		ret.AppendPredecessor(val, frame.WithPredecessorContext("getFuncName"))
		results = append(results, ret)
	}
	return sfvm.NewValues(results), nil
})
