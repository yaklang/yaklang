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

func TestCacheConcurrentSettlementCallbackRegistration(t *testing.T) {
	var saved atomic.Int64
	var callbacks atomic.Int64

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		func(items []*flushItem) error {
			saved.Add(int64(len(items)))
			return nil
		},
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 32; j++ {
				cache.SetSettlementCallback(func(int64, uint64, dbcache.PersistOutcome) {
					callbacks.Add(1)
				})
			}
		}()
	}
	for i := int64(1); i <= 256; i++ {
		cache.Set(&flushItem{id: i})
		cache.MarkDirty([]int64{i}, utils.EvictionReasonCapacityReached)
	}
	wg.Wait()

	require.NoError(t, cache.Barrier())
	require.NoError(t, cache.Close())
	require.Equal(t, int64(256), saved.Load())
	_ = callbacks.Load()
}

func TestCacheConcurrentCloseIsIdempotent(t *testing.T) {
	var saved atomic.Int64
	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		func(items []*flushItem) error {
			saved.Add(int64(len(items)))
			return nil
		},
		nil,
		dbcache.WithSaveSize(8),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)
	for i := int64(1); i <= 128; i++ {
		cache.Set(&flushItem{id: i})
	}

	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- cache.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(128), saved.Load())
}

func TestCacheConcurrentSetMarkDirtyAndUpdate(t *testing.T) {
	var saved atomic.Int64
	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			return item, nil
		},
		func(items []*flushItem) error {
			saved.Add(int64(len(items)))
			return nil
		},
		nil,
		dbcache.WithSaveSize(4),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)

	for i := int64(1); i <= 32; i++ {
		cache.Set(&flushItem{id: i})
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for round := 0; round < 32; round++ {
				id := int64((worker+round)%32 + 1)
				cache.Set(&flushItem{id: id})
				cache.MarkDirty([]int64{id}, utils.EvictionReasonCapacityReached)
				cache.UpdateWhilePending(id, &flushItem{id: id})
			}
		}(worker)
	}
	wg.Wait()
	require.NoError(t, cache.Barrier())
	require.NoError(t, cache.Close())
	require.Greater(t, saved.Load(), int64(0))
}

func TestGetWhilePersistingDoesNotRequeue(t *testing.T) {
	marshalStarted := make(chan struct{})
	releaseMarshal := make(chan struct{})
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var marshalOnce sync.Once
	var saveOnce sync.Once
	var saved atomic.Int64

	cache := dbcache.NewCache[*flushItem, *flushItem](
		0,
		0,
		func(item *flushItem, _ utils.EvictionReason) (*flushItem, error) {
			marshalOnce.Do(func() { close(marshalStarted) })
			<-releaseMarshal
			return item, nil
		},
		func(items []*flushItem) error {
			saved.Add(int64(len(items)))
			saveOnce.Do(func() { close(saveStarted) })
			<-releaseSave
			return nil
		},
		nil,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(10*time.Millisecond),
	)

	cache.Set(&flushItem{id: 1})
	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	select {
	case <-marshalStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("marshal never started")
	}

	_, ok := cache.Get(1)
	require.True(t, ok)
	close(releaseMarshal)
	select {
	case <-saveStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("save never started")
	}

	cache.MarkDirty([]int64{1}, utils.EvictionReasonCapacityReached)
	close(releaseSave)
	require.NoError(t, cache.Barrier())
	require.NoError(t, cache.Close())
	require.Equal(t, int64(1), saved.Load())
}
