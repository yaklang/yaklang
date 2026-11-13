package pass

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// RefineReturnTypes publishes the return types implied by the SSA Return
// values. It runs after InferTypes and MarkVoidReturns so callers and the SSAIR
// signature see the same type as the function body.
func RefineReturnTypes(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	returnTypes := make(map[string]ssa.Type)
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		if _, known := symbol.Lookup(name); known {
			return true
		}
		returns := functionReturns(function)
		if len(returns) == 0 {
			returnTypes[name] = ssa.CreateNullType()
			return true
		}
		allPointer := true
		allInteger := true
		hasValue := false
		for _, ret := range returns {
			values := ret.GetValues()
			if len(values) == 0 {
				continue
			}
			hasValue = true
			for _, value := range values {
				typ := value.GetType()
				if !isPointerSSAType(typ) {
					allPointer = false
				}
				if typ == nil || !isIntegerSSAType(typ) {
					allInteger = false
				}
			}
		}
		switch {
		case !hasValue:
			returnTypes[name] = ssa.CreateNullType()
		case allPointer:
			returnTypes[name] = ssa.NewBasicType(ssa.NumberTypeKind, "ptr64")
		case allInteger:
			returnTypes[name] = ssa.NewBasicType(ssa.NumberTypeKind, "i64")
		default:
			returnTypes[name] = ssa.CreateAnyType()
		}
		return true
	})
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		if typ, ok := returnTypes[name]; ok && function.Type != nil {
			function.Type.ReturnType = typ
		}
		return true
	})
	eachCall(program, func(_ *ssa.Function, call *ssa.Call) {
		if typ, ok := returnTypes[internalCallName(call)]; ok {
			call.SetType(typ)
		}
	})
	return len(returnTypes)
}

func isPointerSSAType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return name == "ptr64" || name == "ptr32" || name == "void *" || name == "char *" ||
		len(name) > 0 && name[len(name)-1] == '*'
}

func isIntegerSSAType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	switch name {
	case "i1", "i8", "i16", "i32", "i64", "int8", "int16", "int32", "int64",
		"uint8", "uint16", "uint32", "uint64", "size_t":
		return true
	default:
		return false
	}
}
