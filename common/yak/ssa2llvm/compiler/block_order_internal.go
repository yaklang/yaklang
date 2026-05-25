package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// collectFunctionBlockIDs returns all SSA blocks that belong to fn, including
// orphan merge blocks referenced by phis but missing from fn.Blocks.
func collectFunctionBlockIDs(fn *ssa.Function) []int64 {
	if fn == nil {
		return nil
	}

	seen := make(map[int64]struct{})
	order := make([]int64, 0)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		order = append(order, id)
	}

	for _, id := range fn.Blocks {
		add(id)
	}
	add(fn.EnterBlock)
	add(fn.DeferBlock)

	// Expand with blocks hosting phis referenced from known blocks.
	for i := 0; i < len(order); i++ {
		blockID := order[i]
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			continue
		}
		bb, ok := ssa.ToBasicBlock(blockVal)
		if !ok || bb == nil {
			continue
		}
		scan := append(append([]int64{}, bb.Phis...), bb.Insts...)
		for _, refID := range scan {
			addBlockForPhiValue(fn, refID, add)
		}
	}
	return order
}

func addBlockForPhiValue(fn *ssa.Function, id int64, add func(int64)) {
	if fn == nil || id <= 0 {
		return
	}
	val, ok := fn.GetValueById(id)
	if !ok || val == nil {
		return
	}
	if inst, ok := val.(ssa.Instruction); ok && inst.IsLazy() {
		if self := inst.Self(); self != nil {
			if materialized, ok := self.(ssa.Value); ok && materialized != nil {
				val = materialized
			}
		}
	}
	phi, ok := val.(*ssa.Phi)
	if !ok || phi == nil || phi.GetBlock() == nil {
		return
	}
	add(phi.GetBlock().GetId())
}

func orderBlocksForCompile(fn *ssa.Function) []int64 {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	blockIDs := collectFunctionBlockIDs(fn)
	visited := make(map[int64]struct{}, len(blockIDs))
	postorder := make([]int64, 0, len(blockIDs))

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

	for _, blockID := range blockIDs {
		if _, ok := visited[blockID]; ok {
			continue
		}
		postorder = append(postorder, blockID)
	}
	return postorder
}
