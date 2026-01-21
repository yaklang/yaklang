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
	case *ssa.Jump:
		return c.compileJump(op)
	case *ssa.If:
		return c.compileIf(op)
	case *ssa.Return:
		return c.compileReturn(op)
	case *ssa.ConstInst:
		return c.compileConst(op)
	case *ssa.Call:
		return c.compileCall(op)
	default:
		// Ignore unimplemented instructions for now
		return nil
	}
}

// getValue resolves an SSA value ID to an LLVM value, performing lazy compilation
// for constants if they haven't been visited yet.
func (c *Compiler) getValue(contextInst ssa.Instruction, id int64) (llvm.Value, error) {
	// 1. Check cache
	if val, ok := c.Values[id]; ok {
		return val, nil
	}

	// 2. Not found, try to find in function and compile if it's a constant
	fn := contextInst.GetFunc()
	if fn == nil {
		return llvm.Value{}, fmt.Errorf("getValue: context instruction has no function")
	}

	valObj, ok := fn.GetValueById(id)
	if !ok {
		return llvm.Value{}, fmt.Errorf("getValue: value %d not found in function", id)
	}

	// 3. Lazy compile if ConstInst
	if constInst, ok := valObj.(*ssa.ConstInst); ok {
		if err := c.compileConst(constInst); err != nil {
			return llvm.Value{}, err
		}
		// Should be in cache now
		if val, ok := c.Values[id]; ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileConst succeded but value %d not cached", id)
	}

	// 4. Return error if not found and not a constant
	// This usually means we are referencing an instruction that hasn't been compiled yet
	// (back-edge or dependency order issue) or not implemented.
	return llvm.Value{}, fmt.Errorf("getValue: value %d not found (dependency missing?)", id)
}

func (c *Compiler) compileBinOp(inst *ssa.BinOp, resultID int64) error {
	lhs, err := c.getValue(inst, inst.X)
	if err != nil {
		return err
	}
	rhs, err := c.getValue(inst, inst.Y)
	if err != nil {
		return err
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

func (c *Compiler) compileConst(inst *ssa.ConstInst) error {
	id := inst.GetId()
	if _, ok := c.Values[id]; ok {
		return nil // Already compiled
	}

	// Handle different constant types
	// For now, assume int64 unless we can detect otherwise
	if inst.IsNumber() {
		// Use Int64 for simplicity as per Phase 1
		val := inst.Number()
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(val), true) // Signed
		c.Values[id] = llvmVal
		return nil
	} else if inst.IsBoolean() {
		// Represent bool as i64 0 or 1 for compatibility with mixed ops,
		// or handle strictly.
		// NOTE: BinOps expect i64 operands in our current implementation.
		// If explicit bool type needed, we might need zext/sext.
		// Let's use i64 0/1 for now.
		bVal := inst.Boolean()
		iVal := uint64(0)
		if bVal {
			iVal = 1
		}
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), iVal, false)
		c.Values[id] = llvmVal
		return nil
	}

	// Fallback/TODO: floats, strings, nil
	// For now, log warning or create undef?
	// Return 0 for unknown to prevent crash?
	fmt.Printf("WARNING: Unsupported constant type for %v (ID: %d)\n", inst.GetRawValue(), id)
	llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	c.Values[id] = llvmVal
	return nil
}

func (c *Compiler) compileReturn(inst *ssa.Return) error {
	if len(inst.Results) == 0 {
		c.Builder.CreateRetVoid()
		return nil
	}

	// Only support single return value for now
	retID := inst.Results[0]
	val, err := c.getValue(inst, retID)
	if err != nil {
		return err
	}
	c.Builder.CreateRet(val)
	return nil
}
