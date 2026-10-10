package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalcConstBinaryComparesByValue(t *testing.T) {
	left := NewConst(6)
	right := NewConst(int64(6))

	eq := CalcConstBinary(left, right, OpEq)
	require.NotNil(t, eq)
	require.True(t, eq.Boolean())

	ne := CalcConstBinary(left, right, OpNotEq)
	require.NotNil(t, ne)
	require.False(t, ne.Boolean())

	different := CalcConstBinary(NewConst(6), NewConst(int64(7)), OpNotEq)
	require.NotNil(t, different)
	require.True(t, different.Boolean())

	sameText := CalcConstBinary(NewConst("a"), NewConst("a"), OpEq)
	require.NotNil(t, sameText)
	require.True(t, sameText.Boolean())
	differentText := CalcConstBinary(NewConst("a"), NewConst("b"), OpNotEq)
	require.NotNil(t, differentText)
	require.True(t, differentText.Boolean())
}
