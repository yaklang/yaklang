package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// foldReturnTails replaces a jump to an acyclic return-only tail with an
// explicit return. The return operand is the current value of the return
// register the architecture declares, so a shared epilogue cannot overwrite the
// value of an earlier branch.
func foldReturnTails(function *core.Function, facts core.ArchFacts) {
	if function == nil || len(function.Blocks) == 0 {
		return
	}
	// The return register belongs to the architecture. Without it the fold has no
	// operand to read, and the tail stays in place.
	returnRegister := facts.Registers.Return
	if returnRegister == "" {
		return
	}
	returnBits := facts.Registers.Width
	if returnBits == 0 {
		returnBits = 64
	}
	blocks := make(map[core.BlockID]core.Block, len(function.Blocks))
	for _, block := range function.Blocks {
		blocks[block.ID] = block
	}
	returnLike := make(map[core.BlockID]bool)
	visiting := make(map[core.BlockID]bool)
	returnView := registerViewFor(facts, returnRegister, returnBits)
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
		// A tail that writes the return register is not a return tail: the write
		// would be skipped and the function would return an earlier value. Clang
		// -O0 emits such a shared epilogue (load the result slot into x0/w0), and
		// folding it reports the value of an argument instead of the result.
		switch last.Kind {
		case core.OpReturn:
			result = !blockWritesReturnRegister(block, facts, returnView) && !blockHasReturningCall(block)
		case core.OpJump:
			if len(last.Targets) == 1 && !blockWritesReturnRegister(block, facts, returnView) && !blockHasReturningCall(block) {
				result = isReturnLike(last.Targets[0])
			}
		case core.OpBranch:
			// A branch is only return-like when both arms reach a return without
			// calling a function that may change the return value.
			if len(last.Targets) == 2 && !blockWritesReturnRegister(block, facts, returnView) && !blockHasReturningCall(block) {
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
				Name: returnRegister,
				Type: core.ValueType{Kind: core.TypeInteger, Bits: returnBits},
			}},
			Address: last.Address,
		}
	}
}

// blockWritesReturnRegister reports whether a block writes the value the
// function returns. The write can name a narrow view of the register, so both
// sides are canonicalized first.
func blockWritesReturnRegister(block core.Block, facts core.ArchFacts, returnView registerView) bool {
	if returnView.canonical == "" {
		return false
	}
	for _, instruction := range block.Instructions {
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		operand := instruction.Operands[0]
		if operand.Kind != core.OperandRegister {
			continue
		}
		if registerViewFor(facts, operand.Name, operand.Type.Bits).canonical == returnView.canonical {
			return true
		}
	}
	return false
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

// FoldReturnTails is the exported pass entry point.
func FoldReturnTails(function *core.Function, facts core.ArchFacts) { foldReturnTails(function, facts) }
