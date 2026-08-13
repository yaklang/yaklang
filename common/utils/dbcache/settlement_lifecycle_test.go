package dbcache_test

import (
	"errors"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- from save_failure_settlement_test.go ---

// settleItem is a minimal MemoryItem used by the save-failure lifecycle tests.
type settleItem struct{ id int64 }

func (i *settleItem) GetId() int64   { return i.id }
func (i *settleItem) SetId(id int64) { i.id = id }

var errSettleSave = errors.New("settle save failure")

// startSettleFailureCache builds a cache whose first save batch blocks until
// every item has been handed to the saver and then fails. WithSaveSize clamps
// to defaultBatchSize (900), so any items beyond the first batch sit in the
// saver buffer when the failure lands — exactly the state that must still be
// settled once each.
func startSettleFailureCache(t *testing.T, total int) (*dbcache.Cache[*settleItem, *settleItem], []int64, chan struct{}) {
	t.Helper()
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var once sync.Once

	cache := dbcache.NewCache[*settleItem, *settleItem](
		0, 0,
		func(item *settleItem, _ utils.EvictionReason) (*settleItem, error) {
			return item, nil
		},
		func(items []*settleItem) error {
			once.Do(func() { close(saveStarted) })
			<-releaseSave
			return errSettleSave
		},
		nil,
		dbcache.WithSaveSize(900),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)

	ids := make([]int64, 0, total)
	for i := int64(1); i <= int64(total); i++ {
		cache.Set(&settleItem{id: i})
		ids = append(ids, i)
	}
	cache.Evict(ids, utils.EvictionReasonCapacityReached)

	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first save batch never started")
	}
	require.Eventually(t, func() bool {
		return cache.Stats().Saver.Pending == int64(total)
	}, 5*time.Second, 10*time.Millisecond,
		"every item must be handed to the saver before the failure lands")
	return cache, ids, releaseSave
}

// TestSaveFailure_FlushSettlesAllPendingNoHang proves the settlement
// invariant that a save failure can never leave a pending request unsettled.
// FlushKeys only returns once the resident persist WaitGroup reaches zero, so
// a return inside the timeout is the exact "persistWG returned to zero"
// assertion; Close must then surface the recorded save error.
func TestSaveFailure_FlushSettlesAllPendingNoHang(t *testing.T) {
	const total = 2000
	cache, ids, release := startSettleFailureCache(t, total)

	done := make(chan struct{})
	go func() {
		cache.FlushKeys(ids, utils.EvictionReasonCapacityReached)
		close(done)
	}()
	close(release)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("FlushKeys hung after save failure: pending requests were dropped without settlement")
	}

	require.Error(t, cache.Close(), "Close must surface the recorded save error")
}

// --- from save_failure_barrier_settlement_test.go ---

// TestSaveFailure_BarrierSettlesEveryPendingExactlyOnce drives the same
// deterministic save failure through Barrier. Every pending request must be
// settled exactly once, the persist WaitGroup must return to zero (Barrier
// only returns after resident.Wait), and Barrier must surface the save error
// instead of hanging.
func TestSaveFailure_BarrierSettlesEveryPendingExactlyOnce(t *testing.T) {
	const total = 2000
	cache, ids, release := startSettleFailureCache(t, total)

	var mu sync.Mutex
	settled := make(map[int64]int)
	cache.SetSettlementCallback(func(key int64, _ uint64, _ dbcache.PersistOutcome) {
		mu.Lock()
		settled[key]++
		mu.Unlock()
	})

	done := make(chan error, 1)
	go func() { done <- cache.Barrier() }()
	close(release)

	var err error
	select {
	case err = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Barrier hung after save failure: pending requests were dropped without settlement")
	}
	require.Error(t, err, "Barrier must surface the recorded save error")

	mu.Lock()
	defer mu.Unlock()
	for _, id := range ids {
		require.Equal(t, 1, settled[id], "key %d must settle exactly once", id)
	}
	_ = cache.Close()
}

// --- from save_error_propagation_test.go ---

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

// --- from settlement_callback_test.go ---

// TestSettlementCallback_StaleGeneration proves that when SnapshotForPersist
// returns false (stale generation), the settlement callback is called
// with outcome=stale, and the key is unreserved.
func TestSettlementCallback_StaleGeneration(t *testing.T) {
	var settlements []dbcache.PersistSettlement
	var mu sync.Mutex

	saveFn := func(items []*flushItem) error { return nil }

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

	cache.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
		mu.Lock()
		settlements = append(settlements, dbcache.PersistSettlement{Key: key, Generation: generation, Outcome: outcome})
		mu.Unlock()
	})

	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	// Regular Set breaks generation → SnapshotForPersist returns false → stale
	cache.Set(&flushItem{id: 1})
	require.NoError(t, cache.Barrier())

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, settlements, "settlement callback must be called")
	hasStale := false
	for _, s := range settlements {
		if s.Outcome == dbcache.PersistStale {
			hasStale = true
		}
	}
	require.True(t, hasStale, "settlement must include at least one stale outcome")
}

// TestSettlementCallback_SaveFailed proves that when save fails,
// the settlement callback is called with outcome=failed.
func TestSettlementCallback_SaveFailed(t *testing.T) {
	var settlements []dbcache.PersistSettlement
	var mu sync.Mutex
	var firstSave atomic.Bool

	saveFn := func(items []*flushItem) error {
		if firstSave.CompareAndSwap(false, true) {
			return errSettlement
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

	cache.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
		mu.Lock()
		settlements = append(settlements, dbcache.PersistSettlement{Key: key, Generation: generation, Outcome: outcome})
		mu.Unlock()
	})

	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	err := cache.Barrier()
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	hasFailed := false
	for _, s := range settlements {
		if s.Outcome == dbcache.PersistFailed {
			hasFailed = true
		}
	}
	require.True(t, hasFailed, "settlement must include failed outcome on save error")
}

// TestSettlementCallback_Success proves that on successful save,
// the settlement callback is called with outcome=success.
func TestSettlementCallback_Success(t *testing.T) {
	var settlements []dbcache.PersistSettlement
	var mu sync.Mutex

	saveFn := func(items []*flushItem) error { return nil }

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

	cache.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
		mu.Lock()
		settlements = append(settlements, dbcache.PersistSettlement{Key: key, Generation: generation, Outcome: outcome})
		mu.Unlock()
	})

	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	require.NoError(t, cache.Barrier())

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, settlements, "settlement callback must be called")
	hasSuccess := false
	for _, s := range settlements {
		if s.Outcome == dbcache.PersistSuccess {
			hasSuccess = true
		}
	}
	require.True(t, hasSuccess, "settlement must include success outcome")
}

func TestSettlementCallback_MarshalFailed(t *testing.T) {
	var settlements []dbcache.PersistSettlement
	var mu sync.Mutex

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return nil, errSettlement
		},
		func(items []*flushItem) error { return nil },
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)

	cache.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
		mu.Lock()
		settlements = append(settlements, dbcache.PersistSettlement{Key: key, Generation: generation, Outcome: outcome})
		mu.Unlock()
	})

	cache.Set(&flushItem{id: 1})
	err := cache.Close()
	require.ErrorIs(t, err, errSettlement)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, settlements, 1)
	require.Equal(t, dbcache.PersistMarshalFailed, settlements[0].Outcome)
}

func TestSettlementCallback_Rejected(t *testing.T) {
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var firstSave atomic.Bool
	var settlements []dbcache.PersistSettlement
	var mu sync.Mutex

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		func(items []*flushItem) error {
			if firstSave.CompareAndSwap(false, true) {
				close(saveStarted)
				<-releaseSave
			}
			return nil
		},
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
		dbcache.WithPersistLimit(1),
	)

	cache.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
		mu.Lock()
		settlements = append(settlements, dbcache.PersistSettlement{Key: key, Generation: generation, Outcome: outcome})
		mu.Unlock()
	})

	cache.Set(&flushItem{id: 1})
	cache.Set(&flushItem{id: 2})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}
	cache.MarkDirty([]int64{2}, utils.EvictionReasonCapacityReached)

	mu.Lock()
	var rejected int
	for _, settlement := range settlements {
		if settlement.Outcome == dbcache.PersistRejected {
			rejected++
		}
	}
	mu.Unlock()
	require.Equal(t, 1, rejected)

	close(releaseSave)
	require.NoError(t, cache.Barrier())
	require.NoError(t, cache.Close())
}

type versionedItem struct {
	id      int64
	version int
}

func (i *versionedItem) GetId() int64   { return i.id }
func (i *versionedItem) SetId(id int64) { i.id = id }

func TestUpdateWhilePending_PersistsLatestValue(t *testing.T) {
	marshalStarted := make(chan struct{})
	releaseMarshal := make(chan struct{})
	var marshalOnce sync.Once
	var saved []int
	var mu sync.Mutex

	cache := dbcache.NewCache[*versionedItem, int](
		0,
		0,
		func(item *versionedItem, _ utils.EvictionReason) (int, error) {
			marshalOnce.Do(func() { close(marshalStarted) })
			<-releaseMarshal
			return item.version, nil
		},
		func(items []int) error {
			mu.Lock()
			saved = append(saved, items...)
			mu.Unlock()
			return nil
		},
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	defer cache.Close()

	cache.Set(&versionedItem{id: 1, version: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	select {
	case <-marshalStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("marshal never started")
	}
	cache.UpdateWhilePending(1, &versionedItem{id: 1, version: 2})
	close(releaseMarshal)

	require.NoError(t, cache.Barrier())
	mu.Lock()
	got := append([]int(nil), saved...)
	mu.Unlock()
	require.Equal(t, []int{1, 2}, got)
}

var errSettlement = newSettlementError()

type settlementError struct{}

func (e *settlementError) Error() string { return "settlement test error" }
func newSettlementError() error          { return &settlementError{} }
