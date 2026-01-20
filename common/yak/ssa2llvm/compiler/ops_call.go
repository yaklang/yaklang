package compiler

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

// compileCall compiles a ssa.Call instruction to LLVM IR.
func (c *Compiler) compileCall(inst *ssa.Call) error {
	// 1. Resolve callee name
	calleeName := ""

	// Try to get the callee Value from Function context
	fn := inst.GetFunc()
	if fn != nil {
		if calleeVal, ok := fn.GetValueById(inst.Method); ok && calleeVal != nil {
			calleeName = calleeVal.GetName()
			// If it's a function, use its name
			if ssaFn, ok := ssa.ToFunction(calleeVal); ok {
				calleeName = ssaFn.GetName()
			}
		}
	}

	// Fallback: generate name from ID if lookup failed
	if calleeName == "" {
		calleeName = fmt.Sprintf("func_%d", inst.Method)
	}

	// 2. Get or declare LLVM function
	llvmFn := c.Mod.NamedFunction(calleeName)
	if llvmFn.IsNil() {
		// Function not found, create a declaration (prototype)
		// Default: all args and return are i64
		argTypes := make([]llvm.Type, len(inst.Args))
		for i := range argTypes {
			argTypes[i] = c.LLVMCtx.Int64Type()
		}
		funcType := llvm.FunctionType(c.LLVMCtx.Int64Type(), argTypes, false)
		llvmFn = llvm.AddFunction(c.Mod, calleeName, funcType)
	}

	// 3. Prepare arguments
	args := make([]llvm.Value, 0, len(inst.Args))
	for _, argID := range inst.Args {
		argVal, ok := c.Values[argID]
		if !ok {
			return fmt.Errorf("compileCall: argument %d not found in Values map", argID)
		}
		args = append(args, argVal)
	}

	// 4. Create call instruction
	// Get function type for CreateCall
	fnType := llvmFn.GlobalValueType()
	callResult := c.Builder.CreateCall(fnType, llvmFn, args, "")

	// 5. Register result if the call has users (returns a value)
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
	}

	return nil
}
