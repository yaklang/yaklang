package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// materializeEntryCallable gives an explicitly selected function the same
// closure environment as an ordinary call. A raw function address is only
// sufficient when the function has no captures. In particular, a check entry
// that captures a recursive function must carry that function's self cell.
func (c *Compiler) entryFunctionCapturesValues(fn *ssa.Function) bool {
	if c == nil || fn == nil || c.capturedFunctionIDs == nil {
		return false
	}
	_, ok := c.capturedFunctionIDs[fn.GetId()]
	return ok
}

func (c *Compiler) materializeEntryCallable(entry, mainFn llvm.Value, block llvm.BasicBlock, invokeCtx llvm.Value) (llvm.Value, error) {
	var selected *ssa.Function
	c.Program.EachFunction(func(fn *ssa.Function) {
		if compiled, ok := c.Funcs[fn.GetId()]; ok && compiled.C == entry.C {
			selected = fn
		}
	})
	// A raw address is valid unless the selected entry itself captures
	// values. callret copies callee blocks into the entry and can leave the
	// callee's free-value table populated. Those captures already have stack
	// pops in the merged body, so they must not make the entry build another
	// closure or declare the merged callee as an external symbol.
	if selected == nil || !c.entryFunctionCapturesValues(selected) {
		return c.Builder.CreatePtrToInt(entry, c.LLVMCtx.Int64Type(), "yak_entry_target"), nil
	}
	parent := selected.GetParent()
	if parent == nil {
		return llvm.Value{}, fmt.Errorf("entry %s has captures but no defining scope", selected.GetName())
	}
	savedFunction, savedValues, savedBlocks, savedEntries := c.function, c.Values, c.Blocks, c.blockEntries
	defer func() {
		c.function, c.Values, c.Blocks, c.blockEntries = savedFunction, savedValues, savedBlocks, savedEntries
	}()
	// Resolve captured defaults in their defining SSA scope, but emit them in
	// the executable wrapper. Never reuse LLVM locals from an already compiled
	// function: they do not dominate this wrapper's entry block.
	c.function = newFunctionCompileContext(parent)
	c.function.llvmFn = mainFn
	c.function.entryBlock = block
	c.function.activeBlockID = parent.EnterBlock
	c.function.invokeCtx = invokeCtx
	c.Values = make(map[int64]llvm.Value)
	c.Blocks = map[int64]llvm.BasicBlock{parent.EnterBlock: block}
	c.blockEntries = map[int64]llvm.BasicBlock{parent.EnterBlock: block}
	return c.materializeCallableClosure(nil, selected)
}
