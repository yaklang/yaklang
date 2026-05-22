package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func orderBlocksForCompile(fn *ssa.Function) []int64 {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	visited := make(map[int64]struct{}, len(fn.Blocks))
	postorder := make([]int64, 0, len(fn.Blocks))

	var visit func(blockID int64)
	visit = func(blockID int64) {
		if blockID <= 0 {
			return
		}
		if _, ok := visited[blockID]; ok {
			return
		}
		visited[blockID] = struct{}{}

		blockVal, ok := fn.GetValueById(blockID)
		if !ok || blockVal == nil {
			return
		}
		bb, ok := ssa.ToBasicBlock(blockVal)
		if !ok || bb == nil {
			return
		}
		for _, succID := range bb.Succs {
			visit(succID)
		}
		postorder = append(postorder, blockID)
	}

	if fn.EnterBlock > 0 {
		visit(fn.EnterBlock)
	}

	for i, j := 0, len(postorder)-1; i < j; i, j = i+1, j-1 {
		postorder[i], postorder[j] = postorder[j], postorder[i]
	}

	for _, blockID := range fn.Blocks {
		if _, ok := visited[blockID]; ok {
			continue
		}
		postorder = append(postorder, blockID)
	}
	return postorder
}
