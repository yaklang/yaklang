package aibalance

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== Input Validation Tests ====================

func TestValidateAmapApiKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid key - alphanumeric",
			key:       "abc123def456",
			expectErr: false,
		},
		{
			name:      "valid key - with hyphens",
			key:       "abc-123-def-456",
			expectErr: false,
		},
		{
			name:      "valid key - with underscores",
			key:       "abc_123_def_456",
			expectErr: false,
		},
		{
			name:      "empty key",
			key:       "",
			expectErr: true,
			errMsg:    "empty key",
		},
		{
			name:      "key too long",
			key:       strings.Repeat("a", maxAmapApiKeyLength+1),
			expectErr: true,
			errMsg:    "key too long",
		},
		{
			name:      "key at max length is valid",
			key:       strings.Repeat("a", maxAmapApiKeyLength),
			expectErr: false,
		},
		{
			name:      "key with special chars - angle brackets (XSS attempt)",
			key:       "<script>alert(1)</script>",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with special chars - quotes (XSS attempt)",
			key:       `abc"onmouseover="alert(1)`,
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with special chars - ampersand",
			key:       "abc&def",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with spaces",
			key:       "abc def",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with SQL injection attempt",
			key:       "abc'; DROP TABLE amap_api_keys; --",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with null byte",
			key:       "abc\x00def",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with newline (injection attempt)",
			key:       "abc\ndef",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "key with unicode",
			key:       "abc\u202edef",
			expectErr: true,
			errMsg:    "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAmapApiKey(tt.key)
			if tt.expectErr {
				require.Error(t, err, "expected error for key: %q", tt.key)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err, "expected no error for key: %q", tt.key)
			}
		})
	}
}

func TestAmapApiKeyPattern(t *testing.T) {
	// Verify the regex pattern is correct
	assert.True(t, amapApiKeyPattern.MatchString("abc123"), "alphanumeric should match")
	assert.True(t, amapApiKeyPattern.MatchString("abc-123"), "hyphens should match")
	assert.True(t, amapApiKeyPattern.MatchString("abc_123"), "underscores should match")
	assert.False(t, amapApiKeyPattern.MatchString(""), "empty should not match")
	assert.False(t, amapApiKeyPattern.MatchString("abc def"), "spaces should not match")
	assert.False(t, amapApiKeyPattern.MatchString("abc<def"), "angle brackets should not match")
	assert.False(t, amapApiKeyPattern.MatchString("abc\"def"), "quotes should not match")
	assert.False(t, amapApiKeyPattern.MatchString("abc'def"), "single quotes should not match")
}

func TestBatchSizeLimit(t *testing.T) {
	assert.Equal(t, 100, maxAmapBatchSize, "batch size should be 100")
	assert.Equal(t, 128, maxAmapApiKeyLength, "max key length should be 128")
	assert.Equal(t, 64*1024, maxAmapRequestBodySize, "max body size should be 64KB")
}

// ==================== Output Sanitization Tests ====================

func TestSanitizeForOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "HTML tags",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:     "double quotes",
			input:    `value="injected"`,
			expected: `value=&#34;injected&#34;`,
		},
		{
			name:     "ampersand",
			input:    "a&b",
			expected: "a&amp;b",
		},
		{
			name:     "mixed HTML entities",
			input:    `<img src=x onerror="alert(1)">`,
			expected: `&lt;img src=x onerror=&#34;alert(1)&#34;&gt;`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "normal error message",
			input:    "amap returned error: info=INVALID_USER_KEY, infocode=10001",
			expected: "amap returned error: info=INVALID_USER_KEY, infocode=10001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForOutput(tt.input)
			assert.Equal(t, tt.expected, result)
			// Verify it matches html.EscapeString behavior
			assert.Equal(t, html.EscapeString(tt.input), result)
		})
	}
}

func TestMaskAPIKeyDoesNotLeakFullKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		masked  string
		noLeak  bool // the full key should NOT appear in the masked version
	}{
		{
			name:   "normal key",
			key:    "abcdefghijklmnop",
			masked: maskAPIKey("abcdefghijklmnop"),
			noLeak: true,
		},
		{
			name:   "short key",
			key:    "abc",
			masked: maskAPIKey("abc"),
			noLeak: true,
		},
		{
			name:   "empty key",
			key:    "",
			masked: maskAPIKey(""),
			noLeak: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noLeak && len(tt.key) > 8 {
				// For keys > 8 chars, the full key should not equal the masked version
				assert.NotEqual(t, tt.key, tt.masked, "full key should not be equal to masked version")
				assert.Contains(t, tt.masked, "****", "masked key should contain asterisks")
			}
		})
	}
}

// ==================== Authentication Tests ====================

func TestAmapEndpointsRequireAuthentication(t *testing.T) {
	// All Amap portal endpoints should NOT be public
	amapEndpoints := []struct {
		path   string
		method string
	}{
		{"/portal/api/amap-keys", "GET"},
		{"/portal/api/amap-keys", "POST"},
		{"/portal/api/amap-keys/123", "DELETE"},
		{"/portal/toggle-amap-key/keys/123", "POST"},
		{"/portal/reset-amap-key-health/keys/123", "POST"},
		{"/portal/test-amap-key/keys/123", "POST"},
		{"/portal/api/amap-keys/check-all", "POST"},
		{"/portal/api/amap-config", "GET"},
		{"/portal/api/amap-config", "POST"},
	}

	for _, ep := range amapEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			// Create a request without any authentication
			req := httptest.NewRequest(ep.method, ep.path, nil)

			// Create a server config and check that these require auth
			config := NewServerConfig()
			assert.NotNil(t, config, "server config should not be nil")

			// checkAuth should return false for unauthenticated requests
			authenticated := config.checkAuth(req)
			assert.False(t, authenticated, "unauthenticated request to %s %s should be rejected", ep.method, ep.path)
		})
	}
}

func TestAmapEndpointsRejectMalformedSession(t *testing.T) {
	config := NewServerConfig()

	malformedTokens := []string{
		"",                    // empty
		"invalid-session-id", // non-existent session
		"<script>alert(1)</script>", // XSS attempt in session
		strings.Repeat("A", 10000), // very long session
	}

	for _, token := range malformedTokens {
		t.Run("token_"+token[:amapTestMin(len(token), 20)], func(t *testing.T) {
			req := httptest.NewRequest("GET", "/portal/api/amap-keys", nil)
			if token != "" {
				req.AddCookie(&http.Cookie{
					Name:  "admin_session",
					Value: token,
				})
			}

			authenticated := config.checkAuth(req)
			assert.False(t, authenticated, "malformed session token should be rejected")
		})
	}
}

// ==================== ID Extraction Security Tests ====================

func TestExtractAmapIDFromPath_Security(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		expectErr bool
		expectID  int
	}{
		{
			name:     "valid path with numeric ID",
			path:     "/portal/api/amap-keys/keys/123",
			expectID: 123,
		},
		{
			name:     "valid path with trailing slash",
			path:     "/portal/api/amap-keys/keys/456/",
			expectID: 456,
		},
		{
			name:      "path with non-numeric ID",
			path:      "/portal/api/amap-keys/keys/abc",
			expectErr: true,
		},
		{
			name:      "path with negative ID",
			path:      "/portal/api/amap-keys/keys/-1",
			expectErr: true,
		},
		{
			name:      "path with SQL injection attempt",
			path:      "/portal/api/amap-keys/keys/1;DROP TABLE amap_api_keys",
			expectErr: true,
		},
		{
			name:      "path with XSS attempt",
			path:      "/portal/api/amap-keys/keys/<script>alert(1)</script>",
			expectErr: true,
		},
		{
			name:      "path with very large ID (overflow attempt)",
			path:      "/portal/api/amap-keys/keys/99999999999999999999",
			expectErr: true,
		},
		{
			name:      "path without keys segment",
			path:      "/portal/api/amap-config",
			expectErr: true,
		},
		{
			name:      "path with path traversal attempt",
			path:      "/portal/api/amap-keys/keys/../../../etc/passwd",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := extractAmapIDFromPath(tt.path)
			if tt.expectErr {
				assert.Error(t, err, "should error for path: %s", tt.path)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectID, id)
			}
		})
	}
}

// ==================== Database CRUD Security Tests ====================

func TestAmapApiKeyCRUD(t *testing.T) {
	// Skip if no database
	db := GetDB()
	if db == nil {
		t.Skip("Database not initialized, skipping test")
	}

	// Ensure table exists
	err := EnsureAmapApiKeyTable()
	require.NoError(t, err)

	// Create a test key
	testKey := &schema.AmapApiKey{
		APIKey:    "test-key-12345678",
		Active:    true,
		IsHealthy: true,
	}
	err = SaveAmapApiKey(testKey)
	require.NoError(t, err)
	assert.NotZero(t, testKey.ID, "key should have an ID after save")

	// Read it back
	retrieved, err := GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.Equal(t, testKey.APIKey, retrieved.APIKey)
	assert.True(t, retrieved.Active)
	assert.True(t, retrieved.IsHealthy)

	// Update stats
	err = UpdateAmapApiKeyStats(testKey.ID, true, 100)
	require.NoError(t, err)

	retrieved, err = GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), retrieved.SuccessCount)
	assert.Equal(t, int64(100), retrieved.LastLatency)
	assert.Equal(t, int64(0), retrieved.ConsecutiveFailures)

	// Toggle status
	err = UpdateAmapApiKeyStatus(testKey.ID, false)
	require.NoError(t, err)

	retrieved, err = GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.Active)

	// Reset health
	err = ResetAmapApiKeyHealth(testKey.ID)
	require.NoError(t, err)

	retrieved, err = GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.IsHealthy)
	assert.Equal(t, "", retrieved.LastCheckError)

	// Delete
	err = DeleteAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)

	_, err = GetAmapApiKeyByID(testKey.ID)
	assert.Error(t, err, "deleted key should not be found")
}

func TestAmapApiKeyConsecutiveFailures(t *testing.T) {
	db := GetDB()
	if db == nil {
		t.Skip("Database not initialized, skipping test")
	}

	err := EnsureAmapApiKeyTable()
	require.NoError(t, err)

	testKey := &schema.AmapApiKey{
		APIKey:    "test-failure-key-abcd",
		Active:    true,
		IsHealthy: true,
	}
	err = SaveAmapApiKey(testKey)
	require.NoError(t, err)
	defer DeleteAmapApiKeyByID(testKey.ID)

	// Record 3 consecutive failures - should mark as unhealthy
	for i := 0; i < 3; i++ {
		err = UpdateAmapApiKeyStats(testKey.ID, false, 500)
		require.NoError(t, err)
	}

	retrieved, err := GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.IsHealthy, "key should be unhealthy after 3 consecutive failures")
	assert.Equal(t, int64(3), retrieved.ConsecutiveFailures)
	assert.Equal(t, int64(3), retrieved.FailureCount)
	assert.Equal(t, int64(0), retrieved.SuccessCount)

	// One success should reset consecutive failures
	err = UpdateAmapApiKeyStats(testKey.ID, true, 50)
	require.NoError(t, err)

	retrieved, err = GetAmapApiKeyByID(testKey.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.IsHealthy, "key should be healthy after a success")
	assert.Equal(t, int64(0), retrieved.ConsecutiveFailures)
	assert.Equal(t, int64(1), retrieved.SuccessCount)
}

func TestAmapConfigCRUD(t *testing.T) {
	db := GetDB()
	if db == nil {
		t.Skip("Database not initialized, skipping test")
	}

	err := EnsureAmapApiKeyTable()
	require.NoError(t, err)

	// Get default config
	config, err := GetAmapConfig()
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, uint(1), config.ID, "config should always use singleton ID 1")

	// Update config
	config.AllowFreeUserAmap = false
	err = SaveAmapConfig(config)
	require.NoError(t, err)

	// Read back
	config2, err := GetAmapConfig()
	require.NoError(t, err)
	assert.False(t, config2.AllowFreeUserAmap)

	// Restore default
	config2.AllowFreeUserAmap = true
	err = SaveAmapConfig(config2)
	require.NoError(t, err)
}

// ==================== Rate Limiter Tests ====================

func TestAmapRateLimiterBasic(t *testing.T) {
	rl := NewAmapRateLimiter()
	defer rl.Stop()

	// First request should always be allowed
	ctx := context.Background()
	err := rl.WaitForRateLimit("trace-1", ctx)
	assert.NoError(t, err, "first request should be allowed")
}

func TestAmapRateLimiterRecordsSuccess(t *testing.T) {
	rl := NewAmapRateLimiter()
	defer rl.Stop()

	// First request
	ctx := context.Background()
	err := rl.WaitForRateLimit("trace-2", ctx)
	assert.NoError(t, err)

	// Record success
	rl.RecordSuccess("trace-2")

	// Next request should need to wait (but we just check it doesn't crash)
	// The 5-second cooldown is too long to test directly in a unit test
}

// ==================== XSS Prevention in Error Output Tests ====================

func TestAmapKeyErrorOutputIsSanitized(t *testing.T) {
	// Simulate a health check error containing HTML
	maliciousError := "<script>alert('xss')</script>"
	sanitized := sanitizeForOutput(maliciousError)

	// The output should NOT contain raw HTML tags
	assert.NotContains(t, sanitized, "<script>")
	assert.NotContains(t, sanitized, "</script>")
	assert.Contains(t, sanitized, "&lt;script&gt;")
}

func TestAmapKeyMaskedOutputIsSanitized(t *testing.T) {
	// Even if someone managed to store a malicious key, the masked output should be safe
	maliciousKey := "<img src=x onerror=alert(1)>abcdefgh"
	masked := maskAPIKey(maliciousKey)
	sanitized := sanitizeForOutput(masked)

	// The output should NOT contain raw HTML tags
	assert.NotContains(t, sanitized, "<img")
	assert.NotContains(t, sanitized, "onerror")
}

// ==================== Amap Key Infocode Error Detection Tests ====================

func TestIsAmapKeyError(t *testing.T) {
	tests := []struct {
		infocode string
		isKeyErr bool
	}{
		{"10001", true},  // INVALID_USER_KEY
		{"10003", true},  // DAILY_QUERY_OVER_LIMIT
		{"10004", true},  // ACCESS_TOO_FREQUENT
		{"10005", true},  // IP_QUERY_OVER_LIMIT
		{"10044", true},  // QUOTA_PLAN_RUN_OUT
		{"10000", false}, // INVALID_PARAMS (not a key error)
		{"20000", false}, // SERVICE_NOT_EXIST
		{"", false},      // empty
		{"abc", false},   // non-numeric
	}

	for _, tt := range tests {
		t.Run("infocode_"+tt.infocode, func(t *testing.T) {
			assert.Equal(t, tt.isKeyErr, isAmapKeyError(tt.infocode))
		})
	}
}

// ==================== Helper ====================

func amapTestMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
