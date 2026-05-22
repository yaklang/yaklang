package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) withLazyCompileInsertPoint(contextInst, targetInst ssa.Instruction, compile func() error) error {
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
	c.setInsertPointBeforeTerminator(targetBB)
	return compile()
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
