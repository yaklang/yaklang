package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A finished function body has its Function.builder cleared, but a child
// builder that was queued for a later lazy build still holds that
// FunctionBuilder in its parentBuilder chain.
//
// Walking the chain through p.builder.parentBuilder dereferences the cleared
// Function.builder and panics. The FunctionBuilder's own parentBuilder field
// stays valid and is the link that must be followed.
func TestCheckMemberSideEffectWalksParentsAfterParentBuilderReleased(t *testing.T) {
	prog := NewTmpProgram("released-parent-builder-side-effect")
	newTestEditor(t, prog)

	_, parentBuilder := newTestBuilderFunc(t, prog, "parent-func", nil)
	parentVar := parentBuilder.CreateVariable("x")
	require.NotNil(t, parentVar)
	parentBuilder.AssignVariable(parentVar, NewUndefined("parent-x"))

	childFunc, childBuilder := newTestBuilderFunc(t, prog, "child-func", parentBuilder)
	require.Equal(t, parentBuilder, childBuilder.parentBuilder)

	// The release path nils Function.builder while the chain still points here.
	parentBuilder.Function.builder = nil

	v := NewUndefined("value")

	// A name the parent scope does not hold forces the walk to advance through
	// the chain, which is where the cleared Function.builder was dereferenced.
	missing := NewVariable(1, "missing", true, nil).(*Variable)
	require.NotPanics(t, func() {
		childBuilder.CheckMemberSideEffect(missing, v)
	}, "advancing the parent chain must not dereference a released Function.builder")
	require.NotPanics(t, func() {
		childFunc.AddSideEffect(missing, v)
	}, "advancing the parent chain must not panic the side-effect walk")

	variable := NewVariable(1, "x", true, nil).(*Variable)
	require.NotPanics(t, func() {
		childBuilder.CheckMemberSideEffect(variable, v)
	}, "a released parent builder must not panic the member side-effect walk")

	require.NotPanics(t, func() {
		childFunc.AddSideEffect(variable, v)
	}, "a released parent builder must not panic the side-effect walk")

	require.Len(t, childFunc.SideEffects, 2)
	require.NotNil(t, childFunc.SideEffects[0].Variable,
		"a side effect always keeps a bind target")
	require.Same(t, parentVar, childFunc.SideEffects[1].Variable,
		"the walk still reaches the parent scope through the FunctionBuilder link")
}
