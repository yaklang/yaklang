package dbcache_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// flushItem is a test item implementing dbcache.MemoryItem.
type flushItem struct{ id int64 }

func (i *flushItem) GetId() int64   { return i.id }
func (i *flushItem) SetId(id int64) { i.id = id }

// FlushStats is the per-flush observability struct that the async persist
// pipeline MUST expose. It does not exist yet; these tests will be RED until
// FlushStats and FlushKeysStats() are implemented on dbcache.Cache.
type FlushStats struct {
	FlushRequestCount     int64
	DedupSkipped          int64
	EnqueueCount          int64
	SavedDelta            int64
	ResidentBefore        int
	ResidentAfter         int
	EnqueueDuration       time.Duration
	BackpressureDuration  time.Duration
}

// TestA_DedupCountForRepeatedFlush proves that when the same keys are
// flushed twice while items are still pending, the second flush should
// count dedup_skipped > 0. Uses MarkDirtyForTest (non-blocking) to
// simulate the async scenario where items are pending but not yet persisted.
func TestA_DedupCountForRepeatedFlush(t *testing.T) {
	releaseSave := make(chan struct{})
	var firstBatch atomic.Bool
	var savedCount atomic.Int64
	saveFn := func(items []*flushItem) error {
		savedCount.Add(int64(len(items)))
		if firstBatch.CompareAndSwap(false, true) {
			<-releaseSave // block first save so items stay pending
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
		dbcache.WithSaveSize(100),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	// Set items 1..10
	for i := int64(1); i <= 10; i++ {
		cache.Set(&flushItem{id: i})
	}

	// First mark-dirty of keys 1..5 (non-blocking — items become pending)
	cache.MarkDirtyForTest([]int64{1, 2, 3, 4, 5}, utils.EvictionReasonCapacityReached)

	// Wait a moment for the marshal pipe to process
	time.Sleep(50 * time.Millisecond)

	// Second mark-dirty of the SAME keys 1..5 — items still pending → dedup
	cache.MarkDirtyForTest([]int64{1, 2, 3, 4, 5}, utils.EvictionReasonCapacityReached)

	// Release the first save
	close(releaseSave)

	// Wait for saves to complete
	time.Sleep(100 * time.Millisecond)

	stats := cache.FlushKeysStats()
	require.Greater(t, stats.DedupSkipped, int64(0),
		"second flush of same pending keys should report dedup_skipped > 0")
}

// TestB_RequestEnqueuedCompletedSeparation proves that request/enqueued/completed
// are tracked separately. SaveStats.BatchItemsTotal counts post-save items;
// FlushStats.EnqueueCount and FlushStats.SavedDelta must distinguish pre-save
// from post-save counts.
func TestB_RequestEnqueuedCompletedSeparation(t *testing.T) {
	var savedCount atomic.Int64
	saveFn := func(items []*flushItem) error {
		savedCount.Add(int64(len(items)))
		time.Sleep(5 * time.Millisecond) // simulate DB write
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

	for i := int64(1); i <= 20; i++ {
		cache.Set(&flushItem{id: i})
	}

	cache.FlushKeys([]int64{1, 2, 3, 4, 5}, utils.EvictionReasonCapacityReached)
	time.Sleep(50 * time.Millisecond) // let writer finish

	stats := cache.FlushKeysStats()

	// EnqueueCount should be > 0 (items were enqueued for save)
	require.Greater(t, stats.EnqueueCount, int64(0),
		"enqueue_count must be > 0 after flush")
	// SavedDelta should equal the number actually persisted by the writer
	require.Equal(t, int64(5), stats.SavedDelta,
		"saved_delta must equal items actually persisted")
	// After writer finishes, enqueue_count == saved_delta
	require.Equal(t, stats.EnqueueCount, stats.SavedDelta,
		"after writer finishes, enqueue_count == saved_delta")
}

// TestD_FlushKeysBlocksUntilWriterCompletes proves that the current
// FlushKeys implementation is synchronous: it blocks the caller until
// the writer has persisted all queued items. This test exposes the
// need for a non-blocking MarkDirty API. The test measures FlushKeys
// wall time and asserts it is >= the writer save time.
// This test PASSES with current code (proving the sync behavior exists)
// but documents the need for async API.
func TestD_FlushKeysBlocksUntilWriterCompletes(t *testing.T) {
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var started atomic.Bool

	saveFn := func(items []*flushItem) error {
		if started.CompareAndSwap(false, true) {
			close(saveStarted)
		}
		<-releaseSave // block save to simulate slow DB
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

	for i := int64(1); i <= 5; i++ {
		cache.Set(&flushItem{id: i})
	}

	// Start FlushKeys in a goroutine — it should block because save is blocked
	done := make(chan struct{})
	go func() {
		cache.FlushKeys([]int64{1, 2, 3, 4, 5}, utils.EvictionReasonCapacityReached)
		close(done)
	}()

	// Wait for save to start
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}

	// FlushKeys should be blocked (not yet done)
	select {
	case <-done:
		t.Fatal("FlushKeys returned before save completed — sync behavior broken")
	case <-time.After(50 * time.Millisecond):
		// Good: FlushKeys is still blocked, proving it's synchronous
	}

	// Release save
	close(releaseSave)

	// Now FlushKeys should complete
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("FlushKeys did not complete after save finished")
	}
}
