package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitVector_SetAndHas(t *testing.T) {
	bits := NewBitVector()
	bits.Set(0)
	bits.Set(63)
	bits.Set(64)

	require.True(t, bits.Has(0))
	require.True(t, bits.Has(63))
	require.True(t, bits.Has(64))
	require.False(t, bits.Has(1))
	require.False(t, bits.Has(-1))
}

func TestBitVector_CloneAndOr(t *testing.T) {
	left := NewBitVector()
	left.Set(1)
	left.Set(66)

	right := left.Clone()
	right.Set(130)

	require.True(t, left.Has(1))
	require.True(t, left.Has(66))
	require.False(t, left.Has(130))

	left.Or(right)
	require.True(t, left.Has(1))
	require.True(t, left.Has(66))
	require.True(t, left.Has(130))
}

func TestBitVector_ForEachAndEmpty(t *testing.T) {
	bits := NewBitVector()
	require.True(t, bits.IsEmpty())

	bits.Set(2)
	bits.Set(64)
	bits.Set(129)
	require.False(t, bits.IsEmpty())

	var got []int
	bits.ForEach(func(index int) {
		got = append(got, index)
	})
	require.Equal(t, []int{2, 64, 129}, got)
}
