package ssa

import "github.com/yaklang/yaklang/common/log"

type Functions []*Function
type FunctionProcess func(functions Functions) Functions

func (f Functions) GetFunctionByHash(hash string) *Function {
	for _, function := range f {
		if function.hash == hash {
			return function
		}
	}
	return nil
}
func (f Functions) CheckFunctionExitByHash(hash string) bool {
	return f.GetFunctionByHash(hash) != nil
}
func (f Functions) GetFunctionBySign(sign *FunctionSign) *Function {
	var process []FunctionProcess
	process = append(process, WithCheckProcessReturnLength(sign.ReturnLength))
	process = append(process, WithCheckProcessReturnType(sign.ReturnType))
	process = append(process, WithCheckProcessParamsType(sign.ParamsType))
	process = append(process, WithCheckProcessParamsLength(sign.ParamLength))
	process = append(process, WithHasEllipsis(sign.hasEllipsis))
	return f.GetFunctionByProcess(process)
}
func (f Functions) GetFunctionByProcess(processes []FunctionProcess) *Function {
	var _funcs = f
	for _, process := range processes {
		result := process(_funcs)
		if len(result) != 0 {
			_funcs = result
		} else {
			log.Warn("not found this function by function_sign")
		}
	}
	//todo： more function???
	return _funcs[0]
}
func (f Functions) Build() {
	for _, function := range f {
		function.Build()
	}
}

type FunctionSign struct {
	ReturnType   Types
	ParamsType   Types
	ParamLength  int
	ReturnLength int
	methodName   string
	hash         string
	FreeValues   map[string]Value // store the captured variable form parent-function, just contain name, and type is Parameter
	hasEllipsis  bool
}

func withProcessFunctions(process func(function *Function) bool) FunctionProcess {
	var returnFunctions Functions
	return func(functions Functions) Functions {
		for _, function := range functions {
			if process(function) {
				returnFunctions = append(returnFunctions, function)
			}
		}
		return returnFunctions
	}
}
func WithHasEllipsis(ellipsis bool) FunctionProcess {
	return withProcessFunctions(func(function *Function) bool {
		return function.hasEllipsis
	})
}

func WithCheckProcessReturnLength(length int) FunctionProcess {
	return withProcessFunctions(func(function *Function) bool {
		return function.ReturnLength == length
	})
}

func WithCheckProcessParamsLength(length int) FunctionProcess {
	return withProcessFunctions(func(function *Function) bool {
		return function.ParamLength == length
	})
}
func WithCheckProcessParamsType(types Types) FunctionProcess {
	/*
		todo: 类型不一样，比如blueprint，还需要去判断函数名称
	*/
	return withProcessFunctions(func(function *Function) bool {
		var flag = true
		if len(types) != len(function.ParamsType) {
			return false
		}
		for i, t := range types {
			if function.ParamsType[i].GetTypeKind() != t.GetTypeKind() {
				flag = false
				break
			}
		}
		return flag
	})
}
func WithCheckProcessReturnType(types Types) FunctionProcess {
	return withProcessFunctions(func(function *Function) bool {
		if len(types) < len(function.ReturnType) {
			//兜底
			return false
		}
		var flag = true
		for i, t := range types {
			if function.ReturnType[i].GetTypeKind() != t.GetTypeKind() {
				flag = false
				return flag
			}
			leftType, leftTypeExit := ToClassBluePrintType(t)
			rightType, rightTypeExit := ToClassBluePrintType(function.ReturnType[i])
			if leftTypeExit && rightTypeExit {
				leftType.IsParent(rightType)
			}
		}
		return flag
	})
}
