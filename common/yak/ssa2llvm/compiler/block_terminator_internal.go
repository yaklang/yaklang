package compiler

/*
#include <llvm-c/Core.h>
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func llvmBasicBlockTerminator(bb llvm.BasicBlock) llvm.Value {
	if bb.IsNil() {
		return llvm.Value{}
	}
	cbb := (C.LLVMBasicBlockRef)(unsafe.Pointer(bb.C))
	res := C.LLVMGetBasicBlockTerminator(cbb)
	var out llvm.Value
	*(*unsafe.Pointer)(unsafe.Pointer(&out)) = unsafe.Pointer(res)
	return out
}

func (c *Compiler) blockHasTerminator(bb llvm.BasicBlock) bool {
	return !llvmBasicBlockTerminator(bb).IsNil()
}

func (c *Compiler) emitSSABlockTerminator(blockID int64, blockObj *ssa.BasicBlock, fn *ssa.Function) error {
	if c == nil || blockObj == nil || fn == nil {
		return nil
	}
	bb, ok := c.Blocks[blockID]
	if !ok || bb.IsNil() {
		return fmt.Errorf("emitSSABlockTerminator: llvm block %d not found", blockID)
	}
	if c.blockHasTerminator(bb) {
		return nil
	}
	c.Builder.SetInsertPointAtEnd(bb)

	if fn.DeferBlock > 0 && blockID == fn.DeferBlock && c.function != nil && !c.function.returnBlock.IsNil() {
		c.Builder.CreateBr(c.function.returnBlock)
		return nil
	}
	if c.function != nil && c.function.catchTargetByBlock != nil {
		if targetID, ok := c.function.catchTargetByBlock[blockID]; ok && targetID > 0 {
			targetBB, ok := c.Blocks[targetID]
			if !ok {
				return fmt.Errorf("catch target block %d not found", targetID)
			}
			c.Builder.CreateBr(targetBB)
			return nil
		}
	}

	if len(blockObj.Succs) == 2 {
		var condID int64 = -1
		for i := len(blockObj.Insts) - 1; i >= 0; i-- {
			instVal, ok := fn.GetValueById(blockObj.Insts[i])
			if !ok {
				continue
			}
			if binOp, ok := instVal.(*ssa.BinOp); ok {
				if binOp.Op == ssa.OpGt || binOp.Op == ssa.OpLt ||
					binOp.Op == ssa.OpGtEq || binOp.Op == ssa.OpLtEq ||
					binOp.Op == ssa.OpEq || binOp.Op == ssa.OpNotEq {
					condID = blockObj.Insts[i]
					break
				}
			}
		}
		if condID != -1 {
			var contextInst ssa.Instruction
			if condInst, ok := fn.GetInstructionById(condID); ok {
				contextInst = condInst
			}
			condVal, err := c.getValue(contextInst, condID)
			if err != nil {
				return err
			}
			condVal = c.coerceToI1(condVal, "if_cond")
			trueBlock := c.Blocks[blockObj.Succs[0]]
			falseBlock := c.Blocks[blockObj.Succs[1]]
			c.Builder.CreateCondBr(condVal, trueBlock, falseBlock)
			return nil
		}
	} else if len(blockObj.Succs) == 1 {
		targetBlock := c.Blocks[blockObj.Succs[0]]
		c.Builder.CreateBr(targetBlock)
		return nil
	}

	return c.emitImplicitFunctionExit(fn)
}

func (c *Compiler) ensureAllBlockTerminators(fn *ssa.Function) error {
	if c == nil || fn == nil {
		return nil
	}
	for blockID, bb := range c.Blocks {
		if bb.IsNil() || c.blockHasTerminator(bb) {
			continue
		}
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			if err := c.ensureBasicBlockTerminator(bb, fn); err != nil {
				return err
			}
			continue
		}
		blockObj, ok := blockVal.(*ssa.BasicBlock)
		if !ok || blockObj == nil {
			if err := c.ensureBasicBlockTerminator(bb, fn); err != nil {
				return err
			}
			continue
		}
		if err := c.emitSSABlockTerminator(blockID, blockObj, fn); err != nil {
			return err
		}
	}
	return nil
}
