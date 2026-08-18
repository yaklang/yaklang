package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type functionCompileContext struct {
	current *ssa.Function

	// ownerValueIDs is the set of SSA value ids that belong to current. It is
	// used to reject cross-function references (front-end artifacts where a
	// value from another function leaks into this function's instruction
	// graph) before they are lazily compiled.
	ownerValueIDs map[int64]struct{}

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

func (c *Compiler) ensureOwnerValueIDs(fn *ssa.Function) {
	if c == nil || c.function == nil || fn == nil || c.function.ownerValueIDs != nil {
		return
	}
	ids := make(map[int64]struct{})
	for _, pid := range fn.Params {
		ids[pid] = struct{}{}
	}
	for _, pid := range fn.ParameterMembers {
		ids[pid] = struct{}{}
	}
	for _, vid := range fn.FreeValues {
		ids[vid] = struct{}{}
	}
	for _, blockID := range fn.Blocks {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			continue
		}
		bb, ok := ssa.ToBasicBlock(blockVal)
		if !ok || bb == nil {
			continue
		}
		for _, iid := range bb.Insts {
			ids[iid] = struct{}{}
		}
		for _, pid := range bb.Phis {
			ids[pid] = struct{}{}
		}
	}
	c.function.ownerValueIDs = ids
}
