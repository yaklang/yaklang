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
