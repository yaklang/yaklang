package ssaapi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const (
	// NativeCall_GetReturns is used to get the returns of a value
	NativeCall_GetReturns = "getReturns"

	// NativeCall_GetFormalParams is used to get the formal params of a value
	NativeCall_GetFormalParams = "getFormalParams"

	// NativeCall_GetFunc is used to get the function of a value
	// find current function instruction which contains the value
	NativeCall_GetFunc = "getFunc"

	// NativeCall_GetCall is used to get the call of a value, generally used to get the call of an opcode
	NativeCall_GetCall = "getCall"

	// NativeCall_GetCaller is used to get the caller of a value
	// find the caller instruction which contains the value
	NativeCall_GetCaller = "getCaller"

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

	// NativeCall_TypeName is used to get the type name of a value
	NativeCall_TypeName = "typeName"

	// NativeCall_FullTypeName is used to get the full type name of a value
	NativeCall_FullTypeName = "fullTypeName"

	// NativeCall_Name is used to get the function name of a value
	NativeCall_Name = "name"

	// NativeCall_String is used to get the function name of a value
	NativeCall_String = "string"
)

func registerNativeCall(name string, options ...func(*NativeCallDocument)) {
	if name == "" {
		return
	}
	n := &NativeCallDocument{
		Name: name,
	}
	for _, o := range options {
		o(n)
	}
	NativeCallDocuments[name] = n
	sfvm.RegisterNativeCall(n.Name, n.Function)
}

func init() {
	registerNativeCall(
		NativeCall_String,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc(`获取输入指令的字符串表示`),
	)

	registerNativeCall(
		NativeCall_Name,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
				}

				if val.getOpcode() == ssa.SSAOpcodeFunction {
					fu, ok := ssa.ToFunction(val.GetSSAValue())
					if ok {
						names = append(names, fu.GetMethodName())
					}
				}

				if val.IsMember() {
					constVal, ok := ssa.ToConst(val.GetKey().GetSSAValue())
					if ok {
						names = append(names, constVal.VarString())
					}
				}

				if udef, ok := ssa.ToFunction(val.GetSSAValue()); ok {
					names = append(names, udef.GetShortVerboseName())
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
						results.AppendPredecessor(v, frame.WithPredecessorContext("getFuncName"))
						vals = append(vals, results)
					}
				}

				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value found")
		}),
		nc_desc(`获取输入指令的名称表示，例如函数名，变量名，或者字段名等`),
	)

	registerNativeCall(
		NativeCall_TypeName,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				t := val.GetType()
				if !t.IsAny() {
					typeStr := t.String()
					results := val.NewValue(ssa.NewConst(typeStr))
					results.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
					vals = append(vals, results)
				} else {
					if b, ok := t.t.(*ssa.BasicType); ok {
						typeStr := b.GetFullTypeName()

						results := val.NewValue(ssa.NewConst(typeStr))
						results.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
						vals = append(vals, results)

						// remove version if it exists
						index := strings.Index(typeStr, ":")
						if index != -1 {
							typeStr = typeStr[:index]
							results := val.NewValue(ssa.NewConst(typeStr))
							results.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
							vals = append(vals, results)
						}

						// get type name
						lastIndex := strings.LastIndex(typeStr, ".")
						if lastIndex != -1 && len(typeStr) > lastIndex+1 {
							typeStr = typeStr[lastIndex+1:]
							results := val.NewValue(ssa.NewConst(typeStr))
							results.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
							vals = append(vals, results)
						}
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value found")
		}),
		nc_desc(`获取输入指令的类型名称表示，例如int，string，或者自定义类型等：

在 Java 中，会尽可能关联到类名或导入名称，可以根据这个确定使用的类行为。
`),
	)

	registerNativeCall(
		NativeCall_FullTypeName,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				t := val.GetType()
				if !t.IsAny() {
					typeStr := t.String()
					results := val.NewValue(ssa.NewConst(typeStr))
					vals = append(vals, results)
				} else {
					if b, ok := t.t.(*ssa.BasicType); ok {
						typeStr := b.GetFullTypeName()
						results := val.NewValue(ssa.NewConst(typeStr))
						vals = append(vals, results)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value found")
		}),
		nc_desc(`获取输入指令的完整类型名称表示，例如int，string，或者自定义类型等

特殊地，在 Java 中，会尽可能使用全限定类名，例如 com.alibaba.fastjson.JSON, 也会尽可能包含 sca 版本`),
	)

	registerNativeCall(
		NativeCall_GetFormalParams,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc(`获取输入指令的形参，输入必须是一个函数指令`),
	)

	registerNativeCall(
		NativeCall_GetReturns,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc(`获取输入指令的返回值，输入必须是一个函数指令`),
	)

	registerNativeCall(
		NativeCall_GetCaller,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}

				if val.IsCall() {
					call := val.GetCallee()
					if call != nil {
						call.AppendPredecessor(v, frame.WithPredecessorContext("getCaller"))
						vals = append(vals, call)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(callers) found")
		}),
		nc_desc(`获取输入指令的调用者，输入必须是一个调用指令(call)`),
	)

	registerNativeCall(
		NativeCall_GetFunc,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				f := val.GetFunction()
				if f != nil {
					f.AppendPredecessor(v, frame.WithPredecessorContext("getFunc"))
					vals = append(vals, f)
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(func) found")
		}),
		nc_desc("获取输入指令的所在的函数，输入可以是任何指令"),
	)

	registerNativeCall(
		NativeCall_GetSiblings,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc("获取输入指令的兄弟指令，一般说的是如果这个指令是一个对象的成员，可以通过这个指令获取这个对象的其他成员。"),
	)

	registerNativeCall(
		NativeCall_GetMembers,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc("获取输入指令的成员指令，一般说的是如果这个指令是一个对象，可以通过这个指令获取这个对象的成员。"),
	)
	registerNativeCall(
		NativeCall_GetObject,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc(`获取输入指令的父对象，一般说的是如果这个指令是一个成员，可以通过这个指令获取这个成员的父对象。`),
	)
	registerNativeCall(
		NativeCall_GetCall,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
		}),
		nc_desc(`获取输入指令的调用指令，输入必须是一个函数指令`),
	)
	registerNativeCall(
		NativeCall_SearchFunc,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
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
					case ssa.SSAOpcodeConstInst:
						// name := val.GetName()
						funcName := val.String()
						if str, err := strconv.Unquote(funcName); err == nil {
							funcName = str
						}
						ok, next, _ := val.ParentProgram.ExactMatch(sfvm.BothMatch, funcName)
						if ok {
							next.AppendPredecessor(val, frame.WithPredecessorContext("searchCall: "+funcName))
							vals = append(vals, next)
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
		}),
		nc_desc(`搜索输入指令的调用指令，输入可以是任何指令，但是会尽可能搜索到调用这个指令的调用指令`),
	)
}
