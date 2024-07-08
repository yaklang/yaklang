package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
)

var ExistedNativeCall = []string{
	NativeCall_SearchFunc,
	NativeCall_GetCurrentFunc,
	NativeCall_GetFunc,
	NativeCall_GetCall,
	NativeCall_GetObject,
	NativeCall_GetMembers,
	NativeCall_GetSiblings,
}

const (
	// NativeCall_GetCurrentFunc is used to get the function of a value
	// in which function the value is
	NativeCall_GetCurrentFunc = "getCurrentFunc"

	// NativeCall_GetReturns is used to get the returns of a value
	NativeCall_GetReturns = "getReturns"

	// NativeCall_GetFormalParams is used to get the formal params of a value
	NativeCall_GetFormalParams = "getFormalParams"

	// NativeCall_GetFunc is used to get the function of a value
	// if the opcode is a call, return callee
	// if not a call, find next call instruction and find function
	NativeCall_GetFunc = "getFunc"

	// NativeCall_GetCall is used to get the call of a value, generally used to get the call of an opcode
	NativeCall_GetCall = "getCall"

	// NativeCall_SearchFunc is used to search the call of a value, generally used to search the call of a function
	// if the input is a call already, check the 'call' 's method(function) 's other call(search mode)
	//
	// searchCall is not like getCall, search call will search all function name(from call) in the program
	NativeCall_SearchFunc = "searchFunc"

	// NativeCall_GetObject is used to get the object of a value
	NativeCall_GetObject = "getObject"

	// NativeCall_GetMembers is used to get the members of a value
	NativeCall_GetMembers = "getMembers"

	// NativeCall_GetSiblings is used to get the siblings of a value
	NativeCall_GetSiblings = "getSiblings"
)

func init() {
	sfvm.RegisterNativeCall(NativeCall_GetFormalParams, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			if val.getOpcode() == ssa.SSAOpcodeFunction {
				rets, ok := ssa.ToFunction(val.node)
				if !ok {
					return nil
				}
				for _, param := range rets.Params {
					newVal := val.NewValue(param)
					newVal.AppendPredecessor(v, frame.WithPredecessorContext("getFormalParams"))
					vals = append(vals, newVal)
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(formal params) found")
	})

	sfvm.RegisterNativeCall(NativeCall_GetReturns, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			originIns := val.node
			funcIns, ok := ssa.ToFunction(originIns)
			if !ok {
				return nil
			}
			for _, ret := range funcIns.Return {
				retVal, ok := ssa.ToReturn(ret)
				if !ok {
					continue
				}
				for _, retIns := range retVal.Results {
					val := val.NewValue(retIns)
					val.AppendPredecessor(v, frame.WithPredecessorContext("getReturns"))
					vals = append(vals, val)
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(returns) found")
	})

	sfvm.RegisterNativeCall(NativeCall_GetFunc, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			switch val.getOpcode() {
			case ssa.SSAOpcodeCall:
				callee := val.GetCallee()
				if callee != nil {
					val.AppendPredecessor(v, frame.WithPredecessorContext("getFunc"))
					vals = append(vals, callee)
				}
			default:
				for _, call := range val.GetUsers() {
					if call.getOpcode() != ssa.SSAOpcodeCall {
						continue
					}
					if callee := call.GetCallee(); callee != nil {
						callee.AppendPredecessor(v, frame.WithPredecessorContext("getFunc"))
						vals = append(vals, callee)
					}
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(func) found")
	})
	sfvm.RegisterNativeCall(NativeCall_GetCurrentFunc, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			val = val.GetFunction()
			if val != nil {
				val.AppendPredecessor(v, frame.WithPredecessorContext("getCurrentFunc"))
				vals = append(vals, val)
				return nil
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(current func) found")
	})
	sfvm.RegisterNativeCall(NativeCall_GetSiblings, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			obj := val.GetObject()
			if obj == nil {
				return nil
			}
			for _, elements := range obj.GetMembers() {
				for _, val := range elements {
					if val == nil {
						continue
					}
					val.AppendPredecessor(v, frame.WithPredecessorContext("getSiblings"))
					vals = append(vals, val)
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(siblings) found")
	})
	sfvm.RegisterNativeCall(NativeCall_GetMembers, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			for _, i := range val.GetMembers() {
				for _, val := range i {
					if val == nil {
						continue
					}
					val.AppendPredecessor(v, frame.WithPredecessorContext("getMembers"))
					vals = append(vals, val)
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(members) found")
	})
	sfvm.RegisterNativeCall(NativeCall_GetObject, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			val = val.GetObject()
			if val != nil {
				val.AppendPredecessor(v, frame.WithPredecessorContext("getObject"))
				vals = append(vals, val)
				return nil
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(parent object) found")
	})
	sfvm.RegisterNativeCall(NativeCall_GetCall, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			for _, u := range val.GetUsers() {
				if u.getOpcode() == ssa.SSAOpcodeCall {
					u.AppendPredecessor(v, frame.WithPredecessorContext("getCall"))
					vals = append(vals, u)
				}
			}
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value(call) found")
	})
	sfvm.RegisterNativeCall(NativeCall_SearchFunc, func(v sfvm.ValueOperator, frame *sfvm.SFFrame) (bool, sfvm.ValueOperator, error) {
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
							next.AppendPredecessor(val, frame.WithPredecessorContext("searchCall: "+funcName))
							vals = append(vals, next)
						}
					}
				case ssa.SSAOpcodeCall:
					callee := val.GetCallee()
					if callee == nil {
						return nil
					}

					log.Warn("callee: ", callee.GetName(), callee.GetVerboseName(), callee.String())

					methodName := callee.GetName()
					if obj := callee.GetObject(); obj != nil {
						methodName, _ = strings.CutPrefix(methodName, fmt.Sprintf("#%d.", obj.GetId()))
					}

					prog := val.ParentProgram
					if prog == nil {
						return utils.Error("ParentProgram is nil")
					}
					haveNext, next, _ := prog.ExactMatch(sfvm.BothMatch, methodName)
					if haveNext && next != nil {
						next.Recursive(func(operator sfvm.ValueOperator) error {
							callee, ok := operator.(*Value)
							if !ok {
								return nil
							}
							vals = append(vals, callee)
							return nil
						})
					}
				default:
					//for _, call := range val.GetCalledBy() {
					//	call.AppendPredecessor(val, frame.WithPredecessorContext("searchCall"))
					//	funcIns := call.GetCallee()
					//	name := funcIns.GetName()
					//	log.Info(name)
					//	vals = append(vals, call)
					//}
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
