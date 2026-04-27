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
	cfg.freeUserDelaySec = 1 // global fallback: 1s base, jitter to 1~2s

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
	cfg.freeUserDelaySec = 5 // global is 5s; would be very slow without override
	cfg.chatRateLimiter.SetModelDelay("instant-free", 0)

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
	cfg.freeUserDelaySec = 0 // disable global
	cfg.chatRateLimiter.SetModelDelay("slow-free", 1)

	raw := buildFreeModelRawHTTP("slow-free")

	start := time.Now()
	resp := sendChatCompletionDirect(t, cfg, raw)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond,
		"per-model override should override the global default")
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
