package compiler

import (
	"context"
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/types"
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

	// Funcs maps YakSSA Function IDs to LLVM function values.
	Funcs map[int64]llvm.Value

	// Program is the YakSSA program being compiled.
	Program *ssa.Program

	TypeConverter *types.TypeConverter

	// ExternBindings maps source-level function names to runtime symbols/signatures.
	ExternBindings map[string]ExternBinding

	// InstrTags carries obfuscator-owned instruction tags emitted during SSA
	// rewriting. The compiler uses them to recognize synthetic operations without
	// extending the SSA package schema.
	InstrTags map[int64]string

	function *functionCompileContext
}

type CompilerOption func(*Compiler)

func WithExternBindings(custom map[string]ExternBinding) CompilerOption {
	return func(c *Compiler) {
		if len(custom) == 0 {
			return
		}
		c.ExternBindings = mergeExternBindings(c.ExternBindings, custom)
	}
}

func WithInstructionTags(tags map[int64]string) CompilerOption {
	return func(c *Compiler) {
		if len(tags) == 0 {
			return
		}
		c.InstrTags = make(map[int64]string, len(tags))
		for id, tag := range tags {
			c.InstrTags[id] = tag
		}
	}
}

// NewCompiler initializes a new Compiler instance.
func NewCompiler(ctx context.Context, prog *ssa.Program, opts ...CompilerOption) *Compiler {
	c := llvm.NewContext()
	comp := &Compiler{
		Ctx:            ctx,
		LLVMCtx:        c,
		Mod:            c.NewModule(prog.Name),
		Builder:        c.NewBuilder(),
		Values:         make(map[int64]llvm.Value),
		Blocks:         make(map[int64]llvm.BasicBlock),
		Funcs:          make(map[int64]llvm.Value),
		Program:        prog,
		TypeConverter:  types.NewTypeConverter(c),
		ExternBindings: cloneExternBindings(defaultExternBindings),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(comp)
		}
	}
	return comp
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
	c.function = newFunctionCompileContext(fn)
	defer func() {
		c.function = nil
	}()
	// 1. Get or declare LLVM function with a stable unique symbol name.
	llvmFn, _ := c.getOrDeclareLLVMFunction(fn)
	if err := c.prepareErrorHandling(fn); err != nil {
		return err
	}

	// 2. Register the InvokeContext parameter.
	if llvmFn.ParamsCount() < 1 {
		return fmt.Errorf("missing invoke context parameter for function %s", fn.GetName())
	}
	ctxParam := llvmFn.Param(0)
	ctxParam.SetName(fmt.Sprintf("ctx_%d", fn.GetId()))
	c.function.invokeCtx = ctxParam

	// 3. Pre-create all BasicBlocks
	// LLVM IR requires jump targets to exist, so we create them first.
	for _, blockID := range fn.Blocks {
		bb := c.LLVMCtx.AddBasicBlock(llvmFn, fmt.Sprintf("bb_%d", blockID))
		c.Blocks[blockID] = bb
	}
	// Add the unified return block last so the real entry block remains first.
	if fn.DeferBlock > 0 {
		c.function.returnBlock = c.LLVMCtx.AddBasicBlock(llvmFn, fmt.Sprintf("yak_ret_%d", fn.GetId()))
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

		blockObj, ok := val.(*ssa.BasicBlock)
		if !ok {
			return fmt.Errorf("value %d is not a BasicBlock", blockID)
		}

		// First, create Phi nodes at the beginning of the block
		for _, phiID := range blockObj.Phis {
			phiVal, ok := fn.GetValueById(phiID)
			if !ok {
				continue
			}
			if phi, ok := phiVal.(*ssa.Phi); ok {
				if err := c.compilePhi(phi); err != nil {
					return err
				}
			}
		}

		// Bind parameters from InvokeContext after phi nodes, to keep entry-block
		// phi ordering valid for LLVM IR.
		if blockID == fn.EnterBlock {
			if err := c.bindParamsFromContext(fn); err != nil {
				return err
			}
		}

		// Then compile regular instructions
		hasTerminator := false
		for _, instID := range blockObj.Insts {
			instVal, ok := fn.GetInstructionById(instID)
			if !ok || instVal == nil {
				continue
			}
			if instVal.IsLazy() {
				instVal = instVal.Self()
			}
			if instVal == nil {
				continue
			}
			inst := instVal
			isTerminator := false
			switch inst.(type) {
			case *ssa.Return, *ssa.Jump, *ssa.If, *ssa.Loop, *ssa.Panic:
				isTerminator = true
			}

			if err := c.compileInstruction(inst); err != nil {
				return err
			}
			if isTerminator {
				hasTerminator = true
				break
			}
		}

		// Add terminator based on block structure if not already present
		if !hasTerminator {
			// Defer block always falls through to the unified return.
			if fn.DeferBlock > 0 && blockID == fn.DeferBlock && !c.function.returnBlock.IsNil() {
				c.Builder.CreateBr(c.function.returnBlock)
				hasTerminator = true
			}
		}
		if !hasTerminator && c.function.catchTargetByBlock != nil {
			if targetID, ok := c.function.catchTargetByBlock[blockID]; ok && targetID > 0 {
				targetBB, ok := c.Blocks[targetID]
				if !ok {
					return fmt.Errorf("catch target block %d not found", targetID)
				}
				c.Builder.CreateBr(targetBB)
				hasTerminator = true
			}
		}
		if !hasTerminator {
			if len(blockObj.Succs) == 2 {
				// This is an If - find the condition from last BinOp (comparison)
				// Look backwards in Insts for the last comparison
				var condID int64 = -1
				for i := len(blockObj.Insts) - 1; i >= 0; i-- {
					instVal, ok := fn.GetValueById(blockObj.Insts[i])
					if !ok {
						continue
					}
					if binOp, ok := instVal.(*ssa.BinOp); ok {
						// Check if it's a comparison
						if binOp.Op == ssa.OpGt || binOp.Op == ssa.OpLt ||
							binOp.Op == ssa.OpGtEq || binOp.Op == ssa.OpLtEq ||
							binOp.Op == ssa.OpEq || binOp.Op == ssa.OpNotEq {
							condID = blockObj.Insts[i]
							break
						}
					}
				}

				if condID != -1 {
					condVal, _ := c.Values[condID]
					trueBlock := c.Blocks[blockObj.Succs[0]]
					falseBlock := c.Blocks[blockObj.Succs[1]]
					c.Builder.CreateCondBr(condVal, trueBlock, falseBlock)
					hasTerminator = true
				}
			} else if len(blockObj.Succs) == 1 {
				// This is a Jump
				targetBlock := c.Blocks[blockObj.Succs[0]]
				c.Builder.CreateBr(targetBlock)
				hasTerminator = true
			}

			// If still no terminator, add default return
			if !hasTerminator {
				if fn.DeferBlock > 0 && !c.function.returnBlock.IsNil() {
					// Implicit return 0 through defer.
					if err := c.storeContextReturn(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)); err != nil {
						return err
					}
					deferBB, ok := c.Blocks[fn.DeferBlock]
					if !ok {
						return fmt.Errorf("defer block %d not found for function %s", fn.DeferBlock, fn.GetName())
					}
					c.Builder.CreateBr(deferBB)
				} else {
					// Implicit return 0.
					if err := c.storeContextReturn(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)); err != nil {
						return err
					}
					c.Builder.CreateRetVoid()
				}
			}
		}
	}

	// 6. Resolve Phis (Pass 2)
	for _, blockID := range fn.Blocks {
		val, ok := fn.GetValueById(blockID)
		if !ok {
			return fmt.Errorf("pass 2: block value %d not found", blockID)
		}
		blockObj, ok := val.(*ssa.BasicBlock)
		if !ok {
			return fmt.Errorf("pass 2: value %d is not a BasicBlock", blockID)
		}

		for _, phiID := range blockObj.Phis {
			phiVal, ok := fn.GetValueById(phiID)
			if !ok {
				continue
			}
			if phi, ok := phiVal.(*ssa.Phi); ok {
				if err := c.resolvePhi(phi); err != nil {
					return err
				}
			}
		}
	}

	if fn.DeferBlock > 0 && !c.function.returnBlock.IsNil() {
		c.Builder.SetInsertPointAtEnd(c.function.returnBlock)
		c.Builder.CreateRetVoid()
	}

	return nil
}
