package aibalance

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

const (
	defaultRPMValue     = 600
	rpmWindowDuration   = 60 * time.Second
	cleanupInterval     = 2 * time.Minute
	staleEntryThreshold = 5 * time.Minute
)

// keyRPMState tracks per-API-key sliding window request timestamps.
type keyRPMState struct {
	mu       sync.Mutex
	requests []time.Time
}

// trimExpired removes timestamps older than the RPM window.
func (s *keyRPMState) trimExpired(now time.Time) {
	cutoff := now.Add(-rpmWindowDuration)
	i := 0
	for i < len(s.requests) && s.requests[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		s.requests = s.requests[i:]
	}
}

// ChatRateLimiter implements per-API-key RPM rate limiting for chat completions,
// with optional per-model RPM overrides and a global queue counter.
type ChatRateLimiter struct {
	states     sync.Map   // map[apiKey]*keyRPMState
	queueCount atomic.Int64
	defaultRPM atomic.Int64
	modelRPM   sync.Map // map[modelName]int64
	stopCh     chan struct{}
	stopOnce   sync.Once
}

func NewChatRateLimiter() *ChatRateLimiter {
	rl := &ChatRateLimiter{
		stopCh: make(chan struct{}),
	}
	rl.defaultRPM.Store(defaultRPMValue)
	go rl.cleanupLoop()
	return rl
}

// SetDefaultRPM updates the global default RPM limit.
func (rl *ChatRateLimiter) SetDefaultRPM(rpm int64) {
	if rpm <= 0 {
		rpm = defaultRPMValue
	}
	rl.defaultRPM.Store(rpm)
}

// SetModelRPM sets an RPM override for a specific model.
func (rl *ChatRateLimiter) SetModelRPM(model string, rpm int64) {
	if rpm <= 0 {
		rl.modelRPM.Delete(model)
		return
	}
	rl.modelRPM.Store(model, rpm)
}

// ClearModelRPM removes all per-model RPM overrides.
func (rl *ChatRateLimiter) ClearModelRPM() {
	rl.modelRPM.Range(func(key, _ any) bool {
		rl.modelRPM.Delete(key)
		return true
	})
}

// GetQueueCount returns the current number of rate-limited (queued) requests.
func (rl *ChatRateLimiter) GetQueueCount() int64 {
	return rl.queueCount.Load()
}

// getEffectiveRPM returns the RPM limit for a given model,
// falling back to the global default.
func (rl *ChatRateLimiter) getEffectiveRPM(modelName string) int64 {
	if v, ok := rl.modelRPM.Load(modelName); ok {
		return v.(int64)
	}
	return rl.defaultRPM.Load()
}

// CheckRateLimit checks whether a request from apiKey for modelName is allowed.
// Returns (allowed, currentQueueLength).
// If allowed, the request is automatically recorded in the sliding window.
func (rl *ChatRateLimiter) CheckRateLimit(apiKey string, modelName string) (bool, int64) {
	now := time.Now()
	rpm := rl.getEffectiveRPM(modelName)

	newState := &keyRPMState{
		requests: []time.Time{now},
	}
	val, loaded := rl.states.LoadOrStore(apiKey, newState)
	if !loaded {
		return true, rl.queueCount.Load()
	}

	state := val.(*keyRPMState)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.trimExpired(now)

	if int64(len(state.requests)) >= rpm {
		rl.queueCount.Add(1)
		qLen := rl.queueCount.Load()
		rl.queueCount.Add(-1)
		return false, qLen
	}

	state.requests = append(state.requests, now)
	return true, rl.queueCount.Load()
}

// cleanupLoop periodically removes stale API key entries.
func (rl *ChatRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-staleEntryThreshold)
			count := 0
			rl.states.Range(func(key, value any) bool {
				state := value.(*keyRPMState)
				state.mu.Lock()
				latest := time.Time{}
				if len(state.requests) > 0 {
					latest = state.requests[len(state.requests)-1]
				}
				state.mu.Unlock()
				if latest.Before(cutoff) {
					rl.states.Delete(key)
					count++
				}
				return true
			})
			if count > 0 {
				log.Infof("chat rate limiter: cleaned up %d stale api-key entries", count)
			}
		case <-rl.stopCh:
			return
		}
	}
}

// Stop stops the background cleanup goroutine.
func (rl *ChatRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}
