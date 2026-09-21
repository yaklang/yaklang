package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteInstDropsPhiEdgeAndMemberLink(t *testing.T) {
	_, builder := newTestBuilder(t)

	left := builder.EmitConstInst(1)
	right := builder.EmitConstInst(2)
	phi := builder.EmitPhi("p", []Value{left, right})
	require.Equal(t, []int64{left.GetId(), right.GetId()}, phi.Edge)

	obj := builder.EmitEmptyContainer()
	key := builder.EmitConstInst("field")
	member := builder.EmitConstInst(3)
	setMemberCallRelationship(obj, key, member)
	_, ok := GetLatestMemberByKey(obj, key)
	require.True(t, ok)

	DeleteInst(left)
	require.Equal(t, []int64{right.GetId()}, phi.Edge)
	require.False(t, userContains(left, phi.GetId()))
	_, stillThere := builder.GetInstructionById(left.GetId())
	require.False(t, stillThere)

	DeleteInst(member)
	_, ok = GetLatestMemberByKey(obj, key)
	require.False(t, ok)
	require.Empty(t, GetObjectKeyPairs(member))
}

func TestValueForSideEffectAssignMergesLoopValue(t *testing.T) {
	_, builder := newTestBuilder(t)

	callee := builder.EmitUndefined("callee")
	call := builder.EmitCall(builder.NewCall(callee, nil))
	require.NotNil(t, call)

	prev := builder.EmitConstInst(1)
	variable := builder.CreateVariable("a")
	builder.AssignVariable(variable, prev)
	builder.CurrentBlock.SetName(LoopBody)

	modify := builder.EmitConstInst(2)
	sideEffect := builder.EmitSideEffect("a", call, modify)
	require.NotNil(t, sideEffect)

	got := valueForSideEffectAssign(builder, variable, sideEffect)
	phi, ok := ToPhi(got)
	require.True(t, ok)
	require.Equal(t, []int64{prev.GetId(), sideEffect.GetId()}, phi.Edge)

	outside := builder.NewBasicBlock("after")
	builder.CurrentBlock = outside
	plain := builder.EmitConstInst(3)
	other := builder.EmitSideEffect("a", call, plain)
	require.Equal(t, other.GetId(), valueForSideEffectAssign(builder, variable, other).GetId())
}
