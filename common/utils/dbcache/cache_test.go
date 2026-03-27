package dbcache_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

type closeFlushItem struct{ id int64 }

func (i *closeFlushItem) GetId() int64   { return i.id }
func (i *closeFlushItem) SetId(id int64) { i.id = id }

func TestResidencyCache(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := 80 * time.Millisecond

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
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

	cache.Set(1, "1")
	cache.Set(2, "2")

	time.Sleep(2 * ttl)

	value1, ok := database.Get(1)
	require.True(t, ok)
	require.Equal(t, "1", value1)

	value2, ok := database.Get(2)
	require.True(t, ok)
	require.Equal(t, "2", value2)

	_, ok = cache.GetResident(1)
	require.False(t, ok)

	loaded, ok := cache.Get(1)
	require.True(t, ok)
	require.Equal(t, "1", loaded)
}

func TestResidencyCache_GetReactivatesPendingEntry(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := 50 * time.Millisecond

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
		0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			go func() {
				time.Sleep(40 * time.Millisecond)
				if value, ok := cache.SnapshotForPersist(key, generation); ok {
					database.Set(key, value)
				}
				cache.FinishPersist(key, generation, true)
			}()
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(1, "hot")
	time.Sleep(ttl + 10*time.Millisecond)

	value, ok := cache.Get(1)
	require.True(t, ok)
	require.Equal(t, "hot", value)

	time.Sleep(60 * time.Millisecond)
	_, ok = cache.GetResident(1)
	require.True(t, ok, "reactivated entry should stay resident after old save ack")
}

func TestResidencyCache_DeleteWithoutSave(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := 50 * time.Millisecond

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
		0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			go func() {
				time.Sleep(40 * time.Millisecond)
				if value, ok := cache.SnapshotForPersist(key, generation); ok {
					database.Set(key, value)
				}
				cache.FinishPersist(key, generation, true)
			}()
			return true
		},
		func(key int) (string, error) {
			if value, ok := database.Get(key); ok {
				return value, nil
			}
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(1, "drop-me")
	time.Sleep(ttl + 10*time.Millisecond)
	cache.DeleteWithoutSave(1)
	time.Sleep(60 * time.Millisecond)

	_, ok := database.Get(1)
	require.False(t, ok)
	_, ok = cache.GetResident(1)
	require.False(t, ok)
}

func TestResidencyCache_WithCapacity(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0,
		1,
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

	cache.Set(1, "1")
	cache.Set(2, "2")

	require.Eventually(t, func() bool {
		_, ok := database.Get(1)
		return ok
	}, time.Second, 20*time.Millisecond)
}

func TestResidencyCache_DisableEnableSave(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := 50 * time.Millisecond

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
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

	cache.DisableSave()
	cache.Set(1, "keep")
	time.Sleep(2 * ttl)

	_, ok := database.Get(1)
	require.False(t, ok)
	_, ok = cache.GetResident(1)
	require.True(t, ok)

	cache.EnableSave()
	require.Eventually(t, func() bool {
		value, exists := database.Get(1)
		return exists && value == "keep"
	}, time.Second, 20*time.Millisecond)
}

func TestResidencyCache_ZeroTTLZeroMaxKeepsItemsUntilClose(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0,
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

	cache.Set(1, "1")
	time.Sleep(100 * time.Millisecond)

	_, ok := database.Get(1)
	require.False(t, ok)
	_, ok = cache.GetResident(1)
	require.True(t, ok)

	cache.Close()
	value, ok := database.Get(1)
	require.True(t, ok)
	require.Equal(t, "1", value)
}

func TestResidencyCache_QueueKeysPersistsOnlySelectedItems(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		0,
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

	cache.Set(1, "keep")
	cache.Set(2, "evict")
	cache.QueueKeys([]int{2}, utils.EvictionReasonDeleted)

	require.Eventually(t, func() bool {
		value, ok := database.Get(2)
		return ok && value == "evict"
	}, time.Second, 20*time.Millisecond)

	_, ok := cache.GetResident(1)
	require.True(t, ok)
	_, ok = cache.GetResident(2)
	require.False(t, ok)
}

func TestResidencyCache_CoolDownKeysExpiresSooner(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		10*time.Second,
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

	cache.Set(1, "hot")
	cache.Set(2, "cold")
	cache.CoolDownKeys([]int{2}, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		value, ok := database.Get(2)
		return ok && value == "cold"
	}, time.Second, 20*time.Millisecond)

	_, ok := cache.GetResident(1)
	require.True(t, ok)
}

func TestResidencyCache_SkipEvictionKeepsHotItemsResidentUntilClose(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := 40 * time.Millisecond

	var cache *dbcache.ResidencyCacheWithKey[int, string]
	cache = dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
		1,
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
		func(value string) bool {
			return value == "hot"
		},
	)

	cache.Set(1, "hot")
	cache.Set(2, "cold")
	time.Sleep(3 * ttl)

	_, ok := cache.GetResident(1)
	require.True(t, ok, "hot items should stay resident across TTL expiration")
	_, ok = cache.GetResident(2)
	require.False(t, ok, "non-hot items should still be evicted")

	_, ok = database.Get(1)
	require.False(t, ok, "hot items should not be persisted during runtime eviction")
	value, ok := database.Get(2)
	require.True(t, ok)
	require.Equal(t, "cold", value)

	cache.Close()
	value, ok = database.Get(1)
	require.True(t, ok, "hot items must still flush on close")
	require.Equal(t, "hot", value)
}

func TestCacheCloseFlushesWithoutTimeout(t *testing.T) {
	saved := 0
	cache := dbcache.NewCache[*closeFlushItem, int](
		10*time.Second,
		0,
		func(item *closeFlushItem, _ utils.EvictionReason) (int, error) {
			return int(item.id), nil
		},
		func(items []int) error {
			saved += len(items)
			return nil
		},
		nil,
		dbcache.WithSaveTimeout(5*time.Second),
	)

	cache.Set(&closeFlushItem{id: 1})
	cache.Set(&closeFlushItem{id: 2})

	start := time.Now()
	cache.Close()
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, 2, saved)
}

func TestCacheCloseFlushesRejectingLateSet(t *testing.T) {
	saved := 0
	var cache *dbcache.Cache[*closeFlushItem, int]

	cache = dbcache.NewCache[*closeFlushItem, int](
		10*time.Second,
		0,
		func(item *closeFlushItem, _ utils.EvictionReason) (int, error) {
			if item.id == 1 {
				cache.Set(&closeFlushItem{id: 2})
			}
			return int(item.id), nil
		},
		func(items []int) error {
			saved += len(items)
			return nil
		},
		nil,
		dbcache.WithSaveTimeout(5*time.Second),
	)

	cache.Set(&closeFlushItem{id: 1})
	cache.Set(&closeFlushItem{id: 2})

	done := make(chan struct{})
	go func() {
		defer close(done)
		cache.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cache.Close hung on stale pending request")
	}

	require.Equal(t, 2, saved)
}

func TestCacheCloseFlushesAllItemsWithPersistLimit(t *testing.T) {
	const total = 12
	saved := make(map[int]struct{}, total)

	cache := dbcache.NewCache[*closeFlushItem, int](
		10*time.Second,
		0,
		func(item *closeFlushItem, _ utils.EvictionReason) (int, error) {
			return int(item.id), nil
		},
		func(items []int) error {
			for _, item := range items {
				saved[item] = struct{}{}
			}
			return nil
		},
		nil,
		dbcache.WithSaveTimeout(5*time.Second),
		dbcache.WithSaveSize(2),
		dbcache.WithPersistLimit(3),
	)

	for i := 1; i <= total; i++ {
		cache.Set(&closeFlushItem{id: int64(i)})
	}

	cache.Close()

	require.Len(t, saved, total, "close should flush every resident item even when persistLimit forces batching")
}

func TestResidencyCache_RejectingPersistLeavesItemResident(t *testing.T) {
	ttl := 30 * time.Millisecond
	var rejected atomic.Int32

	cache := dbcache.NewResidencyCacheWithKey[int, string](
		ttl,
		0,
		func(key int, generation uint64, reason utils.EvictionReason) bool {
			rejected.Add(1)
			return false
		},
		func(key int) (string, error) {
			return "", utils.Errorf("missing key")
		},
	)

	cache.Set(1, "retry")

	require.Eventually(t, func() bool {
		return rejected.Load() == 1
	}, time.Second, 10*time.Millisecond)

	time.Sleep(4 * ttl)
	require.Equal(t, int32(1), rejected.Load(), "rejected persist should leave the item resident instead of immediately rearming eviction")

	value, ok := cache.GetResident(1)
	require.True(t, ok)
	require.Equal(t, "retry", value)
}
