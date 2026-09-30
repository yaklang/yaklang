package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckCanMemberCallExistPhiSelfEdgeDoesNotOverflow(t *testing.T) {
	_, builder := newTestBuilder(t)

	key := builder.EmitConstInst("field")
	base := builder.EmitEmptyContainer()

	phi := builder.EmitPhi("phi", Values{base})
	phi.Edge = append(phi.Edge, phi.GetId())

	res := checkCanMemberCallExist(phi, key)
	require.True(t, res.exist)
	require.NotEmpty(t, res.name)
}

func TestCheckCanMemberCallExistOrTypeDoesNotMutateValueType(t *testing.T) {
	_, builder := newTestBuilder(t)

	key := builder.EmitConstInst("field")
	value := builder.EmitEmptyContainer()

	originalType := NewOrType(CreateStringType(), NewObjectType())
	value.SetType(originalType)

	_ = checkCanMemberCallExist(value, key)

	require.Equal(t, OrTypeKind, value.GetType().GetTypeKind(), "member-call checks should not mutate value types")
}
