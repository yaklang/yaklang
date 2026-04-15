package aibalance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== ChatRateLimiter Unit Tests ====================

func TestChatRateLimiter_FirstRequest(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	allowed, queueLen := rl.CheckRateLimit("key-001", "gpt-4")
	assert.True(t, allowed, "first request should be allowed")
	assert.Equal(t, int64(0), queueLen, "queue should be empty")
}

func TestChatRateLimiter_DefaultRPM(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	assert.Equal(t, int64(defaultRPMValue), rl.defaultRPM.Load(), "default RPM should be 600")
}

func TestChatRateLimiter_SetDefaultRPM(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(100)
	assert.Equal(t, int64(100), rl.defaultRPM.Load())

	rl.SetDefaultRPM(0)
	assert.Equal(t, int64(defaultRPMValue), rl.defaultRPM.Load(), "zero should reset to default")

	rl.SetDefaultRPM(-5)
	assert.Equal(t, int64(defaultRPMValue), rl.defaultRPM.Load(), "negative should reset to default")
}

func TestChatRateLimiter_RPMExceeded(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(5)

	key := "test-key-rpm"
	for i := 0; i < 5; i++ {
		allowed, _ := rl.CheckRateLimit(key, "model-a")
		assert.True(t, allowed, "request %d should be allowed (within RPM)", i+1)
	}

	allowed, queueLen := rl.CheckRateLimit(key, "model-a")
	assert.False(t, allowed, "6th request should be denied (RPM exceeded)")
	assert.Greater(t, queueLen, int64(0), "queue length should be positive when denied")
}

func TestChatRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(3)

	for i := 0; i < 3; i++ {
		allowed, _ := rl.CheckRateLimit("key-A", "model-x")
		assert.True(t, allowed)
	}

	allowed, _ := rl.CheckRateLimit("key-A", "model-x")
	assert.False(t, allowed, "key-A should be rate limited")

	allowed2, _ := rl.CheckRateLimit("key-B", "model-x")
	assert.True(t, allowed2, "key-B should still be allowed (independent)")
}

func TestChatRateLimiter_ModelRPMOverride(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(100)
	rl.SetModelRPM("expensive-model", 2)

	key := "test-key-model"
	for i := 0; i < 2; i++ {
		allowed, _ := rl.CheckRateLimit(key, "expensive-model")
		assert.True(t, allowed, "request %d should be allowed", i+1)
	}

	allowed, _ := rl.CheckRateLimit(key, "expensive-model")
	assert.False(t, allowed, "3rd request to expensive-model should be denied")

	allowedDefault, _ := rl.CheckRateLimit(key, "cheap-model")
	assert.True(t, allowedDefault, "cheap-model should use default RPM and still be allowed")
}

func TestChatRateLimiter_ClearModelRPM(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetModelRPM("model-a", 10)
	rl.SetModelRPM("model-b", 20)

	assert.Equal(t, int64(10), rl.getEffectiveRPM("model-a"))
	assert.Equal(t, int64(20), rl.getEffectiveRPM("model-b"))

	rl.ClearModelRPM()

	assert.Equal(t, rl.defaultRPM.Load(), rl.getEffectiveRPM("model-a"), "after clear, should use default")
	assert.Equal(t, rl.defaultRPM.Load(), rl.getEffectiveRPM("model-b"), "after clear, should use default")
}

func TestChatRateLimiter_SlidingWindowExpiry(t *testing.T) {
	rl := &ChatRateLimiter{
		stopCh: make(chan struct{}),
	}
	rl.defaultRPM.Store(3)

	key := "test-key-window"

	// Manually create a state with old timestamps
	state := &keyRPMState{
		requests: []time.Time{
			time.Now().Add(-90 * time.Second),
			time.Now().Add(-80 * time.Second),
			time.Now().Add(-70 * time.Second),
		},
	}
	rl.states.Store(key, state)

	// All old entries should be expired, so new request should be allowed
	allowed, _ := rl.CheckRateLimit(key, "any-model")
	assert.True(t, allowed, "request should be allowed after old entries expire")
}

func TestChatRateLimiter_QueueCountTransient(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	assert.Equal(t, int64(0), rl.GetQueueCount(), "initial queue should be 0")

	rl.SetDefaultRPM(1)

	allowed1, _ := rl.CheckRateLimit("key-q", "model")
	assert.True(t, allowed1)

	_, queueLen := rl.CheckRateLimit("key-q", "model")
	assert.Greater(t, queueLen, int64(0), "queue count should be positive during denial")

	// After the CheckRateLimit call returns, queue count should be back to 0
	assert.Equal(t, int64(0), rl.GetQueueCount(), "queue should reset after denial returns")
}

func TestChatRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(1000)

	var wg sync.WaitGroup
	concurrency := 100
	allowedCount := int64(0)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", idx)
			allowed, _ := rl.CheckRateLimit(key, "model")
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(concurrency), allowedCount, "all first requests from unique keys should be allowed")
}

func TestChatRateLimiter_ConcurrentSameKey(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetDefaultRPM(10)

	var wg sync.WaitGroup
	concurrency := 50
	allowedCount := int64(0)
	deniedCount := int64(0)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := rl.CheckRateLimit("same-key", "model")
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			} else {
				atomic.AddInt64(&deniedCount, 1)
			}
		}()
	}

	wg.Wait()

	assert.LessOrEqual(t, allowedCount, int64(concurrency))
	assert.GreaterOrEqual(t, allowedCount, int64(1), "at least one request should be allowed")
	total := allowedCount + deniedCount
	assert.Equal(t, int64(concurrency), total, "all requests should be accounted for")
	t.Logf("allowed=%d denied=%d (RPM=10, concurrency=%d)", allowedCount, deniedCount, concurrency)
}

func TestChatRateLimiter_StopIdempotent(t *testing.T) {
	rl := NewChatRateLimiter()
	rl.Stop()
	rl.Stop() // should not panic
}

func TestChatRateLimiter_SetModelRPM_ZeroRemoves(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetModelRPM("model-x", 50)
	assert.Equal(t, int64(50), rl.getEffectiveRPM("model-x"))

	rl.SetModelRPM("model-x", 0)
	assert.Equal(t, rl.defaultRPM.Load(), rl.getEffectiveRPM("model-x"), "zero RPM should remove override")
}

// ==================== Integration: ChatRateLimiter + ServerConfig ====================

func TestServerConfig_ChatRateLimiterInitialized(t *testing.T) {
	cfg := NewServerConfig()
	require.NotNil(t, cfg.chatRateLimiter, "chatRateLimiter should be initialized")
	assert.Equal(t, int64(defaultRPMValue), cfg.chatRateLimiter.defaultRPM.Load())
}

func TestServerConfig_FreeUserDelayDefault(t *testing.T) {
	cfg := NewServerConfig()
	assert.Equal(t, int64(3), cfg.freeUserDelaySec, "default free user delay should be 3")
}
