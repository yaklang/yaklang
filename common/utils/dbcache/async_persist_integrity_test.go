package dbcache_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// TestAsyncPersist_BarrierPersistsAllDirty proves that after MarkDirty +
// Barrier, ALL unique dirty items are persisted to the save function.
// This catches bugs where Barrier doesn't wait for inflight batches.
func TestAsyncPersist_BarrierPersistsAllDirty(t *testing.T) {
	var savedIDs sync.Map // track which IDs were saved
	var savedCount atomic.Int64
	saveFn := func(items []*flushItem) error {
		for _, item := range items {
			savedIDs.Store(item.id, true)
		}
		savedCount.Add(int64(len(items)))
		return nil
	}

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0, 0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		saveFn,
		nil,
		dbcache.WithSaveSize(50), // small batch to create many batches
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	const N = 500
	// Set N items
	for i := int64(1); i <= N; i++ {
		cache.Set(&flushItem{id: i})
	}

	// Mark all dirty in multiple calls (simulating per-unit flush)
	for batch := 0; batch < 10; batch++ {
		start := int64(batch*50 + 1)
		end := int64(batch*50 + 50)
		if end > N {
			end = N
		}
		ids := make([]int64, 0, end-start+1)
		for i := start; i <= end; i++ {
			ids = append(ids, i)
		}
		cache.MarkDirty(ids, utils.EvictionReasonCapacityReached)
	}

	// Barrier must wait for ALL writes
	err := cache.Barrier()
	require.NoError(t, err)

	// All N items must be saved
	require.Equal(t, int64(N), savedCount.Load(),
		"all %d items must be saved after Barrier (got %d)", N, savedCount.Load())

	// Verify each ID was saved
	for i := int64(1); i <= N; i++ {
		_, ok := savedIDs.Load(i)
		require.True(t, ok, "item %d must be saved after Barrier", i)
	}

	// Stats: enqueue count should match
	stats := cache.FlushKeysStats()
	require.Equal(t, int64(N), stats.EnqueueCount,
		"enqueue_count must equal total unique items (%d), got %d", N, stats.EnqueueCount)
}

// TestAsyncPersist_BarrierQueueEmptyAfter proves that after Barrier,
// queue depth and pending are 0.
func TestAsyncPersist_BarrierQueueEmptyAfter(t *testing.T) {
	var savedCount atomic.Int64
	saveFn := func(items []*flushItem) error {
		savedCount.Add(int64(len(items)))
		return nil
	}

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0, 0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		saveFn,
		nil,
		dbcache.WithSaveSize(100),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	for i := int64(1); i <= 200; i++ {
		cache.Set(&flushItem{id: i})
	}

	cache.MarkDirty(makeKeys(1, 200), utils.EvictionReasonCapacityReached)

	err := cache.Barrier()
	require.NoError(t, err)

	// After Barrier, all 200 must be saved
	require.Equal(t, int64(200), savedCount.Load(),
		"all 200 items saved after Barrier")

	// Resident count should be 0 (all evicted via MarkDirty)
	require.Equal(t, 0, cache.Count(),
		"resident count must be 0 after Barrier")
}
