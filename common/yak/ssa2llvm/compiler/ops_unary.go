package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) compileUnOp(inst *ssa.UnOp, resultID int64) error {
	x, err := c.getValue(inst, inst.X)
	if err != nil {
		return err
	}
	x = c.coerceToInt64(x)

	var val llvm.Value
	name := fmt.Sprintf("unop_%d", resultID)
	i64 := c.LLVMCtx.Int64Type()

	switch inst.Op {
	case ssa.OpNot:
		one := llvm.ConstInt(i64, 1, false)
		val = c.Builder.CreateXor(x, one, name)
	case ssa.OpNeg:
		zero := llvm.ConstInt(i64, 0, false)
		val = c.Builder.CreateSub(zero, x, name)
	case ssa.OpPlus:
		val = x
	case ssa.OpBitwiseNot:
		minusOne := llvm.ConstInt(i64, ^uint64(0), true)
		val = c.Builder.CreateXor(x, minusOne, name)
	default:
		return fmt.Errorf("compileUnOp: unsupported opcode %v", inst.Op)
	}

	c.Values[resultID] = val
	if err := c.maybeEmitMemberSet(inst, inst, val); err != nil {
		return err
	}
	return nil
}
