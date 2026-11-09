package ssapass

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core/funcdefs"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// InferTypes propagates pointer types from known C signatures and from
// pointer arithmetic, phi nodes, and calls to functions whose return type is
// already known. It intentionally does not guess struct layouts.
func InferTypes(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	changed := 0
	returnPointer := make(map[string]bool)
	for round := 0; round < 12; round++ {
		roundChanged := false
		program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
			if function == nil || function.IsExtern() {
				return true
			}
			if !returnPointer[function.GetName()] && functionReturnsPointer(function) {
				returnPointer[function.GetName()] = true
				roundChanged = true
			}
			if returnPointer[function.GetName()] && markReturnNulls(function) {
				roundChanged = true
			}
			eachFunctionValue(function, func(value ssa.Value) {
				if inferValue(value, returnPointer) {
					roundChanged = true
					changed++
				}
			})
			return true
		})
		if !roundChanged {
			break
		}
	}
	return changed
}

func markReturnNulls(function *ssa.Function) bool {
	changed := false
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, id := range block.Insts {
			instruction, ok := function.GetInstructionById(id)
			if !ok || instruction == nil {
				continue
			}
			ret, ok := instruction.(*ssa.Return)
			if !ok || ret == nil {
				continue
			}
			for _, item := range ret.GetValues() {
				if setPointer(item) {
					changed = true
				}
			}
		}
	}
	return changed
}

func eachFunctionValue(function *ssa.Function, visit func(ssa.Value)) {
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		ids := make([]int64, 0, len(block.Phis)+len(block.Insts))
		ids = append(ids, block.Phis...)
		ids = append(ids, block.Insts...)
		for _, id := range ids {
			instruction, ok := function.GetInstructionById(id)
			if !ok || instruction == nil {
				continue
			}
			value, ok := ssa.ToValue(instruction)
			if ok && value != nil {
				visit(value)
			}
		}
	}
}

func functionReturnsPointer(function *ssa.Function) bool {
	pointer := false
	eachFunctionValue(function, func(value ssa.Value) {
		instruction, ok := value.(ssa.Instruction)
		if !ok {
			return
		}
		ret, ok := instruction.(*ssa.Return)
		if !ok || ret == nil {
			return
		}
		for _, item := range ret.GetValues() {
			if isPointerType(item.GetType()) {
				pointer = true
			}
		}
	})
	return pointer
}

func inferValue(value ssa.Value, returnPointer map[string]bool) bool {
	changed := false
	switch typed := value.(type) {
	case *ssa.Call:
		if typed == nil {
			return false
		}
		name := callName(typed)
		definition, known := funcdefs.Lookup(name)
		if known {
			if definition.Return.IsPointer() && setPointer(typed) {
				changed = true
			}
			args := typed.GetValuesByIDs(typed.Args)
			for index, arg := range args {
				if index >= len(definition.Params) {
					break
				}
				if definition.Params[index].Type.IsPointer() && setPointer(arg) {
					changed = true
				}
			}
		} else if returnPointer[name] && setPointer(typed) {
			changed = true
		}
	case *ssa.BinOp:
		if typed == nil {
			return false
		}
		if typed.Op == ssa.OpAdd || typed.Op == ssa.OpSub {
			if isPointerValueByID(typed, typed.X) || isPointerValueByID(typed, typed.Y) {
				if setPointer(typed) {
					changed = true
				}
			}
		}
		if typed.Op == ssa.OpEq || typed.Op == ssa.OpNotEq {
			if isPointerValueByID(typed, typed.X) && setZeroPointerByID(typed, typed.Y) {
				changed = true
			}
			if isPointerValueByID(typed, typed.Y) && setZeroPointerByID(typed, typed.X) {
				changed = true
			}
		}
	case *ssa.Phi:
		if typed == nil {
			return false
		}
		values := typed.GetValues()
		pointer := isPointerType(typed.GetType())
		for _, item := range values {
			if isPointerType(item.GetType()) {
				pointer = true
			}
		}
		if pointer {
			if setPointer(typed) {
				changed = true
			}
			for _, item := range values {
				if setPointer(item) {
					changed = true
				}
			}
		}
	case *ssa.TypeCast:
		if typed == nil {
			return false
		}
		if isPointerType(typed.GetType()) {
			if setPointer(typed) {
				changed = true
			}
			if inner, ok := typed.GetValueById(typed.Value); ok && inner != nil && setPointer(inner) {
				changed = true
			}
		}
		if address := loadMarkerAddress(typed); address != nil {
			if setPointer(address) {
				changed = true
			}
		}
	}
	return changed
}

func loadMarkerAddress(cast *ssa.TypeCast) ssa.Value {
	if cast == nil {
		return nil
	}
	value, ok := cast.GetValueById(cast.Value)
	if !ok || value == nil {
		return nil
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return nil
	}
	marker, ok := constant.GetRawValue().(string)
	if !ok || !strings.HasPrefix(marker, "load:") {
		return nil
	}
	var id int64
	if _, err := fmt.Sscanf(strings.TrimPrefix(marker, "load:"), "%d", &id); err != nil {
		return nil
	}
	address, ok := cast.GetValueById(id)
	if !ok || address == nil {
		return nil
	}
	return address
}

func callName(call *ssa.Call) string {
	if call == nil {
		return ""
	}
	value, ok := call.GetValueById(call.Method)
	if !ok || value == nil {
		return ""
	}
	return value.GetName()
}

func isPointerValueByID(instruction ssa.Instruction, id int64) bool {
	value, ok := instruction.GetValueById(id)
	return ok && value != nil && isPointerType(value.GetType())
}

func setZeroPointerByID(instruction ssa.Instruction, id int64) bool {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return false
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil || !isZeroConstant(constant) {
		return false
	}
	return setPointer(constant)
}

func setPointer(value ssa.Value) bool {
	if value == nil || isPointerType(value.GetType()) {
		return false
	}
	value.SetType(ssa.NewBasicType(ssa.NumberTypeKind, "ptr64"))
	if phi, ok := ssa.ToPhi(value); ok && phi != nil {
		for _, item := range phi.GetValues() {
			setPointer(item)
		}
	}
	return true
}

func isPointerType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return strings.Contains(name, "ptr") || strings.Contains(name, "*")
}

func isZeroConstant(constant *ssa.ConstInst) bool {
	if constant == nil || constant.Const == nil {
		return false
	}
	switch value := constant.GetRawValue().(type) {
	case int:
		return value == 0
	case int64:
		return value == 0
	case uint64:
		return value == 0
	case uint32:
		return value == 0
	default:
		return false
	}
}
