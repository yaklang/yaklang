package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type functionCompileContext struct {
	current *ssa.Function

	invokeCtx       llvm.Value
	returnBlock     llvm.BasicBlock
	activeBlockID   int64
	compiledBlocks  map[int64]struct{}
	valueSlots     map[int64]llvm.Value
	storedValues   map[int64]struct{}

	exceptionValueIDs    map[int64]struct{}
	activeHandlerByBlock map[int64]int64
	catchBodyByHandler   map[int64]int64
	catchTargetByBlock   map[int64]int64
}

func newFunctionCompileContext(fn *ssa.Function) *functionCompileContext {
	return &functionCompileContext{
		current:        fn,
		compiledBlocks: make(map[int64]struct{}),
	}
}

func (c *Compiler) currentFunction() *ssa.Function {
	if c == nil || c.function == nil {
		return nil
	}
	return c.function.current
}
