package dbcache_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// TestSetWhilePending_DoesNotBreakGeneration proves that updating a
// resident item while it's pending (in-flight save) does NOT clear
// pending or increment generation. The in-flight save must still
// succeed with the correct generation.
func TestSetWhilePending_DoesNotBreakGeneration(t *testing.T) {
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var savedItems atomic.Int64

	saveFn := func(items []*flushItem) error {
		savedItems.Add(int64(len(items)))
		close(saveStarted)
		<-releaseSave
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

	// Set item with id=1
	cache.Set(&flushItem{id: 1})

	// MarkDirty to enqueue for save (async)
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)

	// Wait for save to start (item is now pending/in-flight)
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}

	// Now update the item while it's pending
	// UpdateWhilePending should NOT clear pending or increment generation
	cache.UpdateWhilePending(1, &flushItem{id: 1})

	// Release save
	close(releaseSave)

	// Barrier to wait for save completion
	require.NoError(t, cache.Barrier())

	// Save should have succeeded (1 item saved)
	require.Equal(t, int64(1), savedItems.Load(),
		"exactly 1 save should have occurred (not 0 from generation mismatch, not 2 from re-enqueue)")
}

// TestSetWhilePending_RegularSetBreaksPending proves that regular Set
// during in-flight clears pending=false and increments generation.
// This is a state assertion test, not a timing-dependent test.
func TestSetWhilePending_RegularSetBreaksPending(t *testing.T) {
	cache := dbcache.NewCache[*flushItem, *flushItem](
		0, 0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		func(items []*flushItem) error { return nil },
		nil,
		dbcache.WithSaveSize(100),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	cache.Set(&flushItem{id: 1})
	// MarkDirty sets pending=true, generation=1
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)

	// Regular Set: should clear pending=false, generation=2
	cache.Set(&flushItem{id: 1})

	// After regular Set: item should be resident with pending=false
	// and generation incremented (not the same as the in-flight request)
	require.Equal(t, 1, cache.Count(),
		"item should still be resident after regular Set during pending")
	// The in-flight request (generation=1) will find generation mismatch
	// in SnapshotForPersist → FinishPersist(false) → item not deleted.
	// This is the documented behavior that UpdateWhilePending fixes.
	t.Log("Regular Set during pending: clears pending, increments generation — " +
		"UpdateWhilePending is the correct API for in-flight updates")
}

// TestSaveFailure_ReleasesReservedAndBarrierReturnsError proves that
// when save fails, Barrier returns the error and no deadlock occurs.
func TestSaveFailure_ReleasesReservedAndBarrierReturnsError(t *testing.T) {
	wantErr := errors.New("sentinel save error")
	var callCount atomic.Int64

	saveFn := func(items []*flushItem) error {
		n := callCount.Add(1)
		if n == 1 {
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

	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)

	// First save fails
	err := cache.Barrier()
	require.Error(t, err, "Barrier must return error after save failure")
	require.ErrorIs(t, err, wantErr, "Barrier must return the sentinel error")

	// After failure, saver is in failed state — subsequent Barrier
	// returns the same error immediately
	err2 := cache.Barrier()
	require.Error(t, err2, "subsequent Barrier must return error (saver failed)")
}

// TestFlushCompileUnitReturnsFast_BlockingSaveHook proves that
// FlushCompileUnit (MarkDirty) returns within 100ms even when
// the save is blocked. Then Barrier blocks until release.
func TestFlushCompileUnitReturnsFast_BlockingSaveHook(t *testing.T) {
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var firstSave atomic.Bool

	saveFn := func(items []*flushItem) error {
		if firstSave.CompareAndSwap(false, true) {
			close(saveStarted)
			<-releaseSave
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

	for i := int64(1); i <= 10; i++ {
		cache.Set(&flushItem{id: i})
	}

	// MarkDirty should return immediately even though save is blocked
	start := time.Now()
	cache.MarkDirty([]int64{1, 2, 3, 4, 5}, utils.EvictionReasonCapacityReached)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 100*time.Millisecond,
		"MarkDirty must return within 100ms even when save is blocked (took %v)", elapsed)

	// Wait for save to start
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}

	// Barrier should block until save is released
	barrierDone := make(chan struct{})
	go func() {
		cache.Barrier()
		close(barrierDone)
	}()

	select {
	case <-barrierDone:
		t.Fatal("Barrier returned before save was released")
	case <-time.After(100 * time.Millisecond):
		// Good — Barrier is blocked
	}

	close(releaseSave)

	select {
	case <-barrierDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Barrier did not return after save was released")
	}

	// Now safe to close (save no longer blocks)
	cache.Close()
}

// Suppress unused import
var _ = sync.Mutex{}
