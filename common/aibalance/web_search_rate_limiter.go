package aibalance

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// traceIDState tracks the rate limiting state for a single Trace-ID
type traceIDState struct {
	lastRequestTime time.Time
	lastSuccessTime time.Time
}

// WebSearchRateLimiter implements per-Trace-ID rate limiting for free web-search users
// Rules:
//   - Minimum 1 second between any two requests from the same Trace-ID
//   - After a successful request, 3 second cooldown before next request is allowed
type WebSearchRateLimiter struct {
	states sync.Map // map[string]*traceIDState
	stopCh chan struct{}
}

// NewWebSearchRateLimiter creates a new rate limiter and starts background cleanup
func NewWebSearchRateLimiter() *WebSearchRateLimiter {
	rl := &WebSearchRateLimiter{
		stopCh: make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// CheckRateLimit checks if a request from the given Trace-ID is allowed
// Returns (allowed, retryAfterSeconds)
func (rl *WebSearchRateLimiter) CheckRateLimit(traceID string) (bool, int) {
	now := time.Now()

	val, loaded := rl.states.Load(traceID)
	if !loaded {
		// First request from this Trace-ID, allow it
		rl.states.Store(traceID, &traceIDState{
			lastRequestTime: now,
		})
		return true, 0
	}

	state := val.(*traceIDState)

	// Check: after a successful request, 3 second cooldown
	if !state.lastSuccessTime.IsZero() {
		sinceSuccess := now.Sub(state.lastSuccessTime)
		if sinceSuccess < 3*time.Second {
			retryAfter := int(3*time.Second-sinceSuccess)/int(time.Second) + 1
			return false, retryAfter
		}
	}

	// Check: minimum 1 second between any two requests
	sinceLastRequest := now.Sub(state.lastRequestTime)
	if sinceLastRequest < 1*time.Second {
		return false, 1
	}

	// Allowed: update last request time
	state.lastRequestTime = now
	return true, 0
}

// RecordSuccess records that a request from the given Trace-ID was successful
// This triggers the 3-second cooldown for subsequent requests
func (rl *WebSearchRateLimiter) RecordSuccess(traceID string) {
	now := time.Now()

	val, loaded := rl.states.Load(traceID)
	if !loaded {
		rl.states.Store(traceID, &traceIDState{
			lastRequestTime: now,
			lastSuccessTime: now,
		})
		return
	}

	state := val.(*traceIDState)
	state.lastSuccessTime = now
}

// cleanupLoop periodically removes stale Trace-ID entries (inactive for > 5 minutes)
func (rl *WebSearchRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-5 * time.Minute)
			count := 0
			rl.states.Range(func(key, value interface{}) bool {
				state := value.(*traceIDState)
				latest := state.lastRequestTime
				if state.lastSuccessTime.After(latest) {
					latest = state.lastSuccessTime
				}
				if latest.Before(cutoff) {
					rl.states.Delete(key)
					count++
				}
				return true
			})
			if count > 0 {
				log.Infof("web search rate limiter: cleaned up %d stale trace-id entries", count)
			}
		case <-rl.stopCh:
			return
		}
	}
}

// Stop stops the background cleanup goroutine
func (rl *WebSearchRateLimiter) Stop() {
	close(rl.stopCh)
}
