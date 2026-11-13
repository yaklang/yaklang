package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// duplicateReturnBlocks copies a shared epilogue into its unconditional
// predecessors. Without Phi, a value written on one branch would otherwise be
// hidden by the last write in linear order at the join.
func duplicateReturnBlocks(function *core.Function) {
	if function == nil {
		return
	}
	for {
		changed := false
		for returnIndex := range function.Blocks {
			block := &function.Blocks[returnIndex]
			if !isReturnEpilogue(*block) {
				continue
			}
			predecessors := jumpPredecessors(function, block.ID)
			if len(predecessors) < 2 {
				continue
			}
			epilogue := append([]core.Instruction(nil), block.Instructions...)
			for _, predecessor := range predecessors {
				instructions := predecessor.Instructions
				predecessor.Instructions = append(instructions[:len(instructions)-1], epilogue...)
			}
			function.Blocks = append(function.Blocks[:returnIndex], function.Blocks[returnIndex+1:]...)
			changed = true
			break
		}
		if !changed {
			return
		}
	}
}

func isReturnEpilogue(block core.Block) bool {
	if len(block.Instructions) == 0 {
		return false
	}
	if block.Instructions[len(block.Instructions)-1].Kind != core.OpReturn {
		return false
	}
	for _, instruction := range block.Instructions {
		switch instruction.Kind {
		case core.OpCall, core.OpStore, core.OpBranch, core.OpJump, core.OpTrap:
			return false
		}
	}
	return true
}

func jumpPredecessors(function *core.Function, target core.BlockID) []*core.Block {
	var result []*core.Block
	for index := range function.Blocks {
		block := &function.Blocks[index]
		last := block.Instructions[len(block.Instructions)-1]
		if last.Kind != core.OpJump || len(last.Targets) != 1 || last.Targets[0] != target {
			continue
		}
		result = append(result, block)
	}
	return result
}

// DuplicateReturnBlocks is the exported pass entry point.
func DuplicateReturnBlocks(function *core.Function) { duplicateReturnBlocks(function) }
