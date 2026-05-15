package aibalance

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// amapTraceIDState tracks the rate limiting state for a single Trace-ID in amap proxy.
type amapTraceIDState struct {
	lastRequestTime time.Time
	mu              sync.Mutex
}

// AmapRateLimiter implements per-Trace-ID rate limiting for free amap proxy users.
// Unlike WebSearchRateLimiter, this limiter blocks (sleeps) instead of returning 429,
// making rate limiting transparent to the client (they only see increased latency).
//
// Rules:
//   - Minimum 1 second between any two requests from the same Trace-ID
//
// 注意：cleanupLoop 是 lazy 启动 (在 ensureCleanupStarted 内 startOnce 触发)。
// 没有实际请求经过的 AmapRateLimiter 不会创建后台 goroutine，避免在测试中
// 大量 NewServerConfig() 但从不发请求的场景下污染 goroutine baseline，
// 进而触发 TestGoroutineTracing 的 leak 误报。
// 关键词: AmapRateLimiter lazy cleanup, goroutine baseline 净化, TestGoroutineTracing 误报修复
type AmapRateLimiter struct {
	states     sync.Map // map[string]*amapTraceIDState
	stopCh     chan struct{}
	stopOnce   sync.Once
	startOnce  sync.Once
}

// NewAmapRateLimiter creates a new rate limiter. The cleanup goroutine is NOT
// started until the first call to WaitForRateLimit / checkAndGetWait,
// keeping cost-of-creation at zero for unused limiters.
func NewAmapRateLimiter() *AmapRateLimiter {
	return &AmapRateLimiter{
		stopCh: make(chan struct{}),
	}
}

// ensureCleanupStarted starts the background cleanup goroutine on first use.
// Subsequent calls are no-ops (sync.Once). If Stop() was already called before
// the first use (stopCh already closed), the goroutine starts and exits
// immediately on the first ticker select, which is harmless.
// 关键词: AmapRateLimiter lazy 启动 cleanupLoop, startOnce 幂等
func (rl *AmapRateLimiter) ensureCleanupStarted() {
	rl.startOnce.Do(func() {
		go rl.cleanupLoop()
	})
}

// WaitForRateLimit blocks until the rate limit allows the request through,
// or the context is cancelled/timed out.
// Returns nil if the request is allowed, or context error if timed out.
func (rl *AmapRateLimiter) WaitForRateLimit(traceID string, ctx context.Context) error {
	for {
		allowed, waitDuration := rl.checkAndGetWait(traceID)
		if allowed {
			return nil
		}

		// Sleep for the required wait duration or until context cancels
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Retry the check after waiting
			continue
		}
	}
}

// checkAndGetWait checks if a request is allowed and returns the wait duration if not.
func (rl *AmapRateLimiter) checkAndGetWait(traceID string) (bool, time.Duration) {
	rl.ensureCleanupStarted()
	now := time.Now()

	newState := &amapTraceIDState{
		lastRequestTime: now,
	}
	val, loaded := rl.states.LoadOrStore(traceID, newState)
	if !loaded {
		// First request from this Trace-ID, allow it
		return true, 0
	}

	state := val.(*amapTraceIDState)
	state.mu.Lock()
	defer state.mu.Unlock()

	// Check: minimum 1 second between any two requests
	sinceLastRequest := now.Sub(state.lastRequestTime)
	if sinceLastRequest < 1*time.Second {
		waitTime := 1*time.Second - sinceLastRequest
		return false, waitTime
	}

	// Allowed: update last request time
	state.lastRequestTime = now
	return true, 0
}

// RecordSuccess is a no-op kept for interface compatibility.
// With the simplified 1s rate limit, no special success handling is needed.
func (rl *AmapRateLimiter) RecordSuccess(traceID string) {
	// no-op: the 1-second interval between requests is enforced in checkAndGetWait
}

// cleanupLoop periodically removes stale Trace-ID entries (inactive for > 5 minutes)
func (rl *AmapRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-5 * time.Minute)
			count := 0
			rl.states.Range(func(key, value interface{}) bool {
				state := value.(*amapTraceIDState)
				state.mu.Lock()
				latest := state.lastRequestTime
				state.mu.Unlock()
				if latest.Before(cutoff) {
					rl.states.Delete(key)
					count++
				}
				return true
			})
			if count > 0 {
				log.Infof("amap rate limiter: cleaned up %d stale trace-id entries", count)
			}
		case <-rl.stopCh:
			return
		}
	}
}

// Stop stops the background cleanup goroutine
func (rl *AmapRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}
