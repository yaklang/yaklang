package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func lookupMemoryFixture() (*ProgramCache, Value, Value) {
	resident := utils.NewSafeMapWithKey[int64, Instruction]()
	a, b := NewConst(1), NewConst(2)
	a.SetId(1)
	b.SetId(2)
	resident.Set(1, a)
	resident.Set(2, b)
	return &ProgramCache{instructions: &instructionStore{resident: resident}}, a, b
}

func TestInstructionLookupSemantics(t *testing.T) {
	c, a, b := lookupMemoryFixture()
	require.Equal(t, []Value{b, a, b}, GetExs(c, ToValue, 0, 2, 99, 1, 2))
	require.Nil(t, GetExs[Value](nil, ToValue, 1))
	require.NotNil(t, GetExs(c, ToValue, 0, 99))
	require.Empty(t, GetExs(c, ToValue, 0, 99))
	v, ok := GetEx(c, ToValue, 1)
	require.True(t, ok)
	require.Same(t, a, v)
	_, ok = GetEx(c, ToValue, 0)
	require.False(t, ok)
	_, ok = GetEx(c, ToValue, 99)
	require.False(t, ok)
	// A successful conversion to a typed nil still counts as missing for GetEx.
	_, ok = GetEx(c, func(Instruction) (*ConstInst, bool) { return nil, true }, 1)
	require.False(t, ok)
	// Cover sees a missing nonzero ID, but zero IDs are always skipped.
	calls := 0
	cover := func(i Instruction) (Instruction, bool) { calls++; return i, true }
	require.Equal(t, []Instruction{nil, b}, GetExs(c, cover, 0, 99, 2))
	require.Equal(t, 2, calls)
}

var lookupResult []Value
var lookupSingle Value

func TestInstructionLookupAllocations(t *testing.T) {
	c, _, _ := lookupMemoryFixture()
	t.Run("single", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() { lookupSingle, _ = GetEx(c, ToValue, 1) })
		require.Zero(t, allocs, "one ID must not allocate a temporary slice")
	})
	t.Run("batch", func(t *testing.T) {
		ids := make([]int64, 64)
		for i := range ids {
			ids[i] = 1
		}
		allocs := testing.AllocsPerRun(100, func() { lookupResult = GetExs(c, ToValue, ids...) })
		require.LessOrEqual(t, allocs, float64(1), "one result slice is sufficient")
	})
}

func BenchmarkInstructionLookup(b *testing.B) {
	c, _, _ := lookupMemoryFixture()
	b.Run("single", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			lookupSingle, _ = GetEx(c, ToValue, 1)
		}
	})
	b.Run("batch64", func(b *testing.B) {
		ids := make([]int64, 64)
		for i := range ids {
			ids[i] = 1
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			lookupResult = GetExs(c, ToValue, ids...)
		}
	})
}
