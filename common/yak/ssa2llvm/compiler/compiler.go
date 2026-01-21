package compiler

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

// Compiler holds the LLVM context and state for compiling a YakSSA program.
type Compiler struct {
	Ctx     context.Context
	LLVMCtx llvm.Context
	Mod     llvm.Module
	Builder llvm.Builder

	// Values maps YakSSA Value IDs to LLVM Values.
	// This is critical because YakSSA uses int64 IDs for all SSA values.
	Values map[int64]llvm.Value

	// Blocks maps YakSSA BasicBlock IDs to LLVM BasicBlocks.
	Blocks map[int64]llvm.BasicBlock

	// Program is the YakSSA program being compiled.
	Program *ssa.Program
}

// NewCompiler initializes a new Compiler instance.
func NewCompiler(ctx context.Context, prog *ssa.Program) *Compiler {
	c := llvm.NewContext()
	return &Compiler{
		Ctx:     ctx,
		LLVMCtx: c,
		Mod:     c.NewModule(prog.Name),
		Builder: c.NewBuilder(),
		Values:  make(map[int64]llvm.Value),
		Blocks:  make(map[int64]llvm.BasicBlock),
		Program: prog,
	}
}

// Dispose releases LLVM resources.
func (c *Compiler) Dispose() {
	c.Builder.Dispose()
	c.Mod.Dispose()
	c.LLVMCtx.Dispose()
}

// Compile (placeholder for future phases)
// currently just returns the module string for verification
// Compile iterates over all functions in the SSA program and compiles them to LLVM IR.
func (c *Compiler) Compile() error {
	var err error
	// Iterate over all functions in the program
	c.Program.EachFunction(func(fn *ssa.Function) {
		if err != nil {
			return
		}
		// Skip declaring external functions again if they serve as intrinsics
		// defined elsewhere, or handle them appropriately.
		// For now, assume all functions in SSA need compilation or declaration.

		// Compile the function
		if checkErr := c.CompileFunction(fn); checkErr != nil {
			err = fmt.Errorf("failed to compile function %s: %w", fn.GetName(), checkErr)
		}
	})
	return err
}

// CompileFunction compiles a single YakSSA function to LLVM IR.
func (c *Compiler) CompileFunction(fn *ssa.Function) error {
	// 1. Create function declaration
	// Assuming int64 for all types for this phase
	paramTypes := make([]llvm.Type, len(fn.Params))
	for i := range paramTypes {
		paramTypes[i] = c.LLVMCtx.Int64Type()
	}

	// retType=Int64, isVarArg=false
	fnType := llvm.FunctionType(c.LLVMCtx.Int64Type(), paramTypes, false)
	llvmFn := llvm.AddFunction(c.Mod, fn.GetName(), fnType)

	// 2. Register parameters to Values map
	for i, paramID := range fn.Params {
		paramVal := llvmFn.Param(i)
		paramVal.SetName(fmt.Sprintf("param_%d", paramID))
		c.Values[paramID] = paramVal
	}

	// 3. Pre-create all BasicBlocks
	// LLVM IR requires jump targets to exist, so we create them first.
	for _, blockID := range fn.Blocks {
		bb := c.LLVMCtx.AddBasicBlock(llvmFn, fmt.Sprintf("bb_%d", blockID))
		c.Blocks[blockID] = bb
	}

	// 4. Compile Instructions in each Block
	for _, blockID := range fn.Blocks {
		bb, ok := c.Blocks[blockID]
		if !ok {
			return fmt.Errorf("block %d not found", blockID)
		}
		c.Builder.SetInsertPointAtEnd(bb)

		// Get Block object from function
		val, ok := fn.GetValueById(blockID)
		if !ok {
			return fmt.Errorf("block value %d not found in function", blockID)
		}

		blockObj, ok := val.(*ssa.BasicBlock) // Type assertion might need adjustment if it's wrapped
		if !ok {
			// ssa.go says: type BasicBlock struct { *anValue ... }
			// It implements Value interface.
			// Let's check if we need to dereference or cast differently.
			// Based on ssa.go, it should work.
			return fmt.Errorf("value %d is not a BasicBlock", blockID)
		}

		hasTerminator := false
		for _, instID := range blockObj.Insts {
			instVal, ok := fn.GetValueById(instID)
			if !ok {
				continue
			}
			if inst, ok := instVal.(ssa.Instruction); ok {
				// Track if we compiled a terminator
				switch inst.(type) {
				case *ssa.Return, *ssa.Jump, *ssa.If:
					hasTerminator = true
				}

				if err := c.compileInstruction(inst); err != nil {
					return err
				}
			}
		}

		// Add default terminator if block doesn't have one
		if !hasTerminator {
			// Default: return 0
			c.Builder.CreateRet(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		}
	}

	// 5. Resolve Phis (Pass 2)
	for _, blockID := range fn.Blocks {
		val, ok := fn.GetValueById(blockID)
		if !ok {
			return fmt.Errorf("pass 2: block value %d not found", blockID)
		}
		blockObj, ok := val.(*ssa.BasicBlock)
		if !ok {
			return fmt.Errorf("pass 2: value %d is not a BasicBlock", blockID)
		}

		for _, instID := range blockObj.Insts {
			instVal, ok := fn.GetValueById(instID)
			if !ok {
				continue
			}
			if phi, ok := instVal.(*ssa.Phi); ok {
				if err := c.resolvePhi(phi); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
