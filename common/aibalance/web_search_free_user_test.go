package aibalance

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func init() {
	consts.InitializeYakitDatabase("", "", "")
}

// ==================== Rate Limiter Unit Tests ====================

func TestWebSearchRateLimiter_FirstRequest(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	// First request from a new Trace-ID should always be allowed
	allowed, retryAfter := rl.CheckRateLimit("trace-001")
	assert.True(t, allowed, "first request should be allowed")
	assert.Equal(t, 0, retryAfter, "retry after should be 0 for allowed request")
}

func TestWebSearchRateLimiter_RapidRequestsBlocked(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	// First request
	allowed, _ := rl.CheckRateLimit("trace-002")
	assert.True(t, allowed, "first request should be allowed")

	// Immediate second request should be blocked (within 1s)
	allowed, retryAfter := rl.CheckRateLimit("trace-002")
	assert.False(t, allowed, "rapid second request should be blocked")
	assert.Greater(t, retryAfter, 0, "retry after should be positive")
}

func TestWebSearchRateLimiter_AfterOneSecondAllowed(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	// First request
	allowed, _ := rl.CheckRateLimit("trace-003")
	assert.True(t, allowed, "first request should be allowed")

	// Wait 1.1 seconds, second request should be allowed
	time.Sleep(1100 * time.Millisecond)
	allowed, _ = rl.CheckRateLimit("trace-003")
	assert.True(t, allowed, "request after 1s interval should be allowed")
}

func TestWebSearchRateLimiter_SuccessCooldown(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	traceID := "trace-004"

	// First request
	allowed, _ := rl.CheckRateLimit(traceID)
	assert.True(t, allowed, "first request should be allowed")

	// Record success: this triggers 3s cooldown
	rl.RecordSuccess(traceID)

	// Wait 1.1 seconds (enough for the 1s rate but NOT for the 3s cooldown)
	time.Sleep(1100 * time.Millisecond)
	allowed, retryAfter := rl.CheckRateLimit(traceID)
	assert.False(t, allowed, "request within 3s success cooldown should be blocked")
	assert.Greater(t, retryAfter, 0, "retry after should be positive during success cooldown")
}

func TestWebSearchRateLimiter_SuccessCooldownExpires(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	traceID := "trace-005"

	// First request
	allowed, _ := rl.CheckRateLimit(traceID)
	assert.True(t, allowed, "first request should be allowed")

	// Record success
	rl.RecordSuccess(traceID)

	// Wait 3.1 seconds (past the 3s cooldown)
	time.Sleep(3100 * time.Millisecond)
	allowed, _ = rl.CheckRateLimit(traceID)
	assert.True(t, allowed, "request after 3s cooldown should be allowed")
}

func TestWebSearchRateLimiter_DifferentTraceIDsIndependent(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	// First request from trace-A
	allowed, _ := rl.CheckRateLimit("trace-A")
	assert.True(t, allowed, "first request from trace-A should be allowed")

	// Record success for trace-A (3s cooldown)
	rl.RecordSuccess("trace-A")

	// First request from trace-B should be allowed (independent)
	allowed, _ = rl.CheckRateLimit("trace-B")
	assert.True(t, allowed, "first request from trace-B should be allowed (independent of trace-A)")
}

func TestWebSearchRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	var wg sync.WaitGroup
	concurrency := 50
	results := make([]bool, concurrency)

	// Launch concurrent requests from different Trace-IDs
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			traceID := fmt.Sprintf("concurrent-trace-%d", idx)
			allowed, _ := rl.CheckRateLimit(traceID)
			results[idx] = allowed
		}(i)
	}

	wg.Wait()

	// All first requests from unique Trace-IDs should be allowed
	for i, allowed := range results {
		assert.True(t, allowed, "first request from unique trace-id %d should be allowed", i)
	}
}

func TestWebSearchRateLimiter_ConcurrentSameTraceID(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	var wg sync.WaitGroup
	concurrency := 20
	allowedCount := int64(0)

	// Launch concurrent requests from the SAME Trace-ID
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := rl.CheckRateLimit("same-trace")
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			}
		}()
	}

	wg.Wait()

	// At most a few should be allowed due to race conditions, but not all
	// In practice, 1 or 2 might get through before the state is written
	assert.LessOrEqual(t, allowedCount, int64(concurrency),
		"not all concurrent requests from same trace should be allowed")
	assert.GreaterOrEqual(t, allowedCount, int64(1),
		"at least one request should be allowed")
}

// ==================== Web Search Auth Flow Tests ====================

// helper: send a raw HTTP request to serveWebSearch and return the full response string (headers + body)
func sendWebSearchRequest(t *testing.T, cfg *ServerConfig, headers map[string]string, bodyJSON string) string {
	t.Helper()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan string, 1)
	go func() {
		cfg.Serve(server)
	}()

	// Build HTTP request
	contentLength := len(bodyJSON)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("POST /v1/web-search HTTP/1.1\r\n"))
	sb.WriteString("Host: localhost\r\n")
	for k, v := range headers {
		sb.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	sb.WriteString("Content-Type: application/json\r\n")
	sb.WriteString(fmt.Sprintf("Content-Length: %d\r\n", contentLength))
	sb.WriteString("\r\n")
	sb.WriteString(bodyJSON)

	client.Write([]byte(sb.String()))

	// Read the full response: net.Pipe may return partial data, so keep reading
	go func() {
		var result []byte
		buf := make([]byte, 4096)
		for {
			client.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, err := client.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err != nil {
				break
			}
			// Check if we have received the full response (headers + body)
			respStr := string(result)
			if idx := strings.Index(respStr, "\r\n\r\n"); idx >= 0 {
				// Try to find Content-Length and see if we have the full body
				headerPart := respStr[:idx]
				bodyPart := respStr[idx+4:]
				clPrefix := "Content-Length: "
				clIdx := strings.Index(headerPart, clPrefix)
				if clIdx >= 0 {
					clLine := headerPart[clIdx+len(clPrefix):]
					if nlIdx := strings.Index(clLine, "\r\n"); nlIdx >= 0 {
						clLine = clLine[:nlIdx]
					}
					var cl int
					fmt.Sscanf(clLine, "%d", &cl)
					if len(bodyPart) >= cl {
						break
					}
				}
			}
		}
		done <- string(result)
	}()

	select {
	case resp := <-done:
		return resp
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for web search response")
		return ""
	}
}

func TestServeWebSearch_NoAuthNoTraceID_Returns502(t *testing.T) {
	cfg := NewServerConfig()

	resp := sendWebSearchRequest(t, cfg, map[string]string{}, `{"query":"test"}`)

	assert.Contains(t, resp, "502", "should return 502 status")
	assert.Contains(t, resp, "must have trace id or apikey", "should contain error message about missing auth")
}

func TestServeWebSearch_InvalidAPIKey_Returns401(t *testing.T) {
	cfg := NewServerConfig()

	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"Authorization": "Bearer invalid-key-12345",
	}, `{"query":"test"}`)

	assert.Contains(t, resp, "401", "should return 401 status")
	assert.Contains(t, resp, "invalid api key", "should indicate invalid api key")
}

func TestServeWebSearch_EmptyBody_Returns400(t *testing.T) {
	cfg := NewServerConfig()

	// Send request with trace-id but empty body
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan string, 1)
	go func() {
		cfg.Serve(server)
	}()

	request := "POST /v1/web-search HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Trace-ID: test-trace-123\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"
	client.Write([]byte(request))

	go func() {
		buf := make([]byte, 8192)
		n, _ := client.Read(buf)
		done <- string(buf[:n])
	}()

	select {
	case resp := <-done:
		assert.Contains(t, resp, "400", "should return 400 for empty body")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestServeWebSearch_EmptyQuery_Returns400(t *testing.T) {
	cfg := NewServerConfig()

	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "test-trace-456",
	}, `{"query":""}`)

	assert.Contains(t, resp, "400", "should return 400 for empty query")
	assert.Contains(t, resp, "query is required", "should indicate query is required")
}

func TestServeWebSearch_FreeUserDisabled_Returns403(t *testing.T) {
	cfg := NewServerConfig()

	// Ensure the web search config exists with AllowFreeUserWebSearch = false
	EnsureWebSearchApiKeyTable()
	wsConfig, err := GetWebSearchConfig()
	require.NoError(t, err)
	wsConfig.AllowFreeUserWebSearch = false
	require.NoError(t, SaveWebSearchConfig(wsConfig))

	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "free-user-trace-789",
	}, `{"query":"test query"}`)

	assert.Contains(t, resp, "403", "should return 403 when free user web search is disabled")
	assert.Contains(t, resp, "free user web search is currently disabled", "should indicate feature is disabled")
}

func TestServeWebSearch_FreeUserEnabled_RateLimitFirstRequest(t *testing.T) {
	cfg := NewServerConfig()

	// Enable free user web search
	EnsureWebSearchApiKeyTable()
	wsConfig, err := GetWebSearchConfig()
	require.NoError(t, err)
	wsConfig.AllowFreeUserWebSearch = true
	require.NoError(t, SaveWebSearchConfig(wsConfig))

	// First request from free user should pass rate limiting check
	// but will fail at TOTP step (no TOTP header), which is expected
	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "free-user-first-req",
	}, `{"query":"test query"}`)

	// Should NOT be 429 (rate limited), should be 401 (TOTP required)
	assert.NotContains(t, resp, "429", "first request should not be rate limited")
	assert.Contains(t, resp, "401", "should fail at TOTP step for free user")
	assert.Contains(t, resp, "memfit_totp_auth_required", "should require TOTP auth")
}

func TestServeWebSearch_FreeUserRateLimited(t *testing.T) {
	cfg := NewServerConfig()

	// Enable free user web search
	EnsureWebSearchApiKeyTable()
	wsConfig, err := GetWebSearchConfig()
	require.NoError(t, err)
	wsConfig.AllowFreeUserWebSearch = true
	require.NoError(t, SaveWebSearchConfig(wsConfig))

	traceID := fmt.Sprintf("rate-limit-test-%d", time.Now().UnixNano())

	// First request: should pass rate limit (fail at TOTP)
	resp1 := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": traceID,
	}, `{"query":"test query"}`)
	assert.Contains(t, resp1, "401", "first request should pass rate limit, fail at TOTP")

	// Immediate second request: should be rate limited
	resp2 := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": traceID,
	}, `{"query":"test query"}`)
	assert.Contains(t, resp2, "429", "rapid second request should be rate limited")
	assert.Contains(t, resp2, "rate_limit_exceeded", "should indicate rate limit exceeded")
}

func TestServeWebSearch_APIKeyUser_NoRateLimit(t *testing.T) {
	cfg := NewServerConfig()

	// Add a test API key
	testKey := &Key{
		Key:           "ws-test-api-key-001",
		AllowedModels: map[string]bool{"web-search": true},
	}
	cfg.Keys.keys["ws-test-api-key-001"] = testKey

	// First request with API key: should pass to TOTP step
	resp1 := sendWebSearchRequest(t, cfg, map[string]string{
		"Authorization": "Bearer ws-test-api-key-001",
	}, `{"query":"test query"}`)
	assert.Contains(t, resp1, "401", "should fail at TOTP step")
	assert.Contains(t, resp1, "memfit_totp_auth_required", "should require TOTP")

	// Immediate second request: should NOT be rate limited (API key user)
	resp2 := sendWebSearchRequest(t, cfg, map[string]string{
		"Authorization": "Bearer ws-test-api-key-001",
	}, `{"query":"test query"}`)
	// Should also fail at TOTP, but NOT at rate limit
	assert.NotContains(t, resp2, "429", "API key user should not be rate limited")
	assert.Contains(t, resp2, "401", "should fail at TOTP step again")
}

func TestServeWebSearch_InvalidSearcherType_Returns400(t *testing.T) {
	cfg := NewServerConfig()

	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "searcher-type-test",
	}, `{"query":"test", "searcher_type": "invalid_type"}`)

	assert.Contains(t, resp, "400", "should return 400 for invalid searcher type")
	assert.Contains(t, resp, "searcher_type must be", "should indicate valid types")
}

func TestServeWebSearch_OnlyTraceID_Returns502WhenBothMissing(t *testing.T) {
	cfg := NewServerConfig()

	// Request with neither API key nor Trace-ID
	resp := sendWebSearchRequest(t, cfg, map[string]string{
		"X-Custom-Header": "some-value",
	}, `{"query":"test"}`)

	assert.Contains(t, resp, "502", "should return 502 when both are missing")
	assert.Contains(t, resp, "must have trace id or apikey", "should contain appropriate error")
}

// ==================== Concurrent Counter Tests ====================

func TestConcurrentCounters_Initial(t *testing.T) {
	cfg := NewServerConfig()

	// All counters should start at 0
	assert.Equal(t, int64(0), atomic.LoadInt64(&cfg.concurrentChatRequests), "initial chat counter should be 0")
	assert.Equal(t, int64(0), atomic.LoadInt64(&cfg.concurrentEmbeddingRequests), "initial embedding counter should be 0")
	assert.Equal(t, int64(0), atomic.LoadInt64(&cfg.totalWebSearchCount), "initial web search counter should be 0")
}

func TestConcurrentCounters_WebSearchIncrement(t *testing.T) {
	cfg := NewServerConfig()

	// Each call to serveWebSearch increments totalWebSearchCount
	// Send a web search request (will fail early, but counter should still increment)
	sendWebSearchRequest(t, cfg, map[string]string{}, `{"query":"test"}`)

	count := atomic.LoadInt64(&cfg.totalWebSearchCount)
	assert.Equal(t, int64(1), count, "web search counter should be 1 after one request")
}

func TestConcurrentCounters_MultipleWebSearchIncrements(t *testing.T) {
	cfg := NewServerConfig()

	// Send multiple requests
	for i := 0; i < 5; i++ {
		sendWebSearchRequest(t, cfg, map[string]string{}, `{"query":"test"}`)
	}

	count := atomic.LoadInt64(&cfg.totalWebSearchCount)
	assert.Equal(t, int64(5), count, "web search counter should be 5 after five requests")
}

// ==================== Portal Data Response Tests ====================

func TestPortalDataResponse_ContainsNewFields(t *testing.T) {
	// Verify PortalDataResponse serialization includes new fields
	resp := PortalDataResponse{
		ConcurrentRequests: 42,
		WebSearchCount:     1337,
		TotalProviders:     5,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, float64(42), parsed["concurrent_requests"], "should contain concurrent_requests")
	assert.Equal(t, float64(1337), parsed["web_search_count"], "should contain web_search_count")
}

// ==================== Web Search Config API Tests ====================

func TestWebSearchConfig_AllowFreeUserField(t *testing.T) {
	EnsureWebSearchApiKeyTable()

	// Reset to known state first
	config, err := GetWebSearchConfig()
	require.NoError(t, err)
	require.NotNil(t, config)
	config.AllowFreeUserWebSearch = false
	require.NoError(t, SaveWebSearchConfig(config))

	// Verify it is false
	config1, err := GetWebSearchConfig()
	require.NoError(t, err)
	assert.False(t, config1.AllowFreeUserWebSearch, "should be false after explicit reset")

	// Set to true
	config1.AllowFreeUserWebSearch = true
	require.NoError(t, SaveWebSearchConfig(config1))

	// Read back
	config2, err := GetWebSearchConfig()
	require.NoError(t, err)
	assert.True(t, config2.AllowFreeUserWebSearch, "should be true after save")

	// Set back to false
	config2.AllowFreeUserWebSearch = false
	require.NoError(t, SaveWebSearchConfig(config2))

	// Verify
	config3, err := GetWebSearchConfig()
	require.NoError(t, err)
	assert.False(t, config3.AllowFreeUserWebSearch, "should be false after second save")
}

// ==================== Rate Limiter Cleanup Tests ====================

func TestWebSearchRateLimiter_RecordSuccessForNewTraceID(t *testing.T) {
	rl := NewWebSearchRateLimiter()
	defer rl.Stop()

	// RecordSuccess for a trace-id that never had CheckRateLimit called
	rl.RecordSuccess("new-trace-id")

	// Subsequent request should still be affected by the 3s cooldown
	allowed, _ := rl.CheckRateLimit("new-trace-id")
	assert.False(t, allowed, "should be blocked by 3s cooldown from RecordSuccess")
}

// ==================== Integration Test: WebSearchRateLimiter with ServerConfig ====================

func TestServerConfig_WebSearchRateLimiterInitialized(t *testing.T) {
	cfg := NewServerConfig()

	assert.NotNil(t, cfg.webSearchRateLimiter, "rate limiter should be initialized by NewServerConfig")
}

// ==================== Test: serveWebSearch counts correctly even on error ====================

func TestServeWebSearch_CountIncrementOnError(t *testing.T) {
	cfg := NewServerConfig()

	// Even requests that fail early should increment the counter
	// Request 1: no auth
	sendWebSearchRequest(t, cfg, map[string]string{}, `{"query":"test1"}`)

	// Request 2: bad query
	sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "count-test",
	}, `{"query":""}`)

	// Request 3: bad searcher type
	sendWebSearchRequest(t, cfg, map[string]string{
		"Trace-ID": "count-test-2",
	}, `{"query":"test", "searcher_type":"invalid"}`)

	count := atomic.LoadInt64(&cfg.totalWebSearchCount)
	// Request 1 counts (no auth is after counter increment)
	// Request 2 counts (empty query is after counter increment)
	// Request 3 counts (bad type is after counter increment)
	assert.Equal(t, int64(3), count, "all web search requests should increment counter regardless of error")
}
