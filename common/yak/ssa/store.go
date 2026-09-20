package ssa

import "github.com/yaklang/yaklang/common/utils/memedit"

type StoredFunctionBuilder struct {
	Current                  *FunctionBuilder
	program                  *Program
	editor                   *memedit.MemEditor
	functionType             *FunctionType
	currentBlock             *BasicBlock
	markedThisClassBlueprint *Blueprint
	parentBuilder            *FunctionBuilder
}

func (s *StoredFunctionBuilder) CurrentBlock() *BasicBlock {
	if s == nil {
		return nil
	}
	return s.currentBlock
}

// NewExecutionBuilder restores lexical builder state into a private view.
// A lazy task must not rewrite the shared declaration builder just to enter
// its captured scope. Function/Program ownership and the function's evolving
// type remain on the original IR objects; restoring a scope does not reset them.
func (s *StoredFunctionBuilder) NewExecutionBuilder() *FunctionBuilder {
	if s == nil || s.Current == nil {
		return nil
	}
	b := *s.Current
	b.executionInitialBlock = b.CurrentBlock
	b.executionInitialReturn = b.IsReturn
	b._editor = s.editor
	b.parentBuilder = s.parentBuilder
	b.MarkedThisClassBlueprint = s.markedThisClassBlueprint
	return &b
}

// CommitExecutionBuilder keeps control-flow progress made by a scoped task.
// Lexical state stays private, but a field initializer can terminate the entry
// block and create a successor: the next initializer must continue in that block.
// Callers must serialize tasks owned by the same declaration function.
func (s *StoredFunctionBuilder) CommitExecutionBuilder(b *FunctionBuilder) {
	if s == nil || s.Current == nil || b == nil || b.Function != s.Current.Function {
		return
	}
	// An unchanged outer view must not undo progress committed by a nested
	// lazy initializer while the outer task was building a child function.
	if b.CurrentBlock == b.executionInitialBlock && b.IsReturn == b.executionInitialReturn {
		return
	}
	s.Current.CurrentBlock = b.CurrentBlock
	s.Current.IsReturn = b.IsReturn
}

func (b *FunctionBuilder) LoadFunctionBuilder(s *StoredFunctionBuilder) {
	if b == nil || s == nil {
		return
	}
	b._editor = s.editor
	// b.IncludeStack = s.IncludeStack
	// b.target = s.target
	// b.CurrentBlock = s.CurrentBlock
	// b.CurrentRange = s.CurrentRange
	// b.parentScope = s.parentScope
	b.parentBuilder = s.parentBuilder
	b.MarkedThisClassBlueprint = s.markedThisClassBlueprint

	// Only overwrite Type if the stored Type is not nil.
	// This prevents lazy-built functions (like TypeScript arrow functions)
	// from having their Type overwritten to nil when SwitchFunctionBuilder
	// restores the saved state after the lazy builder has set the Type.
	if s.functionType != nil {
		b.Function.Type = s.functionType
	}
	// b.Function.FreeValues = s.Function.FreeValues
	// b.Function.ParameterMembers = s.Function.ParameterMembers
	// b.Function.SideEffects = s.Function.SideEffects
	// b.Function.Return = s.Function.Return // Fix for TestTopDef_Anonymous：这会覆盖已经添加的 Return，导致嵌套闭包返回类型丢失
	// b.Function.Blocks = s.Function.Blocks

	b.SetProgram(s.program)
}

func (b *FunctionBuilder) StoreFunctionBuilder() *StoredFunctionBuilder {
	return &StoredFunctionBuilder{
		Current:                  b,
		program:                  b.GetProgram(),
		editor:                   b._editor,
		functionType:             b.Function.Type,
		currentBlock:             b.CurrentBlock,
		markedThisClassBlueprint: b.MarkedThisClassBlueprint,
		parentBuilder:            b.parentBuilder,
	}
}
