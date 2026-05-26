package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) restoreInsertPoint(bb llvm.BasicBlock) {
	if c == nil || bb.IsNil() {
		return
	}
	if c.blockHasTerminator(bb) {
		c.setInsertPointBeforeTerminator(bb)
		return
	}
	c.Builder.SetInsertPointAtEnd(bb)
}

func (c *Compiler) withLazyCompileInsertPoint(contextInst, targetInst ssa.Instruction, compile func() error) error {
	restoreBB := c.restoreInsertBlock(contextInst)

	if c == nil || targetInst == nil || targetInst.GetBlock() == nil {
		return compile()
	}
	if contextInst != nil && contextInst.GetBlock() != nil &&
		contextInst.GetBlock().GetId() == targetInst.GetBlock().GetId() {
		return compile()
	}

	targetBB, ok := c.Blocks[targetInst.GetBlock().GetId()]
	if !ok || targetBB.IsNil() {
		return compile()
	}
	targetBlockID := targetInst.GetBlock().GetId()
	if c.function != nil {
		if _, compiled := c.function.compiledBlocks[targetBlockID]; !compiled {
			// Forward reference: emit in the entry block so the value dominates all uses.
			if fn := c.function.current; fn != nil && fn.EnterBlock > 0 {
				if entryBB, ok := c.Blocks[fn.EnterBlock]; ok && !entryBB.IsNil() {
					c.setInsertPointBeforeTerminator(entryBB)
					err := compile()
					if !restoreBB.IsNil() {
						c.restoreInsertPoint(restoreBB)
					}
					return err
				}
			}
			return compile()
		}
	}
	c.setInsertPointBeforeTerminator(targetBB)
	err := compile()
	if !restoreBB.IsNil() {
		c.restoreInsertPoint(restoreBB)
	}
	return err
}

func (c *Compiler) restoreInsertBlock(contextInst ssa.Instruction) llvm.BasicBlock {
	if contextInst != nil && contextInst.GetBlock() != nil {
		if bb, ok := c.Blocks[contextInst.GetBlock().GetId()]; ok && !bb.IsNil() {
			return bb
		}
	}
	if c != nil && c.function != nil && c.function.activeBlockID > 0 {
		if bb, ok := c.Blocks[c.function.activeBlockID]; ok && !bb.IsNil() {
			return bb
		}
	}
	return llvm.BasicBlock{}
}

func (c *Compiler) setInsertPointBeforeTerminator(bb llvm.BasicBlock) {
	if bb.IsNil() {
		return
	}
	term := c.lastInstruction(bb)
	if !term.IsNil() && c.instructionIsTerminator(term) {
		c.Builder.SetInsertPointBefore(term)
		return
	}
	c.Builder.SetInsertPointAtEnd(bb)
}

func (c *Compiler) instructionIsTerminator(inst llvm.Value) bool {
	if inst.IsNil() {
		return false
	}
	switch inst.NumOperands() {
	case 0:
		return true
	case 1, 3:
		return true
	default:
		return false
	}
}

func (c *Compiler) emitImplicitFunctionExit(fn *ssa.Function) error {
	if err := c.storeContextReturn(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)); err != nil {
		return err
	}
	if fn != nil && fn.DeferBlock > 0 && c.function != nil && !c.function.returnBlock.IsNil() {
		deferBB, ok := c.Blocks[fn.DeferBlock]
		if !ok {
			return fmt.Errorf("defer block %d not found for function %s", fn.DeferBlock, fn.GetName())
		}
		c.Builder.CreateBr(deferBB)
		return nil
	}
	c.Builder.CreateRetVoid()
	return nil
}

func (c *Compiler) ensureBasicBlockTerminator(bb llvm.BasicBlock, fn *ssa.Function) error {
	if c == nil || bb.IsNil() || fn == nil {
		return nil
	}
	if c.blockHasTerminator(bb) {
		return nil
	}
	c.Builder.SetInsertPointAtEnd(bb)
	return c.emitImplicitFunctionExit(fn)
}
