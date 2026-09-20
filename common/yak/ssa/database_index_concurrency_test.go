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
