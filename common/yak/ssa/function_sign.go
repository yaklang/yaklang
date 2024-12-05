package ssa

import "github.com/yaklang/yaklang/common/log"

type Functions []*Function
type FunctionProcess func(functions Functions) Functions

func (f Functions) SearchFunctionWithProcess(process []FunctionProcess) *Function {
	var funcs = f
	var _funcs Functions
	for _, f2 := range process {
		_funcs = f2(funcs)
		if len(_funcs) != 0 {
			funcs = _funcs
		} else {
			log.Warn("not found this function by function_sign")
		}
	}
	//todo： more function???
	return funcs[0]
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
}

func withProcessFunctions(process func(function *Function) bool) FunctionProcess {
	var returnFunctions Functions
	return func(functions Functions) Functions {
		for _, function := range functions {
			if process(function) {
				returnFunctions = append(functions, function)
			}
		}
		return returnFunctions
	}
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
		for i, t := range types {
			if function.ParamsType[i].GetTypeKind() != t.GetTypeKind() {
				flag = false
			}
		}
		return flag
	})
}
func WithCheckProcessReturnType(types Types) FunctionProcess {
	return withProcessFunctions(func(function *Function) bool {
		var flag = true
		for i, t := range types {
			if function.ParamsType[i].GetTypeKind() != t.GetTypeKind() {
				flag = false
			}
		}
		return flag
	})
}
