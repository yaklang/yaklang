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

// ==================== ChatRateLimiter Model Delay Override ====================

// TestChatRateLimiter_ModelDelayOverride verifies that per-model delay
// overrides take precedence over the global free-user delay fallback,
// while models without an override still see the fallback.
func TestChatRateLimiter_ModelDelayOverride(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	const fallback int64 = 5

	assert.Equal(t, fallback, rl.GetEffectiveDelay("any-model", fallback),
		"without override the fallback should be returned")

	rl.SetModelDelay("slow-free", 30)
	assert.Equal(t, int64(30), rl.GetEffectiveDelay("slow-free", fallback),
		"override should win over fallback")

	rl.SetModelDelay("fast-free", 0)
	assert.Equal(t, int64(0), rl.GetEffectiveDelay("fast-free", fallback),
		"explicit 0 override means no delay (overrides fallback)")

	rl.SetModelDelay("slow-free", -1)
	assert.Equal(t, fallback, rl.GetEffectiveDelay("slow-free", fallback),
		"negative value should remove override")
}

// TestChatRateLimiter_ClearModelDelay verifies that ClearModelDelay wipes
// every per-model delay override.
func TestChatRateLimiter_ClearModelDelay(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	rl.SetModelDelay("a-free", 10)
	rl.SetModelDelay("b-free", 20)
	assert.Equal(t, int64(10), rl.GetEffectiveDelay("a-free", 1))
	assert.Equal(t, int64(20), rl.GetEffectiveDelay("b-free", 1))

	rl.ClearModelDelay()
	assert.Equal(t, int64(1), rl.GetEffectiveDelay("a-free", 1),
		"after clear, fallback should be returned")
	assert.Equal(t, int64(1), rl.GetEffectiveDelay("b-free", 1),
		"after clear, fallback should be returned")
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

// ==================== Model RPM Stats Aggregation ====================

// TestGetModelRPMStats verifies cross-apiKey aggregation, threshold
// filtering and descending sort order for the hot-model RPM stats used
// by the "限流配置" page.
func TestGetModelRPMStats(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	// Lift the RPM ceiling so the fake traffic below is not rate-limited,
	// which would cause trimExpired to run on denied requests and skew
	// counters.
	rl.SetDefaultRPM(10000)

	// Drive traffic:
	// model-hot:  25 reqs from key-1 + 10 reqs from key-2 = 35  -> kept
	// model-mid:  15 reqs from key-1 +  5 reqs from key-3 = 20  -> kept (==threshold)
	// model-cold: 5  reqs from key-1                      = 5   -> filtered out
	fire := func(key, model string, n int) {
		for i := 0; i < n; i++ {
			allowed, _ := rl.CheckRateLimit(key, model)
			require.True(t, allowed, "setup request should be allowed, key=%s model=%s i=%d", key, model, i)
		}
	}
	fire("key-1", "model-hot", 25)
	fire("key-2", "model-hot", 10)
	fire("key-1", "model-mid", 15)
	fire("key-3", "model-mid", 5)
	fire("key-1", "model-cold", 5)

	// Also set a model-level override so we can assert EffectiveRPM
	// reflects per-model overrides instead of the global default.
	rl.SetModelRPM("model-hot", 500)

	stats := rl.GetModelRPMStats(20)
	require.Len(t, stats, 2, "only model-hot and model-mid should pass the >=20 threshold, got %+v", stats)

	assert.Equal(t, "model-hot", stats[0].Model, "first entry should be the model with highest RPM")
	assert.Equal(t, int64(35), stats[0].RPM)
	assert.Equal(t, int64(500), stats[0].EffectiveRPM,
		"EffectiveRPM should follow the per-model override")

	assert.Equal(t, "model-mid", stats[1].Model)
	assert.Equal(t, int64(20), stats[1].RPM)
	assert.Equal(t, int64(10000), stats[1].EffectiveRPM,
		"EffectiveRPM without override should fall back to the global default")

	// A lower threshold must include every model we drove traffic to.
	all := rl.GetModelRPMStats(1)
	gotModels := map[string]int64{}
	for _, s := range all {
		gotModels[s.Model] = s.RPM
	}
	assert.Equal(t, int64(35), gotModels["model-hot"])
	assert.Equal(t, int64(20), gotModels["model-mid"])
	assert.Equal(t, int64(5), gotModels["model-cold"])

	// Zero threshold behaves like "return everything".
	zeroAll := rl.GetModelRPMStats(0)
	assert.Equal(t, len(all), len(zeroAll), "minRPM=0 should match minRPM=1 in this scenario")

	// A very high threshold should return an empty slice (non-nil).
	none := rl.GetModelRPMStats(1000)
	require.NotNil(t, none)
	assert.Len(t, none, 0)
}

// TestGetModelRPMStats_EmptyWhenNoTraffic ensures a freshly constructed
// rate limiter returns an empty (non-nil) slice and does not panic.
func TestGetModelRPMStats_EmptyWhenNoTraffic(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	stats := rl.GetModelRPMStats(20)
	require.NotNil(t, stats)
	assert.Len(t, stats, 0)
}

// TestGetModelRPMStats_SlidingWindowDrops verifies that stale requests
// outside the 60s sliding window are excluded from the aggregation.
func TestGetModelRPMStats_SlidingWindowDrops(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(10000)

	// Seed some very old timestamps directly into the internal state and
	// mix in one fresh request to prove only the fresh one survives
	// trimExpired. This avoids waiting 60s in tests.
	bucketKey := "keyZ" + "|" + "model-slide"
	old := time.Now().Add(-2 * rpmWindowDuration)
	rl.states.Store(bucketKey, &keyRPMState{
		requests: []time.Time{old, old, old},
	})
	allowed, _ := rl.CheckRateLimit("keyZ", "model-slide")
	require.True(t, allowed)

	stats := rl.GetModelRPMStats(0)
	var found *ModelRPMStat
	for i := range stats {
		if stats[i].Model == "model-slide" {
			found = &stats[i]
			break
		}
	}
	require.NotNil(t, found, "model-slide should be present")
	assert.Equal(t, int64(1), found.RPM,
		"expired timestamps must be trimmed before counting")
}
