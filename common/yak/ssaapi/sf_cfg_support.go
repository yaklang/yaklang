package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func getFunctionByID(p *Program, funcID int64) (*ssa.Function, error) {
	if p == nil || p.Program == nil || funcID <= 0 {
		return nil, utils.Error("invalid program/function")
	}
	ins, ok := p.Program.GetInstructionById(funcID)
	if !ok || ins == nil {
		return nil, utils.Errorf("function id %d not found", funcID)
	}
	fn, ok := ssa.ToFunction(ins)
	if !ok || fn == nil {
		return nil, utils.Errorf("instruction %d is not a function", funcID)
	}
	return fn, nil
}

func getBlockByID(fn *ssa.Function, blockID int64) (*ssa.BasicBlock, error) {
	if fn == nil || blockID <= 0 {
		return nil, utils.Error("invalid function/block")
	}
	b, ok := fn.GetBasicBlockByID(blockID)
	if !ok || b == nil {
		return nil, utils.Errorf("block id %d not found", blockID)
	}
	return b, nil
}

// isExitLikeBlock reports whether a basic block should be treated as a function-exit terminator
// for stage-1/2 CFG queries. This is intentionally conservative.
func isExitLikeBlock(fn *ssa.Function, blockID int64) bool {
	if fn == nil || blockID <= 0 {
		return false
	}
	blk, ok := fn.GetBasicBlockByID(blockID)
	if !ok || blk == nil {
		return false
	}
	if fn.ExitBlock > 0 && blockID == fn.ExitBlock {
		return true
	}
	// No successors usually means return-like termination in Yak SSA CFG.
	if len(blk.Succs) == 0 {
		return true
	}
	// Explicit return (Yak lowering may not place Return as the sole LastInst).
	if last := blk.LastInst(); last != nil {
		if _, ok := ssa.ToReturn(last); ok {
			return true
		}
	}
	for _, iid := range blk.Insts {
		ins, ok := blk.GetInstructionById(iid)
		if !ok || ins == nil {
			continue
		}
		if _, ok := ssa.ToReturn(ins); ok {
			return true
		}
	}
	// Straight-line lowering: jump / fall-through to the unique exit block.
	if fn.ExitBlock > 0 && len(blk.Succs) == 1 && blk.Succs[0] == fn.ExitBlock {
		return true
	}
	if last := blk.LastInst(); last != nil {
		if j, ok := ssa.ToJump(last); ok && j != nil && fn.ExitBlock > 0 && j.To == fn.ExitBlock {
			return true
		}
	}
	return false
}

// cfgGuardsLoopLatchFromLoop returns the latch block id for a for-loop (body fall-through target),
// used to recognize continue (jump-to-latch) vs break (jump-to-loop.Exit).
func cfgGuardsLoopLatchFromLoop(fn *ssa.Function, l *ssa.Loop) int64 {
	if fn == nil || l == nil {
		return 0
	}
	body, ok := fn.GetBasicBlockByID(l.Body)
	if !ok || body == nil {
		return 0
	}
	for _, s := range body.Succs {
		if s > 0 && s != l.Exit {
			return s
		}
	}
	return 0
}

// cfgGuardsLoopBreakContinueTargets collects Loop.Exit (break) and latch (continue) block ids.
func cfgGuardsLoopBreakContinueTargets(fn *ssa.Function) (loopExits, loopLatches map[int64]struct{}) {
	loopExits = make(map[int64]struct{})
	loopLatches = make(map[int64]struct{})
	if fn == nil {
		return loopExits, loopLatches
	}
	for _, bid := range fn.Blocks {
		b, ok := fn.GetBasicBlockByID(bid)
		if !ok || b == nil {
			continue
		}
		// Name-based (Yak for: addToBlocks renames to "loop.exit-N" / "loop.latch-N").
		if n := b.GetName(); n != "" {
			if strings.Contains(n, ssa.LoopExit) {
				loopExits[bid] = struct{}{}
			}
			if strings.Contains(n, ssa.LoopLatch) {
				loopLatches[bid] = struct{}{}
			}
		}
		for _, iid := range b.Insts {
			ins, ok := b.GetInstructionById(iid)
			if !ok || ins == nil {
				continue
			}
			if lz, ok := ssa.ToLazyInstruction(ins); ok && lz != nil {
				ins = lz.Self()
			}
			loop, ok := ins.(*ssa.Loop)
			if !ok || loop == nil {
				continue
			}
			if loop.Exit > 0 {
				loopExits[loop.Exit] = struct{}{}
			}
			if lid := cfgGuardsLoopLatchFromLoop(fn, loop); lid > 0 {
				loopLatches[lid] = struct{}{}
			}
		}
	}
	return loopExits, loopLatches
}

// cfgGuardsAbortBranchKind classifies the exiting side of an if/two-way guard for <cfgGuards>.
// Priority: Panic → GuardKindEarlyPanic; Return → GuardKindEarlyReturn; Jump to loop exit/latch
// → GuardKindEarlyBreak / GuardKindEarlyContinue; else GuardKindEarlyReturn.
func cfgGuardsAbortBranchKind(fn *ssa.Function, branchBlockID int64, loopExits, loopLatches map[int64]struct{}) string {
	if fn == nil || branchBlockID <= 0 {
		return GuardKindEarlyReturn
	}
	blk, ok := fn.GetBasicBlockByID(branchBlockID)
	if !ok || blk == nil {
		return GuardKindEarlyReturn
	}
	for _, iid := range blk.Insts {
		ins, ok := blk.GetInstructionById(iid)
		if !ok || ins == nil {
			continue
		}
		if lz, ok := ssa.ToLazyInstruction(ins); ok && lz != nil {
			ins = lz.Self()
		}
		if ins == nil {
			continue
		}
		if _, ok := ins.(*ssa.Panic); ok {
			return GuardKindEarlyPanic
		}
		if _, ok := ssa.ToReturn(ins); ok {
			return GuardKindEarlyReturn
		}
	}
	var last ssa.Instruction
	if len(blk.Insts) > 0 {
		last = blk.LastInst()
	}
	if last != nil {
		if lz, ok := ssa.ToLazyInstruction(last); ok && lz != nil {
			last = lz.Self()
		}
		if j, ok := ssa.ToJump(last); ok && j != nil && j.To > 0 {
			if _, ok := loopExits[j.To]; ok {
				return GuardKindEarlyBreak
			}
			if _, ok := loopLatches[j.To]; ok {
				return GuardKindEarlyContinue
			}
		}
	}
	return GuardKindEarlyReturn
}
