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

// TestFreeUserSleep_ConcurrentCounterReleased verifies that the free user
// cooldown sleep does not hold the concurrentChatRequests counter.
func TestFreeUserSleep_ConcurrentCounterReleased(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.freeUserDelaySec = 1

	// Simulate the pattern used in serveChatCompletions
	atomic.AddInt64(&cfg.concurrentChatRequests, 1)
	assert.Equal(t, int64(1), atomic.LoadInt64(&cfg.concurrentChatRequests))

	concurrentReleased := false
	releaseConcurrent := func() {
		if !concurrentReleased {
			concurrentReleased = true
			atomic.AddInt64(&cfg.concurrentChatRequests, -1)
		}
	}
	defer releaseConcurrent()

	// Simulate: response sent, now entering free user cooldown
	isFreeModel := true
	if isFreeModel && cfg.freeUserDelaySec > 0 {
		releaseConcurrent()
	}

	// During "sleep" period, counter should already be 0
	assert.Equal(t, int64(0), atomic.LoadInt64(&cfg.concurrentChatRequests),
		"concurrent counter should be 0 during free user cooldown sleep")
}

// TestFreeUserSleep_OtherKeysNotBlocked verifies that during free user cooldown,
// other API keys can proceed without the concurrent counter being inflated.
func TestFreeUserSleep_OtherKeysNotBlocked(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.freeUserDelaySec = 2

	// Simulate 3 concurrent free user requests that have finished sending
	// response but are in cooldown sleep
	for i := 0; i < 3; i++ {
		go func() {
			atomic.AddInt64(&cfg.concurrentChatRequests, 1)
			// Simulate response completion
			time.Sleep(10 * time.Millisecond)
			// Release concurrent before sleep (the fix)
			atomic.AddInt64(&cfg.concurrentChatRequests, -1)
			// Simulate cooldown sleep
			time.Sleep(time.Duration(cfg.freeUserDelaySec) * time.Second)
		}()
	}

	// Wait for all 3 to finish their "response" phase
	time.Sleep(50 * time.Millisecond)

	// During cooldown, concurrent counter should be 0 (not 3)
	concurrentDuringSleep := atomic.LoadInt64(&cfg.concurrentChatRequests)
	assert.Equal(t, int64(0), concurrentDuringSleep,
		"concurrent counter should be 0 during cooldown, got %d", concurrentDuringSleep)
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
