package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) ssaDefBlock(id int64) (int64, llvm.BasicBlock, bool) {
	if c == nil || c.function == nil || c.function.current == nil || id <= 0 {
		return 0, llvm.BasicBlock{}, false
	}
	fn := c.function.current
	valObj, ok := fn.GetValueById(id)
	if !ok || valObj == nil {
		return 0, llvm.BasicBlock{}, false
	}

	defBlockID := int64(0)
	if inst, ok := valObj.(ssa.Instruction); ok && inst != nil && inst.GetBlock() != nil {
		defBlockID = inst.GetBlock().GetId()
	}
	if defBlockID <= 0 {
		defBlockID = fn.EnterBlock
	}

	targetBB, ok := c.Blocks[defBlockID]
	if (!ok || targetBB.IsNil()) && fn.EnterBlock > 0 {
		if entryBB, ok := c.Blocks[fn.EnterBlock]; ok && !entryBB.IsNil() {
			targetBB = entryBB
			defBlockID = fn.EnterBlock
			ok = true
		}
	}
	if !ok || targetBB.IsNil() {
		return 0, llvm.BasicBlock{}, false
	}
	if c.function.compiledBlocks != nil {
		if _, compiled := c.function.compiledBlocks[defBlockID]; !compiled && fn.EnterBlock > 0 {
			if c.function.activeBlockID == defBlockID {
				return defBlockID, targetBB, true
			}
			if entryBB, ok := c.Blocks[fn.EnterBlock]; ok && !entryBB.IsNil() {
				targetBB = entryBB
				defBlockID = fn.EnterBlock
			}
		}
	}
	return defBlockID, targetBB, true
}

func (c *Compiler) reanchorSSADefInsertPoint(id int64) {
	blockID, bb, ok := c.ssaDefBlock(id)
	if !ok || bb.IsNil() {
		return
	}
	if c.function != nil {
		c.function.activeBlockID = blockID
	}
	c.setInsertPointBeforeTerminator(bb)
}

func (c *Compiler) withSSADefInsertPoint(id int64, fn func()) {
	if c == nil || fn == nil {
		return
	}
	blockID, bb, ok := c.ssaDefBlock(id)
	if !ok {
		fn()
		return
	}

	restore := c.restoreInsertBlock(nil)
	prevActive := int64(0)
	if c.function != nil {
		prevActive = c.function.activeBlockID
		c.function.activeBlockID = blockID
	}
	c.setInsertPointBeforeTerminator(bb)
	fn()
	if !restore.IsNil() {
		c.restoreInsertPoint(restore)
	}
	if c.function != nil {
		c.function.activeBlockID = prevActive
	}
}

func (c *Compiler) markSSAValueStored(id int64) {
	if c == nil || c.function == nil || id <= 0 {
		return
	}
	if c.function.storedValues == nil {
		c.function.storedValues = make(map[int64]struct{})
	}
	c.function.storedValues[id] = struct{}{}
}
