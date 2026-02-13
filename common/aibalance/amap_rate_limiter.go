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
type AmapRateLimiter struct {
	states sync.Map // map[string]*amapTraceIDState
	stopCh chan struct{}
}

// NewAmapRateLimiter creates a new rate limiter and starts background cleanup
func NewAmapRateLimiter() *AmapRateLimiter {
	rl := &AmapRateLimiter{
		stopCh: make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
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
	close(rl.stopCh)
}
