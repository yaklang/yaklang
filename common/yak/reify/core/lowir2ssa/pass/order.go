package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// reorderReversePostorder visits loop headers before their bodies. That lets a
// loop phi be created with its preheader value and completed when the back edge
// is emitted.
func reorderReversePostorder(function *core.Function) {
	if function == nil || len(function.Blocks) == 0 {
		return
	}
	index := make(map[core.BlockID]int, len(function.Blocks))
	for position, block := range function.Blocks {
		index[block.ID] = position
	}
	successors := make(map[int][]int, len(function.Blocks))
	for position, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind != core.OpJump && instruction.Kind != core.OpBranch {
				continue
			}
			targets := append([]core.BlockID(nil), instruction.Targets...)
			if instruction.Kind == core.OpBranch {
				for left, right := 0, len(targets)-1; left < right; left, right = left+1, right-1 {
					targets[left], targets[right] = targets[right], targets[left]
				}
			}
			for _, target := range targets {
				if next, ok := index[target]; ok {
					successors[position] = append(successors[position], next)
				}
			}
		}
	}
	visited := make([]bool, len(function.Blocks))
	postorder := make([]int, 0, len(function.Blocks))
	var visit func(int)
	visit = func(position int) {
		if visited[position] {
			return
		}
		visited[position] = true
		for _, next := range successors[position] {
			visit(next)
		}
		postorder = append(postorder, position)
	}
	visit(0)
	blocks := make([]core.Block, 0, len(function.Blocks))
	for index := len(postorder) - 1; index >= 0; index-- {
		blocks = append(blocks, function.Blocks[postorder[index]])
	}
	for position := range function.Blocks {
		if !visited[position] {
			blocks = append(blocks, function.Blocks[position])
		}
	}
	function.Blocks = blocks
}

// ReorderReversePostorder is the exported pass entry point.
func ReorderReversePostorder(function *core.Function) { reorderReversePostorder(function) }
