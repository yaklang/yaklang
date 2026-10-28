package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeSliceConcurrentAppendAndSnapshot(t *testing.T) {
	var items SafeSlice[int]
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(v int) {
			defer wg.Done()
			items.Append(v)
			_ = items.Snapshot()
		}(i)
	}
	wg.Wait()
	got := items.Snapshot()
	require.Len(t, got, n)
	seen := make(map[int]struct{}, n)
	for _, v := range got {
		seen[v] = struct{}{}
	}
	require.Len(t, seen, n)
}
