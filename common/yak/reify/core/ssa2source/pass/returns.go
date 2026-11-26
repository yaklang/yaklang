package pass

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// MarkVoidReturns turns internal functions whose return value is not
// observable into void functions. Explicit scalar/pointer returns are kept;
// only an unused result produced by a load marker is treated as a spilled
// register with no source-level return value.
func MarkVoidReturns(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	usedResults := make(map[string]bool)
	eachCall(program, func(_ *ssa.Function, call *ssa.Call) {
		name := internalCallName(call)
		if name != "" && call.HasUsers() {
			usedResults[name] = true
		}
	})
	voidFunctions := make(map[string]bool)
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() || function.GetName() == "" {
			return true
		}
		if _, known := symbol.Lookup(name); known {
			return true
		}
		returns := functionReturns(function)
		if len(returns) == 0 {
			voidFunctions[name] = true
			return true
		}
		weak := true
		for _, ret := range returns {
			values := ret.GetValues()
			if len(values) == 0 {
				continue
			}
			for _, value := range values {
				if !weakReturnValue(value) {
					weak = false
				}
			}
		}
		if weak && !usedResults[name] {
			voidFunctions[name] = true
		}
		return true
	})
	if len(voidFunctions) == 0 {
		return 0
	}
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		if voidFunctions[name] {
			for _, id := range function.Blocks {
				block, ok := function.GetBasicBlockByID(id)
				if !ok || block == nil {
					continue
				}
				for _, instructionID := range block.Insts {
					instruction, ok := function.GetInstructionById(instructionID)
					if !ok || instruction == nil {
						continue
					}
					ret, ok := instruction.(*ssa.Return)
					if ok && ret != nil {
						ret.Results = nil
					}
				}
			}
			if function.Type != nil {
				function.Type.ReturnType = ssa.CreateNullType()
			}
		}
		return true
	})
	eachCall(program, func(_ *ssa.Function, call *ssa.Call) {
		if voidFunctions[internalCallName(call)] {
			call.SetType(ssa.CreateNullType())
		}
	})
	return len(voidFunctions)
}

func functionReturns(function *ssa.Function) []*ssa.Return {
	result := make([]*ssa.Return, 0)
	if function == nil {
		return result
	}
	for _, id := range function.Blocks {
		block, ok := function.GetBasicBlockByID(id)
		if !ok || block == nil {
			continue
		}
		for _, instructionID := range block.Insts {
			instruction, ok := function.GetInstructionById(instructionID)
			if !ok || instruction == nil {
				continue
			}
			if ret, ok := instruction.(*ssa.Return); ok && ret != nil {
				result = append(result, ret)
			}
		}
	}
	return result
}

func weakReturnValue(value ssa.Value) bool {
	if value == nil {
		return true
	}
	for depth := 0; depth < 8; depth++ {
		if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
			next, ok := cast.GetValueById(cast.Value)
			if !ok || next == nil {
				return true
			}
			value = next
			continue
		}
		break
	}
	if _, ok := ssa.ToUndefined(value); ok {
		return true
	}
	// A spill reads the current frame, which is the lifter's own scratch space.
	// The value never reached memory the caller can see, so it is not a
	// source-level return value. A load through a pointer is different: it reads
	// program memory and stands for the dereference the source wrote.
	if read, ok := ssa.ToUnOp(value); ok && read != nil {
		return read.Op == ssa.OpSpill
	}
	return false
}
