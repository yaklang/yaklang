package aibalance

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// traceIDState tracks the rate limiting state for a single Trace-ID.
// This struct is treated as immutable once stored in the sync.Map;
// updates are performed by storing a new copy (load/compute/store pattern)
// to avoid data races under concurrent access.
type traceIDState struct {
	lastRequestTime time.Time
	lastSuccessTime time.Time
	mu              sync.Mutex // per-entry mutex to serialize read-modify-write on the same Trace-ID
}

// WebSearchRateLimiter implements per-Trace-ID rate limiting for free web-search users
// Rules:
//   - Minimum 1 second between any two requests from the same Trace-ID
//   - After a successful request, 3 second cooldown before next request is allowed
//
// 注意：cleanupLoop 是 lazy 启动 (在 ensureCleanupStarted 内 startOnce 触发)。
// 没有实际流量的 WebSearchRateLimiter 不会创建后台 goroutine，避免在测试中
// 大量 NewServerConfig() 但从不发 web-search 请求的场景下污染 goroutine
// baseline，进而触发 TestGoroutineTracing 的 leak 误报。
// 关键词: WebSearchRateLimiter lazy cleanup, goroutine baseline 净化, TestGoroutineTracing 误报修复
type WebSearchRateLimiter struct {
	states    sync.Map // map[string]*traceIDState
	stopCh    chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once
}

// NewWebSearchRateLimiter creates a new rate limiter. The cleanup goroutine is
// NOT started until the first call to CheckRateLimit / RecordSuccess,
// keeping cost-of-creation at zero for unused limiters.
func NewWebSearchRateLimiter() *WebSearchRateLimiter {
	return &WebSearchRateLimiter{
		stopCh: make(chan struct{}),
	}
}

// ensureCleanupStarted starts the background cleanup goroutine on first use.
// Subsequent calls are no-ops (sync.Once). If Stop() was already called before
// the first use (stopCh already closed), the goroutine starts and exits
// immediately on the first ticker select, which is harmless.
// 关键词: WebSearchRateLimiter lazy 启动 cleanupLoop, startOnce 幂等
func (rl *WebSearchRateLimiter) ensureCleanupStarted() {
	rl.startOnce.Do(func() {
		go rl.cleanupLoop()
	})
}

// CheckRateLimit checks if a request from the given Trace-ID is allowed
// Returns (allowed, retryAfterSeconds)
func (rl *WebSearchRateLimiter) CheckRateLimit(traceID string) (bool, int) {
	rl.ensureCleanupStarted()
	now := time.Now()

	// LoadOrStore ensures we get-or-create atomically
	newState := &traceIDState{
		lastRequestTime: now,
	}
	val, loaded := rl.states.LoadOrStore(traceID, newState)
	if !loaded {
		// First request from this Trace-ID, allow it
		return true, 0
	}

	state := val.(*traceIDState)
	state.mu.Lock()
	defer state.mu.Unlock()

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
	rl.ensureCleanupStarted()
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
	state.mu.Lock()
	state.lastSuccessTime = now
	state.mu.Unlock()
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
				state.mu.Lock()
				latest := state.lastRequestTime
				if state.lastSuccessTime.After(latest) {
					latest = state.lastSuccessTime
				}
				state.mu.Unlock()
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
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}
