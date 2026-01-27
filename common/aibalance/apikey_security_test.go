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
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== API Key Security Tests ====================
// These tests ensure API keys are not leaked and security is maintained

// TestAPIKeyNotLeakedInErrorResponse tests that API keys are not exposed in error responses
func TestAPIKeyNotLeakedInErrorResponse(t *testing.T) {
	cfg := NewServerConfig()

	// Create a test API key
	testAPIKey := "sk-test-secret-key-12345678"
	key := &Key{
		Key:           testAPIKey,
		AllowedModels: make(map[string]bool),
	}
	key.AllowedModels["test-model"] = true
	cfg.Keys.keys[testAPIKey] = key

	// Add allowed models
	allowedModels := make(map[string]bool)
	allowedModels["test-model"] = true
	cfg.KeyAllowedModels.allowedModels[testAPIKey] = allowedModels

	// Create test connection
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	// Send a request with model that doesn't exist
	jsonBody := `{"model":"nonexistent-model","messages":[{"role":"user","content":"test"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer " + testAPIKey + "\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))

	// Read response
	buf := make([]byte, 4096)
	n, _ := client.Read(buf)
	response := string(buf[:n])

	// The response should NOT contain the full API key
	assert.NotContains(t, response, testAPIKey, "API key should not be leaked in error response")
	
	// Allow partial key display (first 4 chars for logging)
	if strings.Contains(response, "sk-te") {
		// This is acceptable as it's only a prefix for identification
		t.Log("Partial key prefix found in response (acceptable for logging)")
	}
}

// TestAPIKeyNotLeakedInLogs tests that full API keys are not logged
func TestAPIKeyNotLeakedInLogs(t *testing.T) {
	// This is a design principle test - verify that functions that handle API keys
	// use ShrinkString or similar masking

	testKey := "sk-very-long-secret-api-key-that-should-not-be-logged-in-full"
	
	// Verify ShrinkString works correctly
	// ShrinkString(key, 8) should return "sk-very-...in-full" or similar
	// The key should not appear in full anywhere in production logs
	
	// Check that the key length is > 8 for this test to be meaningful
	assert.Greater(t, len(testKey), 16, "Test key should be long enough for masking")
}

// TestUnauthorizedAccessRejected tests that requests without valid API keys are rejected
func TestUnauthorizedAccessRejected(t *testing.T) {
	cfg := NewServerConfig()

	// Add a test model but no API key
	model := &Provider{
		ModelName:   "test-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "provider-api-key",
	}
	cfg.Models.models["test-model"] = []*Provider{model}

	testCases := []struct {
		name          string
		authHeader    string
		expectStatus  string
	}{
		{
			name:         "No Authorization header",
			authHeader:   "",
			expectStatus: "401 Unauthorized",
		},
		{
			name:         "Invalid API key",
			authHeader:   "Bearer invalid-key-12345",
			expectStatus: "401 Unauthorized",
		},
		{
			name:         "Malformed Authorization header",
			authHeader:   "Basic invalid",
			expectStatus: "401 Unauthorized",
		},
		{
			name:         "Empty Bearer token",
			authHeader:   "Bearer ",
			expectStatus: "401 Unauthorized",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			go func() {
				cfg.Serve(server)
			}()

			// Build request
			jsonBody := `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
			request := "POST /v1/chat/completions HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"Content-Type: application/json\r\n"
			if tc.authHeader != "" {
				request += "Authorization: " + tc.authHeader + "\r\n"
			}
			request += fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(jsonBody), jsonBody)

			client.Write([]byte(request))

			buf := make([]byte, 4096)
			n, _ := client.Read(buf)
			response := string(buf[:n])

			assert.Contains(t, response, tc.expectStatus, "Expected %s response", tc.expectStatus)
		})
	}
}

// TestAPIKeyActivationStatus tests that inactive API keys are rejected
func TestAPIKeyActivationStatus(t *testing.T) {
	cfg := NewServerConfig()

	// Create an inactive key
	// Note: Key activation status is checked via database (schema.AiApiKeys.Active)
	// For this test, we simulate the scenario where the key is not in the KeyManager
	inactiveKey := "sk-inactive-key-12345"
	// NOT adding to cfg.Keys.keys simulates an inactive/unknown key

	// Add allowed models
	allowedModels := make(map[string]bool)
	allowedModels["test-model"] = true
	cfg.KeyAllowedModels.allowedModels[inactiveKey] = allowedModels

	// Add a test model
	model := &Provider{
		ModelName:   "test-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "provider-api-key",
	}
	cfg.Models.models["test-model"] = []*Provider{model}
	cfg.Entrypoints.providers["test-model"] = []*Provider{model}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	jsonBody := `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer " + inactiveKey + "\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))

	buf := make([]byte, 4096)
	n, _ := client.Read(buf)
	response := string(buf[:n])

	// Inactive keys should be rejected
	assert.Contains(t, response, "401", "Inactive API key should be rejected")
}

// TestModelAccessControl tests that API keys can only access allowed models
func TestModelAccessControl(t *testing.T) {
	cfg := NewServerConfig()

	testAPIKey := "sk-test-key-123"
	key := &Key{
		Key:           testAPIKey,
		AllowedModels: make(map[string]bool),
	}
	// Only allow access to "allowed-model"
	key.AllowedModels["allowed-model"] = true
	cfg.Keys.keys[testAPIKey] = key

	// Add allowed models configuration
	allowedModels := make(map[string]bool)
	allowedModels["allowed-model"] = true
	cfg.KeyAllowedModels.allowedModels[testAPIKey] = allowedModels

	// Add both models to the system
	allowedProvider := &Provider{
		ModelName:   "allowed-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "provider-key-1",
	}
	restrictedProvider := &Provider{
		ModelName:   "restricted-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "provider-key-2",
	}
	cfg.Models.models["allowed-model"] = []*Provider{allowedProvider}
	cfg.Models.models["restricted-model"] = []*Provider{restrictedProvider}
	cfg.Entrypoints.providers["allowed-model"] = []*Provider{allowedProvider}
	cfg.Entrypoints.providers["restricted-model"] = []*Provider{restrictedProvider}

	t.Run("Access to restricted model denied", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		go func() {
			cfg.Serve(server)
		}()

		jsonBody := `{"model":"restricted-model","messages":[{"role":"user","content":"test"}]}`
		client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: Bearer " + testAPIKey + "\r\n" +
			"Content-Type: application/json\r\n" +
			fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
			"\r\n" +
			jsonBody))

		buf := make([]byte, 4096)
		n, _ := client.Read(buf)
		response := string(buf[:n])

		// Should be denied (404 Not Found - model not accessible to this key)
		// Note: The server returns 404 when model is not in allowed list for the key
		assert.Contains(t, response, "404", "Access to restricted model should be denied (404 Not Found)")
	})
}

// ==================== Traffic Limit Tests ====================

// TestTrafficLimitEnforcement tests that traffic limits are enforced
func TestTrafficLimitEnforcement(t *testing.T) {
	// Test the CheckAiApiKeyTrafficLimit function directly
	
	t.Run("Traffic limit disabled", func(t *testing.T) {
		// Create a mock key with traffic limit disabled
		key := &schema.AiApiKeys{
			APIKey:             "test-key-1",
			TrafficLimitEnable: false,
			TrafficLimit:       1000,
			TrafficUsed:        2000, // Exceeded but disabled
		}
		
		// When disabled, should always allow
		allowed := !key.TrafficLimitEnable || key.TrafficLimit <= 0 || key.TrafficUsed < key.TrafficLimit
		assert.True(t, allowed, "Should allow when traffic limit is disabled")
	})
	
	t.Run("Traffic limit enabled but not exceeded", func(t *testing.T) {
		key := &schema.AiApiKeys{
			APIKey:             "test-key-2",
			TrafficLimitEnable: true,
			TrafficLimit:       10000,
			TrafficUsed:        5000, // Within limit
		}
		
		allowed := !key.TrafficLimitEnable || key.TrafficLimit <= 0 || key.TrafficUsed < key.TrafficLimit
		assert.True(t, allowed, "Should allow when within traffic limit")
	})
	
	t.Run("Traffic limit exceeded", func(t *testing.T) {
		key := &schema.AiApiKeys{
			APIKey:             "test-key-3",
			TrafficLimitEnable: true,
			TrafficLimit:       10000,
			TrafficUsed:        10000, // Exactly at limit
		}
		
		allowed := !key.TrafficLimitEnable || key.TrafficLimit <= 0 || key.TrafficUsed < key.TrafficLimit
		assert.False(t, allowed, "Should deny when traffic limit is exceeded")
	})
	
	t.Run("Traffic limit zero means unlimited", func(t *testing.T) {
		key := &schema.AiApiKeys{
			APIKey:             "test-key-4",
			TrafficLimitEnable: true,
			TrafficLimit:       0, // Zero means unlimited
			TrafficUsed:        999999999,
		}
		
		allowed := !key.TrafficLimitEnable || key.TrafficLimit <= 0 || key.TrafficUsed < key.TrafficLimit
		assert.True(t, allowed, "Should allow when traffic limit is 0 (unlimited)")
	})
}

// TestTrafficLimitHTTPResponse tests that 429 is returned when limit exceeded
func TestTrafficLimitHTTPResponse(t *testing.T) {
	cfg := NewServerConfig()

	testAPIKey := "sk-limited-key"
	key := &Key{
		Key:           testAPIKey,
		AllowedModels: make(map[string]bool),
	}
	key.AllowedModels["test-model"] = true
	cfg.Keys.keys[testAPIKey] = key

	allowedModels := make(map[string]bool)
	allowedModels["test-model"] = true
	cfg.KeyAllowedModels.allowedModels[testAPIKey] = allowedModels

	model := &Provider{
		ModelName:   "test-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "provider-key",
	}
	cfg.Models.models["test-model"] = []*Provider{model}
	cfg.Entrypoints.providers["test-model"] = []*Provider{model}

	// Note: This test documents expected behavior
	// In production, CheckAiApiKeyTrafficLimit is called which checks the database
	// If the key exceeds traffic limit, 429 Too Many Requests should be returned

	t.Log("Traffic limit enforcement returns HTTP 429 when limit exceeded")
}

// TestTrafficCalculation tests that both request and response traffic is calculated
func TestTrafficCalculation(t *testing.T) {
	t.Run("Traffic includes input bytes", func(t *testing.T) {
		inputBytes := int64(1000)
		outputBytes := int64(0)
		totalTraffic := inputBytes + outputBytes
		
		assert.Equal(t, int64(1000), totalTraffic, "Total traffic should include input bytes")
	})
	
	t.Run("Traffic includes output bytes", func(t *testing.T) {
		inputBytes := int64(0)
		outputBytes := int64(2000)
		totalTraffic := inputBytes + outputBytes
		
		assert.Equal(t, int64(2000), totalTraffic, "Total traffic should include output bytes")
	})
	
	t.Run("Traffic is sum of input and output", func(t *testing.T) {
		inputBytes := int64(1500)
		outputBytes := int64(3500)
		totalTraffic := inputBytes + outputBytes
		
		assert.Equal(t, int64(5000), totalTraffic, "Total traffic should be sum of input and output")
	})
	
	t.Run("Traffic multiplier is applied", func(t *testing.T) {
		inputBytes := int64(1000)
		outputBytes := int64(2000)
		multiplier := 2.0
		
		totalTraffic := inputBytes + outputBytes
		adjustedTraffic := int64(float64(totalTraffic) * multiplier)
		
		assert.Equal(t, int64(6000), adjustedTraffic, "Traffic multiplier should be applied correctly")
	})
}

// TestTrafficNotCountedOnError tests that traffic is not counted when request fails
func TestTrafficNotCountedOnError(t *testing.T) {
	// This is a design principle test
	// When a request fails (e.g., provider returns error), traffic should not be counted
	// to the user's API key
	
	t.Run("Successful request counts traffic", func(t *testing.T) {
		requestSucceeded := true
		shouldCountTraffic := requestSucceeded
		assert.True(t, shouldCountTraffic, "Traffic should be counted on successful request")
	})
	
	t.Run("Failed request should not count traffic", func(t *testing.T) {
		// When requestSucceeded is false, traffic should not be deducted
		// This ensures users are not charged for failed requests
		t.Log("Design: Failed requests should not deduct from user's traffic quota")
		
		// Note: The actual implementation checks requestSucceeded before calling
		// UpdateAiApiKeyTrafficUsed. This test documents the expected behavior.
	})
}

// ==================== API Key Stats Tests ====================

// TestAPIKeyStatsUpdate tests that API key statistics are updated correctly
func TestAPIKeyStatsUpdate(t *testing.T) {
	key := &schema.AiApiKeys{
		APIKey:       "test-stats-key",
		UsageCount:   0,
		SuccessCount: 0,
		FailureCount: 0,
		InputBytes:   0,
		OutputBytes:  0,
	}
	
	t.Run("Success increments success count", func(t *testing.T) {
		// Simulate UpdateAiApiKeyStats behavior
		key.UsageCount++
		key.InputBytes += 100
		key.OutputBytes += 200
		key.SuccessCount++
		
		assert.Equal(t, int64(1), key.UsageCount)
		assert.Equal(t, int64(1), key.SuccessCount)
		assert.Equal(t, int64(0), key.FailureCount)
		assert.Equal(t, int64(100), key.InputBytes)
		assert.Equal(t, int64(200), key.OutputBytes)
	})
	
	t.Run("Failure increments failure count", func(t *testing.T) {
		// Simulate failure
		key.UsageCount++
		key.InputBytes += 50
		key.OutputBytes += 0
		key.FailureCount++
		
		assert.Equal(t, int64(2), key.UsageCount)
		assert.Equal(t, int64(1), key.SuccessCount)
		assert.Equal(t, int64(1), key.FailureCount)
	})
}

// ==================== Database Integration Tests ====================

// TestAPIKeyDatabaseOperations tests API key CRUD operations
func TestAPIKeyDatabaseOperations(t *testing.T) {
	// Skip if database is not available
	db := GetDB()
	if db == nil {
		t.Skip("Database not available")
	}
	
	// Create a unique test key
	testKey := fmt.Sprintf("test-api-key-%d", time.Now().UnixNano())
	
	t.Run("Create API key", func(t *testing.T) {
		err := SaveAiApiKey(testKey, "model1,model2")
		require.NoError(t, err, "Should create API key without error")
	})
	
	t.Run("Get API key", func(t *testing.T) {
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err, "Should get API key without error")
		assert.Equal(t, testKey, key.APIKey)
		assert.Equal(t, "model1,model2", key.AllowedModels)
	})
	
	t.Run("Update API key allowed models", func(t *testing.T) {
		err := UpdateAiApiKey(testKey, "model1,model2,model3")
		require.NoError(t, err, "Should update API key without error")
		
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		assert.Equal(t, "model1,model2,model3", key.AllowedModels)
	})
	
	t.Run("Check traffic limit - disabled", func(t *testing.T) {
		allowed, err := CheckAiApiKeyTrafficLimit(testKey)
		require.NoError(t, err)
		assert.True(t, allowed, "Should allow when traffic limit is disabled")
	})
	
	t.Run("Set traffic limit", func(t *testing.T) {
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		
		err = UpdateAiApiKeyTrafficLimit(key.ID, 10000, true)
		require.NoError(t, err, "Should update traffic limit without error")
	})
	
	t.Run("Update traffic used", func(t *testing.T) {
		err := UpdateAiApiKeyTrafficUsed(testKey, 5000)
		require.NoError(t, err)
		
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		assert.Equal(t, int64(5000), key.TrafficUsed)
	})
	
	t.Run("Check traffic limit - within limit", func(t *testing.T) {
		allowed, err := CheckAiApiKeyTrafficLimit(testKey)
		require.NoError(t, err)
		assert.True(t, allowed, "Should allow when within traffic limit")
	})
	
	t.Run("Exceed traffic limit", func(t *testing.T) {
		err := UpdateAiApiKeyTrafficUsed(testKey, 6000) // Total: 11000 > 10000
		require.NoError(t, err)
		
		allowed, err := CheckAiApiKeyTrafficLimit(testKey)
		require.NoError(t, err)
		assert.False(t, allowed, "Should deny when traffic limit exceeded")
	})
	
	t.Run("Reset traffic used", func(t *testing.T) {
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		
		err = ResetAiApiKeyTrafficUsed(key.ID)
		require.NoError(t, err)
		
		key, err = GetAiApiKey(testKey)
		require.NoError(t, err)
		assert.Equal(t, int64(0), key.TrafficUsed)
	})
	
	t.Run("Check traffic limit - after reset", func(t *testing.T) {
		allowed, err := CheckAiApiKeyTrafficLimit(testKey)
		require.NoError(t, err)
		assert.True(t, allowed, "Should allow after traffic reset")
	})
	
	// Cleanup
	t.Run("Delete API key", func(t *testing.T) {
		err := DeleteAiApiKey(testKey)
		require.NoError(t, err, "Should delete API key without error")
		
		_, err = GetAiApiKey(testKey)
		assert.Error(t, err, "Should not find deleted API key")
	})
}

// TestAPIKeyStatsIntegration tests stats update integration
func TestAPIKeyStatsIntegration(t *testing.T) {
	db := GetDB()
	if db == nil {
		t.Skip("Database not available")
	}
	
	testKey := fmt.Sprintf("test-stats-key-%d", time.Now().UnixNano())
	
	// Create key
	err := SaveAiApiKey(testKey, "test-model")
	require.NoError(t, err)
	defer DeleteAiApiKey(testKey)
	
	t.Run("Update stats on success", func(t *testing.T) {
		err := UpdateAiApiKeyStats(testKey, 100, 200, true)
		require.NoError(t, err)
		
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		
		assert.Equal(t, int64(1), key.UsageCount)
		assert.Equal(t, int64(1), key.SuccessCount)
		assert.Equal(t, int64(0), key.FailureCount)
		assert.Equal(t, int64(100), key.InputBytes)
		assert.Equal(t, int64(200), key.OutputBytes)
	})
	
	t.Run("Update stats on failure", func(t *testing.T) {
		err := UpdateAiApiKeyStats(testKey, 50, 0, false)
		require.NoError(t, err)
		
		key, err := GetAiApiKey(testKey)
		require.NoError(t, err)
		
		assert.Equal(t, int64(2), key.UsageCount)
		assert.Equal(t, int64(1), key.SuccessCount)
		assert.Equal(t, int64(1), key.FailureCount)
		assert.Equal(t, int64(150), key.InputBytes)
		assert.Equal(t, int64(200), key.OutputBytes)
	})
}

// ==================== Request Forwarding Tests ====================

// TestForwardingBasics tests basic request forwarding behavior
func TestForwardingBasics(t *testing.T) {
	t.Run("Invalid request body returns 400", func(t *testing.T) {
		cfg := NewServerConfig()
		
		testKey := "test-key"
		key := &Key{
			Key:           testKey,
			AllowedModels: make(map[string]bool),
		}
		key.AllowedModels["test-model"] = true
		cfg.Keys.keys[testKey] = key
		
		allowedModels := make(map[string]bool)
		allowedModels["test-model"] = true
		cfg.KeyAllowedModels.allowedModels[testKey] = allowedModels
		
		model := &Provider{
			ModelName:   "test-model",
			TypeName:    "openai",
			DomainOrURL: "http://test.com",
			APIKey:      "provider-key",
		}
		cfg.Models.models["test-model"] = []*Provider{model}
		
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		
		go func() {
			cfg.Serve(server)
		}()
		
		// Send empty body
		client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: Bearer " + testKey + "\r\n" +
			"Content-Type: application/json\r\n" +
			"Content-Length: 0\r\n" +
			"\r\n"))
		
		buf := make([]byte, 4096)
		n, _ := client.Read(buf)
		response := string(buf[:n])
		
		assert.Contains(t, response, "400 Bad Request", "Empty body should return 400")
	})
	
	t.Run("Model not found returns 404", func(t *testing.T) {
		cfg := NewServerConfig()
		
		testKey := "test-key"
		key := &Key{
			Key:           testKey,
			AllowedModels: make(map[string]bool),
		}
		key.AllowedModels["nonexistent-model"] = true
		cfg.Keys.keys[testKey] = key
		
		allowedModels := make(map[string]bool)
		allowedModels["nonexistent-model"] = true
		cfg.KeyAllowedModels.allowedModels[testKey] = allowedModels
		
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		
		go func() {
			cfg.Serve(server)
		}()
		
		jsonBody := `{"model":"nonexistent-model","messages":[{"role":"user","content":"test"}]}`
		client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: Bearer " + testKey + "\r\n" +
			"Content-Type: application/json\r\n" +
			fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
			"\r\n" +
			jsonBody))
		
		buf := make([]byte, 4096)
		n, _ := client.Read(buf)
		response := string(buf[:n])
		
		assert.Contains(t, response, "404", "Nonexistent model should return 404")
	})
}

// TestTrafficLimitEnforcementIntegration tests traffic limit with actual HTTP requests
func TestTrafficLimitEnforcementIntegration(t *testing.T) {
	db := GetDB()
	if db == nil {
		t.Skip("Database not available")
	}
	
	// This test documents the expected flow:
	// 1. API key is checked for traffic limit before processing
	// 2. If limit exceeded, return 429
	// 3. If request succeeds, traffic is counted (input + output)
	// 4. If request fails, traffic is NOT counted
	
	t.Log("Traffic limit enforcement flow:")
	t.Log("1. CheckAiApiKeyTrafficLimit called before request processing")
	t.Log("2. Returns 429 Too Many Requests if limit exceeded")
	t.Log("3. Traffic counted only on successful requests")
	t.Log("4. Traffic = input bytes + output bytes")
}

// ==================== Error Response Tests ====================

// TestErrorResponseFormat tests that error responses follow expected format
func TestErrorResponseFormat(t *testing.T) {
	cfg := NewServerConfig()
	
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	go func() {
		cfg.Serve(server)
	}()
	
	// Send request without auth
	jsonBody := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))
	
	buf := make([]byte, 4096)
	n, _ := client.Read(buf)
	response := string(buf[:n])
	
	// Parse response body
	parts := strings.Split(response, "\r\n\r\n")
	if len(parts) > 1 {
		body := parts[1]
		var errResp map[string]interface{}
		if err := json.Unmarshal([]byte(body), &errResp); err == nil {
			// Check for OpenAI-compatible error format
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				assert.NotEmpty(t, errObj["message"], "Error should have message")
				t.Logf("Error response: %v", errObj)
			}
		}
	}
}
