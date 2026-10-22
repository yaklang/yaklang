package dbcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type budgetFixture struct {
	id   int64
	body string
}

func (v *budgetFixture) GetId() int64   { return v.id }
func (v *budgetFixture) SetId(id int64) { v.id = id }

// Exercise the actual Cache -> Save adapter. All requests are queued before
// Flush, and the timer cannot fire: batch boundaries depend only on bytes.
func TestCacheForwardsSerializedByteBudget(t *testing.T) {
	var batches [][]string
	cache := NewCache[*budgetFixture, string](0, 0,
		func(v *budgetFixture, _ utils.EvictionReason) (string, error) { return v.body, nil },
		func(rows []string) error {
			batches = append(batches, append([]string(nil), rows...))
			return nil
		}, nil,
		WithSaveSize(900), WithSaveTimeout(time.Hour),
		WithSaveByteBudget(10, func(v string) int { return len(v) }),
	)
	defer cache.CloseWithoutSave()
	// Testing the adapter directly avoids scheduling-dependent partial flushes
	// while the marshal pipe is still delivering requests.
	// Every ordinary record is larger than half the budget, so any batch of
	// two or more exceeds it no matter how the saver splits the queue.
	values := []string{"aaaaaa", "bbbbbb", "cccccc", "dddddd", "oversized-record"}
	for i, body := range values {
		key := int64(i + 1)
		cache.resident.Set(key, &budgetFixture{id: key, body: body})
		requests := cache.resident.MarkPending([]int64{key}, utils.EvictionReasonCapacityReached)
		require.Len(t, requests, 1)
		cache.saver.Save(&saveTask[string]{request: evictionRequest{key: key, generation: requests[0].Generation}, data: body})
	}
	require.NoError(t, cache.saver.Flush())
	var got []string
	for _, batch := range batches {
		size := 0
		for _, v := range batch {
			size += len(v)
		}
		if len(batch) > 1 {
			require.LessOrEqual(t, size, 10, "Cache must not lose the byte budget when wrapping D in saveTask[D]")
		}
		got = append(got, batch...)
	}
	require.Equal(t, values, got, "no record may be lost or reordered; an oversized record is saved alone")
	require.Zero(t, cache.resident.PendingCount())
}

// The predicate is application code: it must be able to inspect/re-enter the
// residency cache. TryLock proves lock ownership synchronously, without sleeps
// or a probabilistic race between goroutines.
func TestResidencyEvictionPredicateOutsideLock(t *testing.T) {
	for _, operation := range []string{"evict", "track", "cooldown", "finish", "restore"} {
		t.Run(operation, func(t *testing.T) {
			var cache *ResidencyCacheWithKey[int, string]
			calls := 0
			cache = NewResidencyCacheWithKey[int, string](time.Hour, 10, nil, nil, func(value string) bool {
				calls++
				unlocked := cache.mu.TryLock()
				if unlocked {
					cache.mu.Unlock()
				}
				assert.True(t, unlocked, "skipEviction must not run under the residency mutex")
				return true
			})
			defer cache.CloseWithoutSave()
			cache.data[1] = &residentItem[int, string]{key: 1, memoryItem: "pinned"}
			switch operation {
			case "evict":
				cache.handleEviction(1, utils.EvictionReasonCapacityReached)
			case "track":
				cache.TrackKeys([]int{1})
			case "cooldown":
				cache.CoolDownKeys([]int{1}, time.Hour)
			case "finish":
				cache.data[1].pending = true
				cache.data[1].generation = 1
				cache.persistWG.Add(1)
				cache.pendingCount.Add(1)
				cache.FinishPersist(1, 1, false)
			case "restore":
				cache.restoreEvictionTracking()
			}
			require.Equal(t, 1, calls)
			require.Zero(t, cache.PendingCount(), "pinned values must not be queued")
		})
	}
}
