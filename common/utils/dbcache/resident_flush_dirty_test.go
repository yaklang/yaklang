package dbcache

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResidentFlushDirtyKeysOnly(t *testing.T) {
	marshaled, saved := 0, 0
	cache := NewResidentFlushCache[int, int, int](16, func(value int, update bool) (int, bool, error) {
		marshaled++
		return value, true, nil
	}, func(batch []int) error {
		saved += len(batch)
		return nil
	})
	for key := 0; key < 1000; key++ {
		cache.Set(key, key)
	}
	require.NoError(t, cache.Flush(false))
	require.Equal(t, 1000, marshaled)
	require.Zero(t, cache.DirtyCount())
	for i := 0; i < 10; i++ {
		require.NoError(t, cache.Flush(true))
	}
	require.Equal(t, 1000, marshaled, "clean flushes must never invoke the marshal callback")
	cache.Set(5, 2000)
	require.NoError(t, cache.Flush(true))
	require.Equal(t, 1001, marshaled, "only the changed key may be visited")
	require.Equal(t, 1001, saved)
}

func TestResidentFlushDirtyRetryAndGeneration(t *testing.T) {
	failed := true
	var cache *ResidentFlushCache[int, int, int]
	var saved []int
	cache = NewResidentFlushCache[int, int, int](1, func(value int, update bool) (int, bool, error) {
		return value, true, nil
	}, func(batch []int) error {
		if failed {
			return errors.New("save rejected")
		}
		saved = append(saved, batch...)
		if batch[0] == 1 {
			cache.Set(1, 2)
		}
		return nil
	})
	cache.SetOnSaved(func(key int, _ int) { cache.DeleteClean(key) })
	cache.Set(1, 1)
	require.ErrorContains(t, cache.Flush(false), "save rejected")
	require.Equal(t, 1, cache.DirtyCount())
	require.Equal(t, 1, cache.Count())
	failed = false
	require.NoError(t, cache.Flush(false))
	require.Equal(t, 1, cache.DirtyCount(), "newer mutation must survive the old save")
	require.Equal(t, 1, cache.Count(), "a newer dirty value must not be released")
	require.NoError(t, cache.Flush(true))
	require.Equal(t, []int{1, 2}, saved)
	require.Zero(t, cache.DirtyCount())
	require.Zero(t, cache.Count())
}
