package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type functionCompileContext struct {
	current *ssa.Function

	invokeCtx      llvm.Value
	returnBlock    llvm.BasicBlock
	llvmFn         llvm.Value
	activeBlockID  int64
	compiledBlocks map[int64]struct{}
	valueSlots     map[int64]llvm.Value
	storedValues   map[int64]struct{}

	exceptionValueIDs    map[int64]struct{}
	activeHandlerByBlock map[int64]int64
	catchBodyByHandler   map[int64]int64
	catchTargetByBlock   map[int64]int64
	switchHandlers       map[int64]*switchHandlerInfo
	pendingMemberSets    map[string]pendingMemberSet
	pendingMemberSetKeys []string
}

type switchHandlerInfo struct {
	condID       int64
	labelIDs     []int64
	trueBlockID  int64
	falseBlockID int64
}

type pendingMemberSet struct {
	source   ssa.Value
	resultID int64
	obj      ssa.Value
	key      ssa.Value
	direct   bool
}

func newFunctionCompileContext(fn *ssa.Function) *functionCompileContext {
	return &functionCompileContext{
		current:           fn,
		compiledBlocks:    make(map[int64]struct{}),
		pendingMemberSets: make(map[string]pendingMemberSet),
	}
}

func (c *Compiler) currentFunction() *ssa.Function {
	if c == nil || c.function == nil {
		return nil
	}
	return c.function.current
}
