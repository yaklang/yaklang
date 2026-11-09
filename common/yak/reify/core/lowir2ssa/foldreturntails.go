package lowir2ssa

import "github.com/yaklang/yaklang/common/yak/reify/core"

// foldReturnTails replaces a jump to an acyclic return-only tail with an
// explicit return. The return operand is the current rax value, so a shared
// epilogue cannot overwrite the value of an earlier branch.
func foldReturnTails(function *core.Function) {
	if function == nil || len(function.Blocks) == 0 {
		return
	}
	blocks := make(map[core.BlockID]core.Block, len(function.Blocks))
	for _, block := range function.Blocks {
		blocks[block.ID] = block
	}
	returnLike := make(map[core.BlockID]bool)
	visiting := make(map[core.BlockID]bool)
	var isReturnLike func(core.BlockID) bool
	isReturnLike = func(id core.BlockID) bool {
		if result, ok := returnLike[id]; ok {
			return result
		}
		if visiting[id] {
			return false
		}
		block, ok := blocks[id]
		if !ok || len(block.Instructions) == 0 {
			return false
		}
		visiting[id] = true
		defer delete(visiting, id)
		last := block.Instructions[len(block.Instructions)-1]
		result := false
		switch last.Kind {
		case core.OpReturn:
			result = !blockHasReturningCall(block)
		case core.OpJump:
			if len(last.Targets) == 1 && !blockHasReturningCall(block) {
				result = isReturnLike(last.Targets[0])
			}
		case core.OpBranch:
			if len(last.Targets) == 2 {
				result = isReturnLike(last.Targets[0]) && isReturnLike(last.Targets[1])
			}
		}
		returnLike[id] = result
		return result
	}
	for index := range function.Blocks {
		block := &function.Blocks[index]
		if len(block.Instructions) == 0 {
			continue
		}
		last := block.Instructions[len(block.Instructions)-1]
		if last.Kind != core.OpJump || len(last.Targets) != 1 {
			continue
		}
		target := last.Targets[0]
		if !isReturnLike(target) || hasBackEdge(target, blocks) {
			continue
		}
		block.Instructions[len(block.Instructions)-1] = core.Instruction{
			Kind: core.OpReturn,
			Operands: []core.Operand{{
				Kind: core.OperandRegister,
				Name: "rax",
				Type: core.ValueType{Kind: core.TypeInteger, Bits: 64},
			}},
			Address: last.Address,
		}
	}
}

func blockHasReturningCall(block core.Block) bool {
	for _, instruction := range block.Instructions {
		if instruction.Kind != core.OpCall || len(instruction.Operands) == 0 {
			continue
		}
		operand := instruction.Operands[0]
		if operand.Kind == core.OperandSymbol && nonReturningCall(operand.Name) {
			continue
		}
		return true
	}
	return false
}

func nonReturningCall(name string) bool {
	switch name {
	case "__stack_chk_fail", "abort", "exit", "_exit", "__assert_fail", "__assert_perror_fail":
		return true
	default:
		return false
	}
}

func hasBackEdge(start core.BlockID, blocks map[core.BlockID]core.Block) bool {
	visiting := make(map[core.BlockID]bool)
	done := make(map[core.BlockID]bool)
	var visit func(core.BlockID) bool
	visit = func(id core.BlockID) bool {
		if id == start {
			return true
		}
		if visiting[id] || done[id] {
			return false
		}
		block, ok := blocks[id]
		if !ok || len(block.Instructions) == 0 {
			return false
		}
		visiting[id] = true
		defer delete(visiting, id)
		last := block.Instructions[len(block.Instructions)-1]
		for _, target := range last.Targets {
			if visit(target) {
				return true
			}
		}
		done[id] = true
		return false
	}
	block, ok := blocks[start]
	if !ok || len(block.Instructions) == 0 {
		return false
	}
	visiting[start] = true
	for _, target := range block.Instructions[len(block.Instructions)-1].Targets {
		if visit(target) {
			return true
		}
	}
	return false
}
