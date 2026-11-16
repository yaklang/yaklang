package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// DropUnreachable removes the blocks the entry cannot reach. An unconditional
// branch leaves the instruction that follows it without a predecessor, and that
// block still holds lifted code: keeping it renders statements no execution
// reaches, and the registers it reads come from a path that never ran.
func DropUnreachable(function *core.Function) {
	if function == nil || len(function.Blocks) == 0 {
		return
	}
	at := make(map[core.BlockID]bool, len(function.Blocks))
	for _, block := range function.Blocks {
		at[block.ID] = true
	}
	reachable := make(map[core.BlockID]bool, len(function.Blocks))
	queue := []core.BlockID{function.Blocks[0].ID}
	reachable[queue[0]] = true
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		block, ok := blockByID(function, id)
		if !ok {
			continue
		}
		for _, target := range blockTargets(block) {
			if reachable[target] || !at[target] {
				continue
			}
			reachable[target] = true
			queue = append(queue, target)
		}
	}
	kept := function.Blocks[:0]
	for _, block := range function.Blocks {
		if !reachable[block.ID] {
			continue
		}
		kept = append(kept, block)
	}
	function.Blocks = kept
	for index := range function.Blocks {
		block := &function.Blocks[index]
		if len(block.Instructions) == 0 {
			continue
		}
		last := &block.Instructions[len(block.Instructions)-1]
		if len(last.Targets) == 0 {
			continue
		}
		targets := last.Targets[:0]
		for _, target := range last.Targets {
			if reachable[target] {
				targets = append(targets, target)
			}
		}
		last.Targets = targets
		if last.Kind == core.OpBranch && len(last.Targets) == 1 {
			last.Kind = core.OpJump
			last.Operands = nil
		}
	}
}

func blockByID(function *core.Function, id core.BlockID) (core.Block, bool) {
	for _, block := range function.Blocks {
		if block.ID == id {
			return block, true
		}
	}
	return core.Block{}, false
}

func blockTargets(block core.Block) []core.BlockID {
	if len(block.Instructions) == 0 {
		return nil
	}
	return block.Instructions[len(block.Instructions)-1].Targets
}
