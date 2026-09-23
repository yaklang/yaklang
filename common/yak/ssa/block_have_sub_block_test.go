package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newHaveSubBlockTestProgram(t *testing.T) *Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("have-sub-block"))
	require.NoError(t, err)
	return NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 1)
}

func newHaveSubBlockTestFunction(t *testing.T, prog *Program) *Function {
	t.Helper()
	f := NewFunctionWithType("main", nil)
	f.SetProgram(prog)
	return f
}

func TestHaveSubBlock_WalksParentChain(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)

	root := f.NewBasicBlock("root")
	mid := f.NewBasicBlock("mid")
	mid.Parent = root.GetId()
	leaf := f.NewBasicBlock("leaf")
	leaf.Parent = mid.GetId()

	require.True(t, root.HaveSubBlock(root), "a block is its own sub block")
	require.True(t, root.HaveSubBlock(mid), "direct child")
	require.True(t, root.HaveSubBlock(leaf), "grand child through the parent chain")
	require.True(t, mid.HaveSubBlock(leaf), "direct child of the middle block")

	require.False(t, leaf.HaveSubBlock(root), "the parent is not a sub block of the child")
	require.False(t, leaf.HaveSubBlock(mid))
	require.False(t, root.HaveSubBlock(nil))

	sibling := f.NewBasicBlock("sibling")
	require.False(t, root.HaveSubBlock(sibling), "a block without a parent link is not a sub block")
}

// TestHaveSubBlock_DirectParentWithoutLookup locks in the fast path: when the
// walked block points at b itself, the answer must not depend on the parent
// block being resolvable from the program cache. The previous implementation
// loaded the parent and reported "not a basic block" when the load failed,
// answering false for a block that is a direct child.
func TestHaveSubBlock_DirectParentWithoutLookup(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)

	parent := f.NewBasicBlock("parent")
	child := f.NewBasicBlock("child")
	child.Parent = parent.GetId()

	prog.Cache.DeleteInstruction(parent)
	require.Nil(t, prog.Cache.GetInstruction(parent.GetId()), "precondition: parent block evicted from the cache")

	require.True(t, parent.HaveSubBlock(child),
		"direct child must be recognized even when the parent block cannot be resolved")
}

func TestHaveSubBlock_StopsWhenIntermediateParentIsGone(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)

	root := f.NewBasicBlock("root")
	mid := f.NewBasicBlock("mid")
	mid.Parent = root.GetId()
	leaf := f.NewBasicBlock("leaf")
	leaf.Parent = mid.GetId()

	prog.Cache.DeleteInstruction(mid)

	require.False(t, root.HaveSubBlock(leaf),
		"the walk stops at the unresolvable parent instead of reporting an ancestor hit")
}

func TestHaveSubBlock_StopsOnParentCycle(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)

	outer := f.NewBasicBlock("outer")
	a := f.NewBasicBlock("a")
	b := f.NewBasicBlock("b")
	a.Parent = b.GetId()
	b.Parent = a.GetId()

	require.False(t, outer.HaveSubBlock(a), "a cyclic parent chain must terminate")
}

func TestHaveSubBlock_RejectsNonBlockValues(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)
	root := f.NewBasicBlock("root")

	var typedNil *BasicBlock
	require.False(t, root.HaveSubBlock(typedNil), "typed nil block must not panic or match")
	require.False(t, root.HaveSubBlock(NewUndefined("not-a-block")), "plain values are not blocks")
}
