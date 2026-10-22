package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestResidentIndexConcurrentAppendPreservesEveryID(t *testing.T) {
	index := utils.NewSafeMapWithKey[string, []int64]()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 256; i++ {
				appendResidentIndex(index, "shared", int64(worker*256+i+1))
			}
		}(worker)
	}
	wg.Wait()
	ids, ok := index.Get("shared")
	require.True(t, ok)
	require.Len(t, ids, 16*256)
	seen := make(map[int64]bool)
	for _, id := range ids {
		require.False(t, seen[id], "duplicated ID %d", id)
		seen[id] = true
	}
}

func TestResidentIndexRemovePreservesReaderSnapshot(t *testing.T) {
	index := utils.NewSafeMapWithKey[string, []int64]()
	for id := int64(1); id <= 4; id++ {
		appendResidentIndex(index, "x", id)
	}
	snapshot, _ := index.Get("x")
	removeResidentIndex(index, "x", 2)
	appendResidentIndex(index, "x", 5)
	require.Equal(t, []int64{1, 2, 3, 4}, snapshot)
	current, _ := index.Get("x")
	require.Equal(t, []int64{1, 3, 4, 5}, current)
}

func TestResidentIndexRemoveMissKeepsPublishedSlice(t *testing.T) {
	index := utils.NewSafeMapWithKey[string, []int64]()
	appendResidentIndex(index, "x", 1)
	appendResidentIndex(index, "x", 1)
	appendResidentIndex(index, "x", 2)
	before, ok := index.Get("x")
	require.True(t, ok)

	removeResidentIndex(index, "missing", 1)
	_, created := index.Get("missing")
	require.False(t, created)

	removeResidentIndex(index, "x", 99)
	afterMiss, _ := index.Get("x")
	require.Equal(t, &before[0], &afterMiss[0], "a miss must not publish another backing array")

	removeResidentIndex(index, "x", 1)
	afterHit, _ := index.Get("x")
	require.Equal(t, []int64{1, 1, 2}, before)
	require.Equal(t, []int64{2}, afterHit)
	require.NotEqual(t, &before[0], &afterHit[0])
}

func BenchmarkResidentIndexRemoveHit(b *testing.B) {
	index := utils.NewSafeMapWithKey[string, []int64]()
	const size = 256
	for id := int64(1); id <= size; id++ {
		appendResidentIndex(index, "x", id)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := int64(i%size) + 1
		removeResidentIndex(index, "x", id)
		appendResidentIndex(index, "x", id)
	}
}

func BenchmarkResidentIndexRemoveMiss(b *testing.B) {
	index := utils.NewSafeMapWithKey[string, []int64]()
	for id := int64(1); id <= 256; id++ {
		appendResidentIndex(index, "x", id)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		removeResidentIndex(index, "x", 10_000)
	}
}

func TestBlueprintFieldReplaceMissDoesNotCopy(t *testing.T) {
	fields := newBlueprintMemberMap()
	from := NewConst("from")
	to := NewConst("to")
	other := NewConst("other")
	from.SetId(10)
	to.SetId(11)
	other.SetId(12)
	fields.Set("hit", []Value{other, from, nil, from})
	fields.Set("miss", []Value{other, nil})

	missBefore, _ := fields.Get("miss")
	hitBefore, _ := fields.Get("hit")
	replaceBlueprintFieldReference(fields, from, to)

	missAfter, _ := fields.Get("miss")
	hitAfter, _ := fields.Get("hit")
	require.Equal(t, &missBefore[0], &missAfter[0])
	require.Equal(t, []Value{other, from, nil, from}, hitBefore)
	require.Equal(t, []Value{other, to, nil, to}, hitAfter)
}
