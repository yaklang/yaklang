package pass

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type inferFuncCache struct {
	name   string
	vals   []ssa.Value
	rets   []*ssa.Return
	active bool
}

func collectInferFunc(function *ssa.Function) inferFuncCache {
	cf := inferFuncCache{name: function.GetName(), active: true}
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, id := range block.Phis {
			if instruction, ok := function.GetInstructionById(id); ok && instruction != nil {
				if value, ok := ssa.ToValue(instruction); ok && value != nil {
					cf.vals = append(cf.vals, value)
				}
			}
		}
		for _, id := range block.Insts {
			if instruction, ok := function.GetInstructionById(id); ok && instruction != nil {
				if ret, ok := instruction.(*ssa.Return); ok && ret != nil {
					cf.rets = append(cf.rets, ret)
				}
				if value, ok := ssa.ToValue(instruction); ok && value != nil {
					cf.vals = append(cf.vals, value)
				}
			}
		}
	}
	return cf
}

func cachedReturnsPointer(rets []*ssa.Return) bool {
	for _, ret := range rets {
		for _, item := range ret.GetValues() {
			if isPointerType(item.GetType()) {
				return true
			}
		}
	}
	return false
}

func markCachedReturnNulls(rets []*ssa.Return) bool {
	changed := false
	for _, ret := range rets {
		for _, item := range ret.GetValues() {
			if setPointer(item) {
				changed = true
			}
		}
	}
	return changed
}

func runInferRounds(funcs []inferFuncCache, localPtrs map[string]bool, sharedReturnsPtr func(string) bool) int {
	if len(funcs) == 0 {
		return 0
	}
	changed := 0
	for round := 0; round < 12; round++ {
		roundChanged := false
		ptrSetGrew := false
		for i := range funcs {
			cf := &funcs[i]
			if !cf.active {
				continue
			}
			fnChanged := false
			isPtr := localPtrs[cf.name] || (sharedReturnsPtr != nil && sharedReturnsPtr(cf.name))
			if !isPtr && cachedReturnsPointer(cf.rets) {
				localPtrs[cf.name] = true
				isPtr = true
				ptrSetGrew = true
				fnChanged = true
			}
			if isPtr && markCachedReturnNulls(cf.rets) {
				fnChanged = true
			}
			for _, value := range cf.vals {
				if inferValueWith(value, localPtrs, sharedReturnsPtr) {
					fnChanged = true
					changed++
				}
			}
			cf.active = fnChanged
			if fnChanged {
				roundChanged = true
			}
		}
		if !roundChanged {
			break
		}
		if ptrSetGrew {
			for i := range funcs {
				funcs[i].active = true
			}
		}
	}
	return changed
}

// InferTypes propagates pointer types from known C signatures and from
// pointer arithmetic, phi nodes, and calls to functions whose return type is
// already known. It intentionally does not guess struct layouts.
func InferTypes(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	var funcs []inferFuncCache
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function != nil && !function.IsExtern() {
			funcs = append(funcs, collectInferFunc(function))
		}
		return true
	})
	returnPointer := make(map[string]bool, len(funcs))
	return runInferRounds(funcs, returnPointer, nil)
}

func inferValueWith(value ssa.Value, localPtrs map[string]bool, sharedReturnsPtr func(string) bool) bool {
	changed := false
	switch typed := value.(type) {
	case *ssa.Call:
		if typed == nil {
			return false
		}
		name := callName(typed)
		definition, known := symbol.Lookup(name)
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
		} else if (localPtrs[name] || (sharedReturnsPtr != nil && sharedReturnsPtr(name))) && setPointer(typed) {
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
		if typed.Op == ssa.OpStore {
			// The address a write goes through is a pointer, and a stored value
			// that has no type yet takes the type the store declares.
			if setPointerByID(typed, typed.X) {
				changed = true
			}
			stored := valueByID(typed, typed.Y)
			if stored != nil && typed.GetType() != nil && stored.GetType() != nil && stored.GetType().String() == "any" {
				stored.SetType(typed.GetType())
				changed = true
			}
		}
	case *ssa.UnOp:
		if typed == nil {
			return false
		}
		if typed.Op == ssa.OpLoad || typed.Op == ssa.OpSpill {
			// The operand of a memory read is the address it dereferences.
			if setPointerByID(typed, typed.X) {
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
	}
	return changed
}

// valueByID resolves one operand of an instruction, or nil when it is gone.
func valueByID(instruction ssa.Instruction, id int64) ssa.Value {
	value, ok := instruction.GetValueById(id)
	if !ok {
		return nil
	}
	return value
}

func setPointerByID(instruction ssa.Instruction, id int64) bool {
	value := valueByID(instruction, id)
	if value == nil || isPointerType(value.GetType()) {
		return false
	}
	return setPointer(value)
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
