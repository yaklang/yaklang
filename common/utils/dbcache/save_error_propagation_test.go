package dbcache_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// TestSaveErrorPropagation_BarrierReturnsError proves that Barrier
// returns the save error immediately, not after processing 729 more batches.
func TestSaveErrorPropagation_BarrierReturnsError(t *testing.T) {
	var callCount atomic.Int64
	wantErr := errors.New("UNIQUE constraint failed")
	saveFn := func(items []*flushItem) error {
		n := callCount.Add(1)
		if n == 2 {
			return wantErr
		}
		return nil
	}

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0, 0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		saveFn,
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	// First save succeeds
	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	require.NoError(t, cache.Barrier())

	// Second save fails
	cache.Set(&flushItem{id: 2})
	cache.MarkDirty([]int64{2}, utils.EvictionReasonCapacityReached)
	err := cache.Barrier()
	require.Error(t, err, "Barrier must return error from failed save")

	// Third save: Barrier should return the recorded error immediately
	// without waiting for the save to succeed (since saver is in failed state)
	cache.Set(&flushItem{id: 3})
	cache.MarkDirty([]int64{3}, utils.EvictionReasonCapacityReached)
	err2 := cache.Barrier()
	require.Error(t, err2, "Barrier must return error immediately after saver failure")
}

// TestSaveErrorPropagation_StopsAfterFirstError proves that after the first
// save error, the saver stops processing new batches (no infinite retry).
func TestSaveErrorPropagation_StopsAfterFirstError(t *testing.T) {
	var callCount atomic.Int64
	saveFn := func(items []*flushItem) error {
		n := callCount.Add(1)
		if n >= 2 {
			return errors.New("UNIQUE constraint failed")
		}
		return nil
	}

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0, 0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		saveFn,
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	// Trigger first save (succeeds)
	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	cache.Barrier()

	// Trigger second save (fails)
	cache.Set(&flushItem{id: 2})
	cache.MarkDirty([]int64{2}, utils.EvictionReasonCapacityReached)
	cache.Barrier()

	// Trigger more saves — saver should NOT process them
	for i := int64(3); i <= 100; i++ {
		cache.Set(&flushItem{id: i})
		cache.MarkDirty([]int64{i}, utils.EvictionReasonCapacityReached)
	}
	cache.Barrier()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Total save calls should be low (not 100+)
	totalCalls := callCount.Load()
	t.Logf("Total save calls after error: %d (should be small, not 100+)", totalCalls)
	require.Less(t, totalCalls, int64(10),
		"saver should stop processing after first error, not continue retrying (got %d calls)", totalCalls)
}
