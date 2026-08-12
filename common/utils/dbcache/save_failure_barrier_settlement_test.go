package dbcache_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

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
