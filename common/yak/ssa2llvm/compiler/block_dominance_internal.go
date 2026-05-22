package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func computeBlockDominators(fn *ssa.Function) map[int64]map[int64]struct{} {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	blockSet := make(map[int64]struct{}, len(fn.Blocks))
	for _, blockID := range fn.Blocks {
		blockSet[blockID] = struct{}{}
	}

	dom := make(map[int64]map[int64]struct{}, len(fn.Blocks))
	for _, blockID := range fn.Blocks {
		dom[blockID] = cloneBlockSet(blockSet)
	}
	if fn.EnterBlock > 0 {
		dom[fn.EnterBlock] = map[int64]struct{}{fn.EnterBlock: {}}
	}

	changed := true
	for changed {
		changed = false
		for _, blockID := range fn.Blocks {
			preds := predecessorBlockIDs(fn, blockID)
			if len(preds) == 0 {
				continue
			}
			next := cloneBlockSet(dom[preds[0]])
			for _, predID := range preds[1:] {
				next = intersectBlockSets(next, dom[predID])
			}
			next[blockID] = struct{}{}
			if !blockSetsEqual(next, dom[blockID]) {
				dom[blockID] = next
				changed = true
			}
		}
	}
	return dom
}

func predecessorBlockIDs(fn *ssa.Function, blockID int64) []int64 {
	if fn == nil || blockID <= 0 {
		return nil
	}
	var preds []int64
	for _, fromID := range fn.Blocks {
		fromVal, ok := fn.GetValueById(fromID)
		if !ok {
			continue
		}
		fromBB, ok := ssa.ToBasicBlock(fromVal)
		if !ok || fromBB == nil {
			continue
		}
		for _, succID := range fromBB.Succs {
			if succID == blockID {
				preds = append(preds, fromID)
				break
			}
		}
	}
	return preds
}

func blockDominates(dom map[int64]map[int64]struct{}, defBlockID, useBlockID int64) bool {
	if defBlockID <= 0 || useBlockID <= 0 {
		return defBlockID == useBlockID
	}
	if defBlockID == useBlockID {
		return true
	}
	useDom, ok := dom[useBlockID]
	if !ok {
		return false
	}
	_, ok = useDom[defBlockID]
	return ok
}

func cloneBlockSet(in map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func intersectBlockSets(a, b map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func blockSetsEqual(a, b map[int64]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
