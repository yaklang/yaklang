// Package ssapass applies typed C function definitions and value type
// refinements to a YakSSA program after LowIR conversion.
package ssapass

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/funcdefs"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// ApplyFunctionDefine sets the return type and known argument types of calls
// whose target matches a C definition. It returns the number of matched calls.
func ApplyFunctionDefine(program *ssa.Program) int {
	matched := 0
	eachCall(program, func(_ *ssa.Function, call *ssa.Call) {
		definition, ok := definitionForCall(call)
		if !ok {
			return
		}
		matched++
		call.SetType(ssaType(definition.Return))
		if !definition.Variadic && len(call.Args) > len(definition.Params) {
			call.Args = call.Args[:len(definition.Params)]
		}
		for index, argID := range call.Args {
			if index >= len(definition.Params) {
				break
			}
			param := definition.Params[index]
			if param.Type.IsVoid() {
				continue
			}
			value, ok := call.GetValueById(argID)
			if !ok || value == nil {
				continue
			}
			value.SetType(ssaType(param.Type))
			if parameter, ok := ssa.ToParameter(value); ok && parameter != nil && param.Type.IsString() && param.Name != "" {
				parameter.SetName(param.Name)
			}
		}
	})
	return matched
}

func definitionForCall(call *ssa.Call) (funcdefs.Definition, bool) {
	if call == nil {
		return funcdefs.Definition{}, false
	}
	value, ok := call.GetValueById(call.Method)
	if !ok || value == nil {
		return funcdefs.Definition{}, false
	}
	function, ok := ssa.ToFunction(value)
	if !ok || function == nil {
		return funcdefs.Definition{}, false
	}
	return funcdefs.Lookup(function.GetName())
}

func ssaType(typ funcdefs.Type) ssa.Type {
	switch typ.Kind {
	case funcdefs.Void:
		return ssa.CreateNullType()
	case funcdefs.String:
		return ssa.CreateStringType()
	case funcdefs.Pointer, funcdefs.Struct:
		return ssa.NewBasicType(ssa.NumberTypeKind, "ptr64")
	case funcdefs.Int:
		if typ.Bits == 64 {
			return ssa.NewBasicType(ssa.NumberTypeKind, "int64")
		}
		return ssa.NewBasicType(ssa.NumberTypeKind, "int32")
	case funcdefs.UInt:
		if typ.Bits == 64 {
			return ssa.NewBasicType(ssa.NumberTypeKind, "uint64")
		}
		return ssa.NewBasicType(ssa.NumberTypeKind, "uint32")
	case funcdefs.Size:
		return ssa.NewBasicType(ssa.NumberTypeKind, "size_t")
	case funcdefs.Float:
		if typ.Bits == 32 {
			return ssa.NewBasicType(ssa.NumberTypeKind, "f32")
		}
		return ssa.NewBasicType(ssa.NumberTypeKind, "f64")
	default:
		return ssa.CreateAnyType()
	}
}

func eachCall(program *ssa.Program, visit func(*ssa.Function, *ssa.Call)) {
	if program == nil {
		return
	}
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			instructionIDs := make([]int64, 0, len(block.Insts)+len(block.Phis))
			instructionIDs = append(instructionIDs, block.Insts...)
			instructionIDs = append(instructionIDs, block.Phis...)
			for _, instructionID := range instructionIDs {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if call, ok := ssa.ToCall(instruction); ok && call != nil {
					visit(function, call)
				}
			}
		}
		return true
	})
}
