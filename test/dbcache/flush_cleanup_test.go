package dbcache_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// TestQueueKeys_FinishPersistRemovesFromResident verifies that after
// QueueKeys marks a set of keys for persistence and the enqueuePersist
// callback settles each one via FinishPersist(success=true), the items
// are removed from the resident cache. GetResident must report false for
// every flushed key while non-queued keys must remain resident.
func TestQueueKeys_FinishPersistRemovesFromResident(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0, // no TTL, no capacity — items stay resident until explicitly flushed
		0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			value, ok := cache.SnapshotForPersist(key, generation)
			if ok {
				database.Set(key, value)
			}
			cache.FinishPersist(key, generation, true)
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	// Seed resident items.
	cache.Set(1, "one")
	cache.Set(2, "two")
	cache.Set(3, "three")

	// Sanity: all three are resident before flush.
	for _, k := range []int{1, 2, 3} {
		_, ok := cache.GetResident(k)
		require.True(t, ok, "key %d should be resident before QueueKeys", k)
	}

	// Flush only keys 1 and 3 via QueueKeys; key 2 must stay resident.
	cache.QueueKeys([]int{1, 3}, utils.EvictionReasonDeleted)

	// QueueKeys runs synchronously with enqueuePersist, which calls
	// FinishPersist inline, so removal is observable immediately.
	_, ok := cache.GetResident(1)
	require.False(t, ok, "key 1 should be removed from resident cache after FinishPersist")
	_, ok = cache.GetResident(3)
	require.False(t, ok, "key 3 should be removed from resident cache after FinishPersist")

	// Key 2 was not queued and must remain resident.
	value, ok := cache.GetResident(2)
	require.True(t, ok, "key 2 should still be resident (not queued for flush)")
	require.Equal(t, "two", value)

	// Persisted values should be present in the backing store.
	v, ok := database.Get(1)
	require.True(t, ok)
	require.Equal(t, "one", v)
	v, ok = database.Get(3)
	require.True(t, ok)
	require.Equal(t, "three", v)
	_, ok = database.Get(2)
	require.False(t, ok, "key 2 should not have been persisted")

	// Resident count should reflect the single remaining item.
	require.Equal(t, 1, cache.Count())
}

// TestQueueKeys_FinishPersistAsyncRemoval verifies the same removal
// contract when FinishPersist is called asynchronously (simulating a
// background save goroutine) rather than inline inside enqueuePersist.
//
// We deliberately avoid GetResident during the in-flight window because
// GetResident clears the pending flag, which would race with the async
// FinishPersist that checks the pending/generation pair. Instead we use
// Count() and GetAll() (which do not mutate pending state) to observe
// residence, and only call GetResident after settlement completes.
func TestQueueKeys_FinishPersistAsyncRemoval(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	var pendingTasks []dbcache.PersistRequest[int]
	var mu sync.Mutex

	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0,
		0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			// Defer settlement so FinishPersist happens off the QueueKeys path.
			mu.Lock()
			pendingTasks = append(pendingTasks, dbcache.PersistRequest[int]{
				Key:        key,
				Generation: generation,
				Reason:     reason,
			})
			mu.Unlock()
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(10, "ten")
	cache.Set(20, "twenty")
	cache.Set(30, "thirty")

	// Items are pending but not yet settled — all three remain resident.
	// Use Count (non-mutating) to confirm nothing has been removed yet.
	cache.QueueKeys([]int{10, 30}, utils.EvictionReasonDeleted)
	require.Equal(t, 3, cache.Count(), "all items remain resident while persist is in-flight")
	require.Equal(t, int64(2), cache.PendingCount(), "two items should be pending")

	// Drain the pending tasks asynchronously, persisting and settling each.
	mu.Lock()
	tasks := append([]dbcache.PersistRequest[int](nil), pendingTasks...)
	mu.Unlock()

	var settled atomic.Int32
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task dbcache.PersistRequest[int]) {
			defer wg.Done()
			if value, ok := cache.SnapshotForPersist(task.Key, task.Generation); ok {
				database.Set(task.Key, value)
			}
			cache.FinishPersist(task.Key, task.Generation, true)
			settled.Add(1)
		}(task)
	}
	wg.Wait()

	require.Equal(t, int32(2), settled.Load())

	// After settlement, queued keys must be gone from resident cache.
	_, ok := cache.GetResident(10)
	require.False(t, ok, "key 10 should be removed after async FinishPersist")
	_, ok = cache.GetResident(30)
	require.False(t, ok, "key 30 should be removed after async FinishPersist")

	// The never-queued key must still be resident.
	value, ok := cache.GetResident(20)
	require.True(t, ok, "key 20 should remain resident (never queued)")
	require.Equal(t, "twenty", value)

	require.Equal(t, 1, cache.Count())
}

// TestQueueKeys_FlushFailureKeepsResidentAndRetry verifies that when
// FinishPersist is called with success=false, the item remains in the
// resident cache with pending=false, and can be re-queued for another
// flush attempt that succeeds.
func TestQueueKeys_FlushFailureKeepsResidentAndRetry(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	var flushAttempts atomic.Int32

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0, 0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			attempt := flushAttempts.Add(1)
			if attempt == 1 {
				// First attempt: simulate save failure
				cache.FinishPersist(key, generation, false)
				return true
			}
			// Second attempt: succeed
			if value, ok := cache.SnapshotForPersist(key, generation); ok {
				database.Set(key, value)
			}
			cache.FinishPersist(key, generation, true)
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(100, "hundred")

	// First flush: fails, item stays resident
	cache.QueueKeys([]int{100}, utils.EvictionReasonDeleted)
	_, ok := cache.GetResident(100)
	require.True(t, ok, "item should remain resident after flush failure")
	require.Equal(t, 1, cache.Count(), "item should still be in resident cache")
	_, ok = database.Get(100)
	require.False(t, ok, "item should NOT be in backing store after failed flush")

	// Second flush: succeeds, item removed
	// Note: flushAttempts is NOT reset — the next call will see attempt=2 (succeed)
	cache.QueueKeys([]int{100}, utils.EvictionReasonDeleted)
	_, ok = cache.GetResident(100)
	require.False(t, ok, "item should be removed after successful retry flush")
	require.Equal(t, 0, cache.Count(), "resident cache should be empty")
	v, ok := database.Get(100)
	require.True(t, ok, "item should be in backing store after successful retry")
	require.Equal(t, "hundred", v)
}

// TestSetDuringFlushOldGenDoesNotDeleteNewValue verifies that when a
// Set() occurs while a flush (QueueKeys) is in-flight for the same key:
//  1. The old generation's FinishPersist(success=true) must NOT delete
//     the new value (generation mismatch protects it).
//  2. The new value remains resident and can be persisted by a subsequent flush.
func TestSetDuringFlushOldGenDoesNotDeleteNewValue(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	var flushCount atomic.Int32

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0, 0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			value, ok := cache.SnapshotForPersist(key, generation)
			if ok {
				database.Set(key, value)
			}

			if flushCount.Add(1) == 1 {
				// First flush: simulate a concurrent Set BEFORE FinishPersist.
				// This increments the generation, so FinishPersist with the old
				// generation should be a no-op (item not deleted).
				cache.Set(key, "newValue")
				cache.FinishPersist(key, generation, true)
				// FinishPersist with mismatched gen → no-op, item stays.
			} else {
				// Second flush: normal persist, no Set during flush.
				cache.FinishPersist(key, generation, true)
			}
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(200, "original")

	// First flush: during the callback, Set("newValue") is called.
	// The old generation's FinishPersist must not delete the new value.
	cache.QueueKeys([]int{200}, utils.EvictionReasonDeleted)

	// The new value should still be resident (old gen didn't delete it)
	value, ok := cache.GetResident(200)
	require.True(t, ok, "new value should still be resident after Set during flush")
	require.Equal(t, "newValue", value, "new value should be 'newValue'")

	// The backing store has the old value (persisted before Set)
	v, ok := database.Get(200)
	require.True(t, ok)
	require.Equal(t, "original", v, "backing store should have the old value")

	// Second flush: persist the new value (no Set during this flush)
	cache.QueueKeys([]int{200}, utils.EvictionReasonDeleted)

	// After second flush, the new value should be in the backing store
	// and removed from resident.
	v, ok = database.Get(200)
	require.True(t, ok, "new value should be in backing store after second flush")
	require.Equal(t, "newValue", v, "backing store should have 'newValue'")

	_, ok = cache.GetResident(200)
	require.False(t, ok, "item should be removed from resident after second flush")
	require.Equal(t, 0, cache.Count())
}

// TestFlushAllResidentDoesNotLeak verifies that flushing all resident
// items removes every single one, and the resident count reaches zero.
// This tests the "Flush/Close doesn't leak resident" requirement.
func TestFlushAllResidentDoesNotLeak(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0, 0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			if value, ok := cache.SnapshotForPersist(key, generation); ok {
				database.Set(key, value)
			}
			cache.FinishPersist(key, generation, true)
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	// Seed many items
	for i := 1; i <= 100; i++ {
		cache.Set(i, fmt.Sprintf("value-%d", i))
	}
	require.Equal(t, 100, cache.Count())

	// Flush all
	allKeys := make([]int, 100)
	for i := 0; i < 100; i++ {
		allKeys[i] = i + 1
	}
	cache.QueueKeys(allKeys, utils.EvictionReasonDeleted)

	// All should be removed from resident
	require.Equal(t, 0, cache.Count(), "no items should remain resident after flushing all")

	// All should be in the backing store
	for i := 1; i <= 100; i++ {
		v, ok := database.Get(i)
		require.True(t, ok, "key %d should be in backing store", i)
		require.Equal(t, fmt.Sprintf("value-%d", i), v)
	}
}
