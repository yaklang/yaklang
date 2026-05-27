package aibalance

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentRelease_Idempotent verifies that the releaseConcurrent pattern
// used in serveChatCompletions is idempotent (only decrements once).
func TestConcurrentRelease_Idempotent(t *testing.T) {
	var counter int64
	atomic.AddInt64(&counter, 1)

	released := false
	release := func() {
		if !released {
			released = true
			atomic.AddInt64(&counter, -1)
		}
	}

	assert.Equal(t, int64(1), atomic.LoadInt64(&counter))

	release()
	assert.Equal(t, int64(0), atomic.LoadInt64(&counter))

	release()
	assert.Equal(t, int64(0), atomic.LoadInt64(&counter), "second release should be no-op")

	release()
	assert.Equal(t, int64(0), atomic.LoadInt64(&counter), "third release should be no-op")
}

// TestConcurrentRelease_DeferSafe verifies that defer + explicit call pattern
// doesn't double-decrement.
func TestConcurrentRelease_DeferSafe(t *testing.T) {
	var counter int64

	func() {
		atomic.AddInt64(&counter, 1)
		released := false
		release := func() {
			if !released {
				released = true
				atomic.AddInt64(&counter, -1)
			}
		}
		defer release()

		// Simulate explicit release before function returns (like free user sleep)
		release()
		assert.Equal(t, int64(0), atomic.LoadInt64(&counter),
			"counter should be 0 after explicit release")

		// Simulate sleep period
		time.Sleep(10 * time.Millisecond)

		// When defer runs, it should NOT decrement again
	}()

	assert.Equal(t, int64(0), atomic.LoadInt64(&counter),
		"counter should still be 0 after defer runs")
}

// TestFreeUserPreCallDelay_DelaysBeforeProvider verifies that when a free
// model has a positive effective delay, serveChatCompletions sleeps BEFORE
// dispatching to providers, so the client perceives the throttle (the
// previous post-call sleep was ineffective because the response had
// already been delivered).
//
// Implementation notes:
//   - We use a free model (-free suffix) so no API key is required.
//   - There are no providers configured, so the request will eventually
//     return a 5xx after passing through auth, RPM check, and pre-call
//     delay. The total elapsed time of the call is what we assert on.
func TestFreeUserPreCallDelay_DelaysBeforeProvider(t *testing.T) {
	persistDBRateLimitRPM(t, 100)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(100)
	cfg.freeUserDelayMinSec = 1 // global fallback: 1s base, Max=0 -> 老语义 N~2N (1~2s)
	cfg.freeUserDelayMaxSec = 0

	raw := buildFreeModelRawHTTP("delay-precall-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond,
		"pre-call delay should make the call take at least the base delay")
	assert.NotContains(t, resp, "429",
		"request must pass the RPM check before being delayed")
}

// TestFreeUserPreCallDelay_PerModelOverride verifies that a per-model delay
// override of 0 disables the pre-call delay for that model even when the
// global free-user delay is non-zero.
func TestFreeUserPreCallDelay_PerModelOverride(t *testing.T) {
	persistDBRateLimitRPM(t, 100)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(100)
	cfg.freeUserDelayMinSec = 5 // global is 5s; would be very slow without override
	cfg.freeUserDelayMaxSec = 0
	cfg.chatRateLimiter.SetModelDelay("instant-free", 0, 0)

	raw := buildFreeModelRawHTTP("instant-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 2*time.Second,
		"per-model override of 0 must disable the pre-call delay; elapsed=%v", elapsed)
	assert.NotContains(t, resp, "429",
		"request must pass the RPM check")
}

// TestFreeUserPreCallDelay_PerModelOverrideHigher verifies that a per-model
// delay override greater than the global takes effect.
func TestFreeUserPreCallDelay_PerModelOverrideHigher(t *testing.T) {
	persistDBRateLimitRPM(t, 100)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(100)
	cfg.freeUserDelayMinSec = 0 // disable global
	cfg.freeUserDelayMaxSec = 0
	cfg.chatRateLimiter.SetModelDelay("slow-free", 1, 0) // Max=0 -> 老语义 1~2s

	raw := buildFreeModelRawHTTP("slow-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond,
		"per-model override should override the global default")
	assert.NotContains(t, resp, "429")
}

// TestFreeUserDelay_RangeMinMax 验证：当配置了 Min/Max 时延迟在 [Min, Max] 区间内随机。
// 例如 Min=0, Max=2 -> 实际延迟 0~2 秒。
// 关键词: FreeUserDelay N~M 随机延迟测试
func TestFreeUserDelay_RangeMinMax(t *testing.T) {
	persistDBRateLimitRPM(t, 100)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(100)
	// 配置 Max=2 让区间为 [0, 2] 秒
	cfg.freeUserDelayMinSec = 0
	cfg.freeUserDelayMaxSec = 2

	raw := buildFreeModelRawHTTP("range-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	// 上界稍宽：网络/调度开销允许 < 3s
	assert.Less(t, elapsed, 3*time.Second,
		"delay should not exceed Max + scheduler slack")
	assert.NotContains(t, resp, "429")
}

// TestFreeUserDelay_BackwardCompatN2N 验证：Max=0 时按老语义 N~2N。
// 关键词: FreeUserDelay 兼容 N~2N
func TestFreeUserDelay_BackwardCompatN2N(t *testing.T) {
	// 直接断言 computeJitterDelaySec：Min=2, Max=0 -> 区间 [2, 4]
	for i := 0; i < 50; i++ {
		got := computeJitterDelaySec(2, 0)
		assert.GreaterOrEqual(t, got, int64(2))
		assert.LessOrEqual(t, got, int64(4))
	}

	// Min=0, Max=5 -> 区间 [0, 5]
	for i := 0; i < 50; i++ {
		got := computeJitterDelaySec(0, 5)
		assert.GreaterOrEqual(t, got, int64(0))
		assert.LessOrEqual(t, got, int64(5))
	}

	// Min=3, Max=3 -> 固定 3
	for i := 0; i < 10; i++ {
		assert.Equal(t, int64(3), computeJitterDelaySec(3, 3))
	}

	// Min=0, Max=0 -> 0
	assert.Equal(t, int64(0), computeJitterDelaySec(0, 0))

	// 负值兜底归零
	assert.Equal(t, int64(0), computeJitterDelaySec(-1, -1))
}

// TestFreeUserDelay_PerModelOverrideRange 验证：模型级 (Min, Max) 覆盖生效。
// 关键词: FreeUserDelay 模型级 N~M 覆盖
func TestFreeUserDelay_PerModelOverrideRange(t *testing.T) {
	persistDBRateLimitRPM(t, 100)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(100)
	cfg.freeUserDelayMinSec = 10 // 全局很慢
	cfg.freeUserDelayMaxSec = 20
	cfg.chatRateLimiter.SetModelDelay("range-override-free", 0, 1) // 模型 0~1 秒

	raw := buildFreeModelRawHTTP("range-override-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 2*time.Second,
		"per-model override [0,1] should win over global [10,20]; elapsed=%v", elapsed)
	assert.NotContains(t, resp, "429")
}

// TestRPM_UsesExternalModelName verifies that RPM rate limiting uses
// the external-facing model name (from request), not any internal forwarding name.
func TestRPM_UsesExternalModelName(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(100)
	rl.SetModelRPM("memfit-standard-free", 2)

	key := "test-key"

	// The external name "memfit-standard-free" should be rate limited at RPM=2
	allowed1, _ := rl.CheckRateLimit(key, "memfit-standard-free")
	assert.True(t, allowed1)
	allowed2, _ := rl.CheckRateLimit(key, "memfit-standard-free")
	assert.True(t, allowed2)
	allowed3, _ := rl.CheckRateLimit(key, "memfit-standard-free")
	assert.False(t, allowed3, "third request should be denied (model RPM=2)")

	// An internal forwarded name "gpt-4o" should NOT be rate limited
	// by the "memfit-standard-free" rule (uses default RPM=100)
	allowed4, _ := rl.CheckRateLimit(key, "gpt-4o")
	assert.True(t, allowed4, "internal model name should use default RPM, not memfit override")
}

// TestRPM_FreeUsersPerModelBucket verifies that free users have independent
// RPM buckets per model so that a high-RPM model does not block a low-RPM model.
func TestRPM_FreeUsersPerModelBucket(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(3)

	freeKey := "free-user"

	for i := 0; i < 3; i++ {
		allowed, _ := rl.CheckRateLimit(freeKey, "model-a-free")
		assert.True(t, allowed, "model-a-free request %d should be allowed", i+1)
	}

	denied, _ := rl.CheckRateLimit(freeKey, "model-a-free")
	assert.False(t, denied, "model-a-free 4th request should be denied (RPM=3)")

	allowed, _ := rl.CheckRateLimit(freeKey, "model-b-free")
	assert.True(t, allowed, "model-b-free should have its own bucket and still be allowed")
}

// TestRPM_HighRPMModelDoesNotBlockLowRPM reproduces the original bug:
// a high-RPM model filling the shared bucket would permanently block a low-RPM model.
func TestRPM_HighRPMModelDoesNotBlockLowRPM(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(100)
	rl.SetModelRPM("light-free", 999999)
	rl.SetModelRPM("q1-free", 3)

	freeKey := "free-user"

	for i := 0; i < 50; i++ {
		allowed, _ := rl.CheckRateLimit(freeKey, "light-free")
		assert.True(t, allowed, "light-free request %d should be allowed", i+1)
	}

	allowed, _ := rl.CheckRateLimit(freeKey, "q1-free")
	assert.True(t, allowed, "q1-free should not be blocked by light-free requests")
}
