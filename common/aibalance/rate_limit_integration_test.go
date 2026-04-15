package aibalance

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildFreeModelRawHTTP builds a raw HTTP request for a free model (no API key needed).
func buildFreeModelRawHTTP(model string) string {
	body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"hello"}],"stream":false}`, model)
	return fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
}

// sendChatCompletionDirect calls serveChatCompletions directly on the ServerConfig.
func sendChatCompletionDirect(t *testing.T, cfg *ServerConfig, rawHTTP string) string {
	t.Helper()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.serveChatCompletions(server, []byte(rawHTTP))
		server.Close()
	}()

	done := make(chan string, 1)
	go func() {
		var result []byte
		buf := make([]byte, 8192)
		for {
			client.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, err := client.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		done <- string(result)
	}()

	select {
	case resp := <-done:
		return resp
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for chat completion response")
		return ""
	}
}

// persistDBRateLimitRPM sets the RPM in DB so that LoadProvidersFromDatabase
// (triggered when no providers are found) applies the correct RPM.
func persistDBRateLimitRPM(t *testing.T, rpm int64) {
	t.Helper()
	EnsureRateLimitConfigTable()
	rlCfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	rlCfg.DefaultRPM = rpm
	require.NoError(t, SaveRateLimitConfig(rlCfg))
}

func resetDBRateLimitRPM(t *testing.T) {
	t.Helper()
	persistDBRateLimitRPM(t, 600)
}

// ==================== Integration: Free-user end-to-end 429 ====================

func TestChatCompletion_FreeUser_429_FullChain(t *testing.T) {
	persistDBRateLimitRPM(t, 1)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(1)

	raw := buildFreeModelRawHTTP("test-free")

	resp1 := sendChatCompletionDirect(t, cfg, raw)
	assert.NotContains(t, resp1, "429", "first free-user request should not be 429")

	resp2 := sendChatCompletionDirect(t, cfg, raw)
	assert.Contains(t, resp2, "429", "second free-user request should be 429 (RPM=1)")
	assert.Contains(t, resp2, "X-AIBalance-Info", "429 should contain X-AIBalance-Info header")
	assert.Contains(t, resp2, "rate_limit_exceeded", "429 should contain rate_limit_exceeded")
	assert.Contains(t, resp2, "Retry-After: 10", "429 should contain Retry-After header")
}

func TestChatCompletion_FreeUser_RPM2_ThirdDenied(t *testing.T) {
	persistDBRateLimitRPM(t, 2)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(2)

	raw := buildFreeModelRawHTTP("test-free")

	resp1 := sendChatCompletionDirect(t, cfg, raw)
	assert.NotContains(t, resp1, "429", "first request should pass")

	resp2 := sendChatCompletionDirect(t, cfg, raw)
	assert.NotContains(t, resp2, "429", "second request should pass (RPM=2)")

	resp3 := sendChatCompletionDirect(t, cfg, raw)
	assert.Contains(t, resp3, "429", "third request should be 429 (RPM=2 exceeded)")
}

func TestChatCompletion_FreeUser_DifferentModels_IndependentBucket(t *testing.T) {
	persistDBRateLimitRPM(t, 1)
	defer resetDBRateLimitRPM(t)

	cfg := NewServerConfig()
	defer cfg.Close()
	cfg.chatRateLimiter.SetDefaultRPM(1)

	rawA := buildFreeModelRawHTTP("model-a-free")
	rawB := buildFreeModelRawHTTP("model-b-free")

	resp1 := sendChatCompletionDirect(t, cfg, rawA)
	assert.NotContains(t, resp1, "429", "model-a-free first request should pass")

	resp2 := sendChatCompletionDirect(t, cfg, rawA)
	assert.Contains(t, resp2, "429", "model-a-free second request should be 429 (RPM=1)")

	resp3 := sendChatCompletionDirect(t, cfg, rawB)
	assert.NotContains(t, resp3, "429", "model-b-free should have independent bucket and pass")
}

// ==================== Unit: Rate limiter per-key independence (no I/O) ====================

func TestRateLimiter_KeyIsolation(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(1)

	allowed, _ := rl.CheckRateLimit("key-A", "m")
	assert.True(t, allowed)

	allowed2, _ := rl.CheckRateLimit("key-A", "m")
	assert.False(t, allowed2, "key-A second request should be denied")

	allowed3, _ := rl.CheckRateLimit("key-B", "m")
	assert.True(t, allowed3, "key-B first request should be allowed (independent)")
}

func TestRateLimiter_ModelOverrideIsolation(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()
	rl.SetDefaultRPM(100)
	rl.SetModelRPM("expensive", 1)

	allowed, _ := rl.CheckRateLimit("key-1", "expensive")
	assert.True(t, allowed)

	allowed2, _ := rl.CheckRateLimit("key-1", "expensive")
	assert.False(t, allowed2, "expensive model should be denied (model RPM=1)")

	allowed3, _ := rl.CheckRateLimit("key-1", "cheap")
	assert.True(t, allowed3, "cheap model should use default RPM=100")
}

// ==================== Unit: writeRateLimitResponse format ====================

func TestWriteRateLimitResponse_Format(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeRateLimitResponse(server, 42)
		server.Close()
	}()

	var result []byte
	buf := make([]byte, 4096)
	for {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := client.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	resp := string(result)

	assert.Contains(t, resp, "HTTP/1.1 429 Too Many Requests")
	assert.Contains(t, resp, "X-AIBalance-Info: 42")
	assert.Contains(t, resp, "Retry-After: 10")

	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(resp[bodyIdx+4:]), &parsed)
	require.NoError(t, err, "429 body should be valid JSON")
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "rate_limit_exceeded", errObj["type"])
	assert.Equal(t, float64(42), errObj["queue_length"])
	assert.Contains(t, errObj["message"], "Rate limit exceeded")
	assert.Contains(t, errObj["message"], "queue position 42")
}

func TestWriteRateLimitResponse_QueueZero(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeRateLimitResponse(server, 0)
		server.Close()
	}()

	buf := make([]byte, 4096)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := client.Read(buf)
	resp := string(buf[:n])

	assert.Contains(t, resp, "HTTP/1.1 429")
	assert.Contains(t, resp, "X-AIBalance-Info: 0")
}

// ==================== PublicStatsResponse field test ====================

func TestPublicStatsResponse_ContainsQueueCount(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	require.NotNil(t, cfg.chatRateLimiter)
	assert.Equal(t, int64(0), cfg.chatRateLimiter.GetQueueCount(), "initial queue count should be 0")
}

func TestPublicStatsResponse_QueueCountJSON(t *testing.T) {
	resp := PublicStatsResponse{
		QueueCount:         7,
		ConcurrentRequests: 3,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, float64(7), parsed["queue_count"], "should contain queue_count in JSON")
	assert.Equal(t, float64(3), parsed["concurrent_requests"])
}
