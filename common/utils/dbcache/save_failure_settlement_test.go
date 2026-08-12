package dbcache_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

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
