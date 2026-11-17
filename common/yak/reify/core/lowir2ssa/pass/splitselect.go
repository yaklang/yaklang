package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// SplitSelect rewrites a conditional move into the control flow it stands for: the
// condition ends the block, each arm carries one candidate, and a merge block holds
// a Phi that names both. A conditional move is a data-flow choice, and the SSA
// conversion has no instruction for it, so the choice has to exist as control flow
// before the conversion runs.
func SplitSelect(function *core.Function) {
	if function == nil {
		return
	}
	for {
		rewritten := false
		next := core.BlockID(0)
		for _, block := range function.Blocks {
			if block.ID > next {
				next = block.ID
			}
		}
		blocks := make([]core.Block, 0, len(function.Blocks))
		for _, block := range function.Blocks {
			head, arms, ok := splitOneSelect(block, next)
			if !ok {
				blocks = append(blocks, block)
				continue
			}
			rewritten = true
			next += 3
			blocks = append(blocks, head)
			blocks = append(blocks, arms...)
		}
		function.Blocks = blocks
		if !rewritten {
			return
		}
	}
}

// splitOneSelect splits one select out of a block. The block keeps its identity so
// incoming edges still enter the condition, and the instructions after the select
// continue in the merge block.
func splitOneSelect(block core.Block, lastID core.BlockID) (core.Block, []core.Block, bool) {
	index := -1
	for at, instruction := range block.Instructions {
		if instruction.Kind == core.OpSelect {
			index = at
			break
		}
	}
	if index < 0 {
		return block, nil, false
	}
	selected := block.Instructions[index]
	if len(selected.Operands) < 3 || len(selected.Results) != 1 {
		return block, nil, false
	}
	condition, whenTrue, whenFalse := selected.Operands[0], selected.Operands[1], selected.Operands[2]
	trueID, falseID, joinID := lastID+1, lastID+2, lastID+3

	head := core.Block{ID: block.ID, Address: block.Address}
	head.Instructions = append(head.Instructions, block.Instructions[:index]...)
	head.Instructions = append(head.Instructions, core.Instruction{
		Kind:     core.OpBranch,
		Operands: []core.Operand{condition},
		Targets:  []core.BlockID{trueID, falseID},
		Address:  selected.Address,
	})
	arm := func(id core.BlockID) core.Block {
		return core.Block{ID: id, Address: selected.Address, Instructions: []core.Instruction{{
			Kind:    core.OpJump,
			Targets: []core.BlockID{joinID},
			Address: selected.Address,
		}}}
	}
	join := core.Block{ID: joinID, Address: selected.Address}
	join.Instructions = append(join.Instructions, core.Instruction{
		Kind:     core.OpPhi,
		Operands: []core.Operand{whenTrue, whenFalse},
		Results:  selected.Results,
		Address:  selected.Address,
	})
	join.Instructions = append(join.Instructions, block.Instructions[index+1:]...)
	return head, []core.Block{arm(trueID), arm(falseID), join}, true
}
