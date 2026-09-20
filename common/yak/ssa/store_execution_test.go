package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestStoredBuilderExecutionDoesNotMutateDeclaration(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-scope")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	original := memedit.NewMemEditor("original")
	decl.SetEditor(original)
	stored := decl.StoreFunctionBuilder()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			view := stored.NewExecutionBuilder()
			view.SetEditor(memedit.NewMemEditor("private"))
			view.MarkedThisClassBlueprint = NewBlueprint("private")
		}()
	}
	wg.Wait()
	require.Same(t, original, decl.GetEditor())
	require.Nil(t, decl.MarkedThisClassBlueprint)
	require.Same(t, prog, decl.GetProgram())
}

func TestStoredBuilderExecutionKeepsControlFlowProgress(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-control-flow")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	stored := decl.StoreFunctionBuilder()
	first := stored.NewExecutionBuilder()
	next := first.NewBasicBlock("next-initializer")
	first.EmitJump(next)
	first.CurrentBlock = next
	stored.CommitExecutionBuilder(first)
	second := stored.NewExecutionBuilder()
	require.Same(t, next, second.CurrentBlock)
	require.False(t, second.IsBlockFinish())
}

func TestStoredBuilderExecutionPreservesNestedControlFlow(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-nested-control-flow")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	stored := decl.StoreFunctionBuilder()
	outer := stored.NewExecutionBuilder()
	// Building a method may trigger another class's initializer in the
	// declaration function before the outer lexical view returns.
	inner := stored.NewExecutionBuilder()
	next := inner.NewBasicBlock("nested-initializer")
	inner.EmitJump(next)
	inner.CurrentBlock = next
	stored.CommitExecutionBuilder(inner)
	stored.CommitExecutionBuilder(outer)
	resumed := stored.NewExecutionBuilder()
	require.Same(t, next, resumed.CurrentBlock)
	require.False(t, resumed.IsBlockFinish())
}

func TestStoredBuilderNestedExecutionSharesLiveControlFlow(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-live-control-flow")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	decl.SetEditor(memedit.NewMemEditor("declaration"))
	stored := decl.StoreFunctionBuilder()
	outer, leaveOuter := stored.EnterExecutionBuilder()
	outer.SetEditor(memedit.NewMemEditor("outer"))
	first := outer.NewBasicBlock("first")
	outer.EmitJump(first)
	outer.CurrentBlock = first
	inner, leaveInner := stored.EnterExecutionBuilder()
	require.Same(t, first, inner.CurrentBlock)
	require.Equal(t, "declaration", inner.GetEditor().GetSourceCode())
	second := inner.NewBasicBlock("second")
	inner.EmitJump(second)
	inner.CurrentBlock = second
	leaveInner()
	require.Same(t, second, outer.CurrentBlock)
	require.Equal(t, "outer", outer.GetEditor().GetSourceCode())
	leaveOuter()
	require.Same(t, second, decl.CurrentBlock)
	require.Nil(t, decl.Function.executionBuilder)
}

func TestStoredBuilderCapturedPrivateScopeUsesLatestOwnerFlow(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "builder-captured-private-scope")
	decl := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	stored := decl.StoreFunctionBuilder()
	view, leave := stored.EnterExecutionBuilder()
	view.SetEditor(memedit.NewMemEditor("captured"))
	captured := view.StoreFunctionBuilder()
	leave()
	other, leaveOther := stored.EnterExecutionBuilder()
	next := other.NewBasicBlock("after-capture")
	other.EmitJump(next)
	other.CurrentBlock = next
	leaveOther()
	resumed, leaveResumed := captured.EnterExecutionBuilder()
	require.Same(t, next, resumed.CurrentBlock)
	require.Equal(t, "captured", resumed.GetEditor().GetSourceCode())
	leaveResumed()
}
