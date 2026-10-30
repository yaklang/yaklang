package aispec

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

// Engine-wide FIFO admission by attempt START time. Completion never refunds
// RPM capacity. Healthy requests may overlap; recovery admits one probe at a time.
type modelRequestQueue struct {
	mu            sync.Mutex
	queue         []*modelRequestTicket
	starts        []time.Time
	window        time.Duration // zero means one minute; short windows are used only in tests
	cooldownUntil time.Time
	backoff       time.Duration
	generation    uint64
	probeActive   bool
	changed       chan struct{}
}
type modelRequestTicket struct{ identity byte }

var globalModelRequests = &modelRequestQueue{changed: make(chan struct{})}

// ModelAttemptDone reports the result once. It does not release a rate permit.
type ModelAttemptDone func(success, transient bool, retryAfter time.Duration) time.Duration

func AcquireModelRequest(ctx context.Context, notify func(string)) (ModelAttemptDone, error) {
	return globalModelRequests.acquire(ctx, notify)
}
func (g *modelRequestQueue) signal() { close(g.changed); g.changed = make(chan struct{}) }

// Sliding window is (now-window, now]. A request is charged on admission,
// including errors/timeouts/retries, and expires only with time.
func (g *modelRequestQueue) windowReady(now time.Time, rpm int64) time.Time {
	window := g.window
	if window == 0 {
		window = time.Minute
	}
	expired := 0
	for expired < len(g.starts) && !g.starts[expired].After(now.Add(-window)) {
		expired++
	}
	g.starts = g.starts[expired:]
	if rpm > 0 && int64(len(g.starts)) >= rpm {
		return g.starts[len(g.starts)-int(rpm)].Add(window)
	}
	return now
}

func (g *modelRequestQueue) acquire(ctx context.Context, notify func(string)) (ModelAttemptDone, error) {
	ticket := new(modelRequestTicket)
	g.mu.Lock()
	g.queue = append(g.queue, ticket)
	notified := false
	remove := func() {
		for i, queued := range g.queue {
			if queued == ticket {
				g.queue = append(g.queue[:i], g.queue[i+1:]...)
				break
			}
		}
		g.signal()
	}
	for {
		if err := ctx.Err(); err != nil {
			remove()
			g.mu.Unlock()
			return nil, err
		}
		rpm, updated := consts.ModelRequestLimit()
		now := time.Now()
		next := g.windowReady(now, rpm)
		// Disabling proactive limiting does not erase already observed cooldown.
		if rpm == 0 && g.backoff == 0 {
			remove()
			g.mu.Unlock()
			return nil, nil
		}
		var timer *time.Timer
		var ready <-chan time.Time
		if g.queue[0] == ticket && !g.probeActive {
			if g.cooldownUntil.After(next) {
				next = g.cooldownUntil
			}
			if delay := next.Sub(now); delay > 0 {
				timer = time.NewTimer(delay)
				ready = timer.C
			} else {
				remove()
				if rpm > 0 {
					g.starts = append(g.starts, now)
				}
				probe, generation := g.backoff > 0, g.generation
				if probe {
					g.probeActive = true
				}
				g.mu.Unlock()
				var once sync.Once
				var delay time.Duration
				return func(success, transient bool, retryAfter time.Duration) time.Duration {
					once.Do(func() { delay = g.complete(probe, generation, success, transient, retryAfter) })
					return delay
				}, nil
			}
		}
		changed := g.changed
		g.mu.Unlock()
		if !notified && notify != nil {
			notified = true
			notify(fmt.Sprintf("模型请求等待全局滑动窗口（连续 60 秒最多 %d 次；0 表示不限频）或故障冷却，可随时停止任务", rpm))
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-updated:
		case <-ready:
		}
		if timer != nil {
			timer.Stop()
		}
		g.mu.Lock()
	}
}

func (g *modelRequestQueue) complete(probe bool, generation uint64, success, transient bool, retryAfter time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if probe {
		g.probeActive = false
	}
	if transient {
		g.generation++
		if g.backoff == 0 {
			g.backoff = 2 * time.Second
		} else {
			g.backoff = min(2*g.backoff, 5*time.Minute)
		}
		delay := max(g.backoff, retryAfter) + time.Duration(rand.Int63n(int64(g.backoff/10)+1))
		if until := time.Now().Add(delay); until.After(g.cooldownUntil) {
			g.cooldownUntil = until
		}
	} else if success && probe && generation == g.generation {
		// An older in-flight success must not cancel a newer request's Retry-After.
		g.backoff = 0
		g.cooldownUntil = time.Time{}
	}
	g.signal()
	return max(time.Duration(0), time.Until(g.cooldownUntil))
}
