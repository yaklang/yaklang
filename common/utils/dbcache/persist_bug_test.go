package dbcache

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type bugItem struct{ id int64 }

func (i *bugItem) GetId() int64   { return i.id }
func (i *bugItem) SetId(id int64) { i.id = id }

// TestPersistLimitCloseWithExistingPending reproduces the large-project bug:
// items evicted by TTL during compile create pending backlog that exceeds
// persistLimit. When close arrives, enqueueCloseRequests batches interact
// with the existing pending count → exceed persistLimit → rejection.
//
// On main: FAILS — Close returns "resident items were not persisted"
// With closing flag fix: PASSES — all items flush correctly
func TestPersistLimitCloseWithExistingPending(t *testing.T) {
	const total = 30
	var savedCount atomic.Int32
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var started atomic.Bool

	cache := NewCache[*bugItem, int](
		5*time.Millisecond,  // short TTL — trigger evictions quickly
		0,
		func(item *bugItem, _ utils.EvictionReason) (int, error) {
			return int(item.id), nil
		},
		func(items []int) error {
			if started.CompareAndSwap(false, true) {
				close(saveStarted)
			}
			<-releaseSave // block save — items stay pending
			savedCount.Add(int32(len(items)))
			return nil
		},
		nil,
		WithSaveTimeout(10*time.Millisecond),
		WithSaveSize(1),
		WithPersistLimit(2), // very low persist limit
	)

	// Insert items — TTL evictions will start quickly
	for i := 1; i <= total; i++ {
		cache.Set(&bugItem{id: int64(i)})
	}

	// Wait for at least one save to be triggered (items are pending)
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}

	// Now many items are in the pending queue (save is blocked).
	// pendingCount >> persistLimit(2).
	// Close the cache while items are still pending.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- cache.Close()
	}()

	// Give enqueueCloseRequests time to try processing
	time.Sleep(100 * time.Millisecond)

	// Release the save — allow everything to drain
	close(releaseSave)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error (bug on main): %v", err)
		}
		t.Logf("all items persisted correctly — fix verified")
	case <-time.After(5 * time.Second):
		t.Fatal("Close() hung")
	}

	if int(savedCount.Load()) != total {
		t.Fatalf("not all items saved: %d of %d", savedCount.Load(), total)
	}
}
