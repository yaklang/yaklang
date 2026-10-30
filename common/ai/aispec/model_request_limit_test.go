package aispec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func useModelLimit(t *testing.T, rpm int64) *modelRequestQueue {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{ModelRequestsPerMinute: rpm})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	return &modelRequestQueue{changed: make(chan struct{})}
}
func TestGlobalModelLimitSlidingWindowAndCompletion(t *testing.T) {
	g := useModelLimit(t, 3)
	now := time.Now()
	// Three requests in a 60s rolling window. Responses do not refund permits.
	g.starts = []time.Time{now.Add(-59 * time.Second), now.Add(-30 * time.Second), now}
	require.Equal(t, now.Add(time.Second), g.windowReady(now, 3))
	g.complete(false, 0, true, false, 0)
	require.Len(t, g.starts, 3)
	require.Equal(t, now.Add(time.Second), g.windowReady(now.Add(time.Second), 3))
	require.Len(t, g.starts, 2)
	// Lowering RPM uses the Nth newest timestamp, not the oldest.
	require.Equal(t, now.Add(time.Minute), g.windowReady(now.Add(time.Second), 1))
}
func TestGlobalModelLimitAllowsConcurrentStartsAndBlocksFourth(t *testing.T) {
	g := useModelLimit(t, 3)
	g.window = 100 * time.Millisecond
	var callbacks []ModelAttemptDone
	for i := 0; i < 3; i++ {
		done, err := g.acquire(context.Background(), nil)
		require.NoError(t, err)
		callbacks = append(callbacks, done)
	}
	// All three are still in flight; fourth waits for time, not any response.
	start := time.Now()
	done, err := g.acquire(context.Background(), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(start), 80*time.Millisecond)
	require.Len(t, callbacks, 3)
	done(true, false, 0)
	for _, cb := range callbacks {
		cb(true, false, 0)
	}
}
func TestGlobalModelLimitFIFOAndCancel(t *testing.T) {
	g := useModelLimit(t, 1)
	done, err := g.acquire(context.Background(), nil)
	require.NoError(t, err)
	done(false, false, 0)
	results := make(chan int, 3)
	cancels := make([]context.CancelFunc, 3)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		cancels[i] = cancel
		defer cancel()
		go func(i int) {
			cb, err := g.acquire(ctx, nil)
			if err == nil {
				results <- i
				if cb != nil {
					cb(true, false, 0)
				}
			}
		}(i)
		require.Eventually(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) == i+1 }, time.Second, time.Millisecond)
	}
	cancels[1]()
	require.Eventually(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) == 2 }, time.Second, time.Millisecond)
	// Release one waiting ticket at a time to observe FIFO despite goroutine scheduling.
	for _, want := range []int{0, 2} {
		g.mu.Lock()
		g.starts = nil
		g.signal()
		g.mu.Unlock()
		select {
		case got := <-results:
			require.Equal(t, want, got)
		case <-time.After(time.Second):
			t.Fatal("queue stalled")
		}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{})
	cb, err := g.acquire(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, cb)
}
func TestGlobalModelLimitPreservesCooldownAgainstLateSuccess(t *testing.T) {
	g := useModelLimit(t, 3)
	first, _ := g.acquire(context.Background(), nil)
	late, _ := g.acquire(context.Background(), nil)
	require.GreaterOrEqual(t, first(false, true, 15*time.Minute), 15*time.Minute)
	late(true, false, 0)
	require.Greater(t, time.Until(g.cooldownUntil), 14*time.Minute)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := g.acquire(ctx, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// The single recovery probe succeeds; it may clear the cooldown.
	g.mu.Lock()
	g.cooldownUntil = time.Now()
	g.mu.Unlock()
	probe, err := g.acquire(context.Background(), nil)
	require.NoError(t, err)
	blocked, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, err = g.acquire(blocked, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	probe(true, false, 0)
	require.Zero(t, g.backoff)
}

func TestGlobalModelLimitChangeWakesQueuedRequests(t *testing.T) {
	g := useModelLimit(t, 1)
	first, err := g.acquire(context.Background(), nil)
	require.NoError(t, err)
	defer first(true, false, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		report, err := g.acquire(ctx, nil)
		if report != nil {
			report(true, false, 0)
		}
		result <- err
	}()
	require.Eventually(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) == 1 }, time.Second, time.Millisecond)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{ModelRequestsPerMinute: 2})
	require.NoError(t, <-result)
	require.Len(t, g.starts, 2)
}
