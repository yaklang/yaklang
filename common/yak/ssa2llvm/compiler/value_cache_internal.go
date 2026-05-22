package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) useBlockID(contextInst ssa.Instruction) int64 {
	if contextInst != nil && contextInst.GetBlock() != nil {
		return contextInst.GetBlock().GetId()
	}
	if c != nil && c.function != nil && c.function.activeBlockID > 0 {
		return c.function.activeBlockID
	}
	return 0
}

func (c *Compiler) cacheValue(id int64, val llvm.Value) {
	if c == nil || id <= 0 || val.IsNil() {
		return
	}
	c.Values[id] = val
	if c.function == nil {
		return
	}
	if c.function.valueBlock == nil {
		c.function.valueBlock = make(map[int64]int64)
	}
	if blockID := c.function.activeBlockID; blockID > 0 {
		c.function.valueBlock[id] = blockID
	}
}

func (c *Compiler) getCachedValue(contextInst ssa.Instruction, id int64) (llvm.Value, bool) {
	if c == nil {
		return llvm.Value{}, false
	}
	val, ok := c.Values[id]
	if !ok || val.IsNil() {
		return llvm.Value{}, false
	}
	if c.isPortableCachedValue(id, val) {
		return val, true
	}

	useBlockID := c.useBlockID(contextInst)
	if useBlockID <= 0 || c.function == nil {
		return val, true
	}

	defBlockID, recorded := c.function.valueBlock[id]
	if !recorded || defBlockID == useBlockID {
		return val, true
	}
	if blockDominates(c.function.blockDominators, defBlockID, useBlockID) {
		return val, true
	}

	return llvm.Value{}, false
}

func (c *Compiler) isPortableCachedValue(id int64, val llvm.Value) bool {
	if val.IsNil() {
		return false
	}
	if c != nil && c.function != nil && c.function.current != nil {
		if valObj, ok := c.function.current.GetValueById(id); ok {
			switch valObj.(type) {
			case *ssa.ConstInst, *ssa.Undefined:
				return true
			}
			if _, ok := ssa.ToFunction(valObj); ok {
				return true
			}
		}
	}
	return false
}

func (c *Compiler) withEntryInsertPoint(fn *ssa.Function, fnDo func() error) error {
	if c == nil || fn == nil || fn.EnterBlock <= 0 {
		return fnDo()
	}
	entryBB, ok := c.Blocks[fn.EnterBlock]
	if !ok || entryBB.IsNil() {
		return fnDo()
	}

	restoreBB := c.restoreInsertBlock(nil)
	prevActive := int64(0)
	if c.function != nil {
		prevActive = c.function.activeBlockID
		c.function.activeBlockID = fn.EnterBlock
	}
	c.setInsertPointBeforeTerminator(entryBB)
	err := fnDo()
	if !restoreBB.IsNil() {
		c.Builder.SetInsertPointAtEnd(restoreBB)
	}
	if c.function != nil {
		c.function.activeBlockID = prevActive
	}
	return err
}
