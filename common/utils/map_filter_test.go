package utils

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeMapValuesMatching(t *testing.T) {
	m := NewSafeMapWithKey[string, int]()
	m.Set("keep", 1)
	m.Set("drop", 2)
	require.Equal(t, []int{1}, m.ValuesMatching(func(k string) bool { return k == "keep" }))
	require.Empty(t, m.ValuesMatching(func(string) bool { return false }))
	// The snapshot survives subsequent map writes and can be processed unlocked.
	values := m.ValuesMatching(func(string) bool { return true })
	m.Clear()
	require.ElementsMatch(t, []int{1, 2}, values)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Set("key", j)
				m.ValuesMatching(func(k string) bool { return k == "key" })
			}
		}()
	}
	wg.Wait()
	require.Zero(t, testing.AllocsPerRun(100, func() {
		m.ValuesMatching(func(string) bool { return false })
	}), "misses must not allocate a whole-map snapshot")
}

func BenchmarkSafeMapSelectiveSnapshot(b *testing.B) {
	m := NewSafeMapWithKey[string, []int64]()
	for i := 0; i < 100000; i++ {
		m.Set(fmt.Sprint(i), []int64{int64(i)})
	}
	match := func(k string) bool { return k == "100" }
	b.Run("whole-map", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.ForEach(func(k string, v []int64) bool { _ = match(k); return true })
		}
	})
	b.Run("matching-values", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.ValuesMatching(match)
		}
	})
}
