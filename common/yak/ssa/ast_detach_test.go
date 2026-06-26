package ssa

import (
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
)

func TestDetachAST_SlimsRootButKeepsDownwardChildren(t *testing.T) {
	parent := antlr.NewBaseParserRuleContext(nil, -1)
	child := antlr.NewBaseParserRuleContext(parent, -1)
	parent.AddChild(child)

	require.Equal(t, antlr.Tree(parent), child.GetParent(), "precondition: child points to parent")
	require.Len(t, parent.GetChildren(), 1, "precondition: parent has the child")

	got := DetachAST(child)

	require.Equal(t, child, got, "DetachAST returns node for chaining")
	require.Nil(t, child.GetParent(), "parent pointer must be cut")
	require.Len(t, parent.GetChildren(), 1, "parent.children must remain intact")
	require.Equal(t, antlr.Tree(child), parent.GetChild(0), "the same child object is still referenced downward")
}

func TestDetachAST_NilSafe(t *testing.T) {
	require.NotPanics(t, func() {
		var typedNil *antlr.BaseParserRuleContext
		DetachAST[antlr.Tree](typedNil)
		DetachAST[antlr.Tree](nil)
	})
}

func TestDetachAST_TerminalNodeIsNilSafe(t *testing.T) {
	term := antlr.NewTerminalNodeImpl(nil)
	require.NotPanics(t, func() {
		DetachAST[antlr.Tree](term)
	})
}
