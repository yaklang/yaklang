package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratePhiWithNilCfgEntryBlock(t *testing.T) {
	_, builder := newTestBuilder(t)
	createPhi := generatePhi(builder, nil, nil)
	const1 := builder.EmitConstInst(1)
	const2 := builder.EmitConstInst(2)

	phi := createPhi("test_phi", []Value{const1, const2})
	require.NotNil(t, phi)

	phiInst, ok := ToPhi(phi)
	require.True(t, ok)
	require.Equal(t, int64(-1), phiInst.CFGEntryBasicBlock)
	require.Equal(t, []int64{const1.GetId(), const2.GetId()}, phiInst.Edge)
}

func TestGeneratePhiTrivialReturnsValue(t *testing.T) {
	_, builder := newTestBuilder(t)
	createPhi := generatePhi(builder, nil, nil)
	a := builder.EmitConstInst(1)
	before := len(builder.CurrentBlock.Phis)

	got := createPhi("a", []Value{a, a, nil, a})
	require.Equal(t, a.GetId(), got.GetId())
	_, ok := ToPhi(got)
	require.False(t, ok)
	require.Equal(t, before, len(builder.CurrentBlock.Phis))

	require.Nil(t, createPhi("empty", nil))
	require.Nil(t, createPhi("nils", []Value{nil, nil}))
}

func TestGeneratePhiDedupesEdges(t *testing.T) {
	_, builder := newTestBuilder(t)
	createPhi := generatePhi(builder, nil, nil)
	a := builder.EmitConstInst(1)
	b := builder.EmitConstInst(2)

	got := createPhi("ab", []Value{a, a, b, nil, a})
	phi, ok := ToPhi(got)
	require.True(t, ok)
	require.Equal(t, []int64{a.GetId(), b.GetId()}, phi.Edge)
}

func TestEmitPhiDedupesButKeepsSingleEdge(t *testing.T) {
	_, builder := newTestBuilder(t)
	a := builder.EmitConstInst(1)

	phi := builder.EmitPhi("a", []Value{a, a, nil})
	require.NotNil(t, phi)
	require.Equal(t, []int64{a.GetId()}, phi.Edge)
	require.Nil(t, builder.EmitPhi("empty", nil))
}
