package compiler

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

func (c *Compiler) compileInstruction(inst ssa.Instruction) error {
	id := inst.GetId()

	switch op := inst.(type) {
	case *ssa.BinOp:
		return c.compileBinOp(op, id)
	case *ssa.Return:
		return c.compileReturn(op)
	default:
		// Ignore unimplemented instructions for now
		return nil
	}
}

func (c *Compiler) compileBinOp(inst *ssa.BinOp, resultID int64) error {
	lhs, ok1 := c.Values[inst.X]
	rhs, ok2 := c.Values[inst.Y]

	if !ok1 || !ok2 {
		return fmt.Errorf("compileBinOp: operands not found (X:%d, Y:%d)", inst.X, inst.Y)
	}

	var val llvm.Value
	name := fmt.Sprintf("val_%d", resultID)

	switch inst.Op {
	case ssa.OpAdd:
		val = c.Builder.CreateAdd(lhs, rhs, name)
	case ssa.OpSub:
		val = c.Builder.CreateSub(lhs, rhs, name)
	case ssa.OpMul:
		val = c.Builder.CreateMul(lhs, rhs, name)
	case ssa.OpDiv:
		val = c.Builder.CreateSDiv(lhs, rhs, name) // Signed div for now
	case ssa.OpMod:
		val = c.Builder.CreateSRem(lhs, rhs, name) // Signed rem
	default:
		return fmt.Errorf("unknown BinOp opcode: %v", inst.Op)
	}

	c.Values[resultID] = val
	return nil
}

func (c *Compiler) compileReturn(inst *ssa.Return) error {
	if len(inst.Results) == 0 {
		c.Builder.CreateRetVoid()
		return nil
	}

	// Only support single return value for now
	retID := inst.Results[0]
	val, ok := c.Values[retID]
	if !ok {
		return fmt.Errorf("return value not found: %d", retID)
	}
	c.Builder.CreateRet(val)
	return nil
}
