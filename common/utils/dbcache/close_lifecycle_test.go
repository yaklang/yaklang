package dbcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type closeLifecycleItem struct{ id int64 }

func (item *closeLifecycleItem) GetId() int64   { return item.id }
func (item *closeLifecycleItem) SetId(id int64) { item.id = id }

func newCloseLifecycleCache(save SaveFunc[int64]) (*Cache[*closeLifecycleItem, int64], *atomic.Int64) {
	cache := NewCache[*closeLifecycleItem, int64](
		// Keep a real TTL worker to cover its non-idempotent Stop, without
		// waiting for expiration or triggering incidental background saves.
		time.Hour, 0,
		func(item *closeLifecycleItem, _ utils.EvictionReason) (int64, error) { return item.id, nil },
		save, nil,
		WithSaveTimeout(time.Hour),
	)
	canceled := new(atomic.Int64)
	cancel := cache.cancel
	cache.cancel = func() {
		canceled.Add(1)
		cancel()
	}
	return cache, canceled
}

func waitCloseLifecycle[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("cache close did not reach its completion state")
		var zero T
		return zero
	}
}

func TestCacheCloseModesShareLifecycle(t *testing.T) {
	for _, testcase := range []struct {
		name    string
		discard []bool
		want    []int64
	}{
		{name: "discard twice", discard: []bool{true, true}},
		{name: "save then discard", discard: []bool{false, true}, want: []int64{1, 2}},
		{name: "discard then save", discard: []bool{true, false}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			var saved []int64
			cache, canceled := newCloseLifecycleCache(func(items []int64) error {
				saved = append(saved, items...)
				return nil
			})
			cache.Set(&closeLifecycleItem{id: 1})
			cache.Set(&closeLifecycleItem{id: 2})
			done := make(chan []error, 1)
			go func() {
				var errs []error
				for _, discard := range testcase.discard {
					if discard {
						cache.CloseWithoutSave()
					} else {
						errs = append(errs, cache.Close())
					}
				}
				done <- errs
			}()
			for _, err := range waitCloseLifecycle(t, done) {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, testcase.want, saved, "the first close mode decides whether residents are saved")
			require.True(t, cache.IsClosed())
			require.Zero(t, cache.Count())
			require.Equal(t, int64(1), canceled.Load(), "the cache pipeline must be shut down only once")
		})
	}
}

type closeLifecycleResult struct {
	err            error
	beforeRelease  bool
	residentClosed bool
	canceled       int64
}

func TestCacheConcurrentCloseModesWaitForFirst(t *testing.T) {
	for _, discardFirst := range []bool{false, true} {
		name := "save first"
		if discardFirst {
			name = "discard first"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			var released atomic.Bool
			var enterOnce, releaseOnce sync.Once
			unblock := func() {
				released.Store(true)
				releaseOnce.Do(func() { close(release) })
			}
			t.Cleanup(unblock)
			var saved atomic.Int64
			cache, canceled := newCloseLifecycleCache(func(items []int64) error {
				if !discardFirst {
					enterOnce.Do(func() { close(entered) })
					<-release
				}
				saved.Add(int64(len(items)))
				return nil
			})
			cache.Set(&closeLifecycleItem{id: 1})
			cache.Set(&closeLifecycleItem{id: 2})
			if discardFirst {
				cache.AsyncDrainAndShrink(func() {
					close(entered)
					<-release
				})
				waitCloseLifecycle(t, entered)
			}
			first := make(chan error, 1)
			go func() {
				if discardFirst {
					cache.CloseWithoutSave()
					first <- nil
				} else {
					first <- cache.Close()
				}
			}()
			if discardFirst {
				// This closes only after the discard path owns closeOnce and is
				// waiting for the already-entered observation callback.
				waitCloseLifecycle(t, cache.asyncDrainCancel)
			} else {
				waitCloseLifecycle(t, entered)
			}
			require.True(t, cache.IsClosed())
			require.False(t, cache.resident.IsClosed(), "the first close is still blocked at its state barrier")
			require.Zero(t, canceled.Load())

			const waiters = 4
			started := make(chan struct{})
			results := make(chan closeLifecycleResult, waiters)
			for index := 0; index < waiters; index++ {
				go func(discard bool) {
					started <- struct{}{}
					var err error
					if discard {
						cache.CloseWithoutSave()
					} else {
						err = cache.Close()
					}
					results <- closeLifecycleResult{
						err: err, beforeRelease: !released.Load(),
						residentClosed: cache.resident.IsClosed(), canceled: canceled.Load(),
					}
				}(index%2 == 0)
			}
			for index := 0; index < waiters; index++ {
				waitCloseLifecycle(t, started)
			}
			unblock()
			require.NoError(t, waitCloseLifecycle(t, first))
			for index := 0; index < waiters; index++ {
				result := waitCloseLifecycle(t, results)
				require.NoError(t, result.err)
				require.False(t, result.beforeRelease, "a later close returned while the first close was still blocked")
				require.True(t, result.residentClosed, "every close waits until resident shutdown is complete")
				require.Equal(t, int64(1), result.canceled)
			}
			wantSaved := int64(2)
			if discardFirst {
				wantSaved = 0
			}
			require.Equal(t, wantSaved, saved.Load())
			require.Zero(t, cache.Count())
			require.Equal(t, int64(1), canceled.Load())
			require.True(t, cache.marshalPipe.IsContextCancel())
			require.ErrorIs(t, cache.saver.ctx.Err(), context.Canceled)
		})
	}
}

func TestCacheCloseModesKeepPersistError(t *testing.T) {
	for _, discardFirst := range []bool{false, true} {
		name := "save first"
		if discardFirst {
			name = "discard first after queued failure"
		}
		t.Run(name, func(t *testing.T) {
			failure := errors.New("close lifecycle save failure")
			cache, _ := newCloseLifecycleCache(func([]int64) error { return failure })
			cache.Set(&closeLifecycleItem{id: 1})
			if discardFirst {
				cache.Evict([]int64{1}, utils.EvictionReasonDeleted)
				require.ErrorIs(t, cache.Barrier(), failure)
			}
			done := make(chan []error, 1)
			go func() {
				var first error
				if !discardFirst {
					first = cache.Close()
				}
				cache.CloseWithoutSave()
				done <- []error{first, cache.Close(), cache.Close()}
			}()
			errs := waitCloseLifecycle(t, done)
			if !discardFirst {
				require.ErrorIs(t, errs[0], failure)
				require.Equal(t, errs[0], errs[1], "discard after save must keep the original close error")
			}
			require.ErrorIs(t, errs[1], failure)
			require.Equal(t, errs[1], errs[2], "all save callers must receive the completed close error")
			if discardFirst {
				require.NotContains(t, errs[1].Error(), "were not persisted", "discard must not report intentional drops as persistence failures")
			}
		})
	}
}

func TestCacheDiscardKeepsMarshalError(t *testing.T) {
	failure := errors.New("close lifecycle marshal failure")
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var saved atomic.Int64
	cache := NewCache[*closeLifecycleItem, int64](
		time.Hour, 0,
		func(*closeLifecycleItem, utils.EvictionReason) (int64, error) {
			close(entered)
			<-release
			return 0, failure
		},
		func(items []int64) error { saved.Add(int64(len(items))); return nil },
		nil, WithSaveTimeout(time.Hour),
	)
	cache.Set(&closeLifecycleItem{id: 1})
	cache.Set(&closeLifecycleItem{id: 2})
	cache.Evict([]int64{1}, utils.EvictionReasonDeleted)
	waitCloseLifecycle(t, entered)
	done := make(chan error, 1)
	go func() {
		cache.CloseWithoutSave()
		done <- cache.Close()
	}()
	waitCloseLifecycle(t, cache.asyncDrainCancel)
	unblock()
	err := waitCloseLifecycle(t, done)
	require.ErrorIs(t, err, failure)
	require.NotContains(t, err.Error(), "were not persisted", "discarding the second resident must not add a save failure")
	require.Zero(t, saved.Load())
	require.Zero(t, cache.Count())
}
