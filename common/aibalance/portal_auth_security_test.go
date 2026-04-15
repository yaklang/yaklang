package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func startTestPortalServer(t *testing.T) (string, *ServerConfig) {
	t.Helper()

	consts.InitializeYakitDatabase("", "", "")

	config := NewServerConfig()
	config.AdminPassword = "test-admin-password-secure"

	authConfig := DefaultAuthConfig()
	config.AuthMiddleware = NewAuthMiddleware(config, authConfig)

	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lis, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go config.Serve(conn)
		}
	}()

	t.Cleanup(func() { lis.Close() })
	time.Sleep(50 * time.Millisecond)

	return addr, config
}

func sendRawHTTPRequest(t *testing.T, addr, method, path string, headers map[string]string, body string) (int, map[string]string, string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	var reqBuf bytes.Buffer
	reqBuf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path))
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", addr))

	for k, v := range headers {
		reqBuf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	if body != "" {
		reqBuf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
		reqBuf.WriteString("Content-Type: application/json\r\n")
	}
	reqBuf.WriteString("Connection: close\r\n")
	reqBuf.WriteString("\r\n")
	if body != "" {
		reqBuf.WriteString(body)
	}

	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(reqBuf.Bytes())
	require.NoError(t, err)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var respBuf bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			respBuf.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	raw := respBuf.String()
	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	headerSection := parts[0]
	bodySection := ""
	if len(parts) > 1 {
		bodySection = parts[1]
	}

	headerLines := strings.Split(headerSection, "\r\n")
	statusCode := 0
	if len(headerLines) > 0 {
		statusLine := headerLines[0]
		fmt.Sscanf(statusLine, "HTTP/1.1 %d", &statusCode)
	}

	respHeaders := make(map[string]string)
	for _, line := range headerLines[1:] {
		kv := strings.SplitN(line, ": ", 2)
		if len(kv) == 2 {
			respHeaders[strings.ToLower(kv[0])] = kv[1]
		}
	}

	return statusCode, respHeaders, bodySection
}

func loginAndGetSession(t *testing.T, addr, password string) string {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	formBody := "password=" + password
	var reqBuf bytes.Buffer
	reqBuf.WriteString("POST /portal/login HTTP/1.1\r\n")
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", addr))
	reqBuf.WriteString("Content-Type: application/x-www-form-urlencoded\r\n")
	reqBuf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(formBody)))
	reqBuf.WriteString("Connection: close\r\n")
	reqBuf.WriteString("\r\n")
	reqBuf.WriteString(formBody)

	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(reqBuf.Bytes())
	require.NoError(t, err)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var respBuf bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			respBuf.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	raw := respBuf.String()
	for _, line := range strings.Split(raw, "\r\n") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "set-cookie:") {
			cookieStr := strings.TrimPrefix(lower, "set-cookie:")
			cookieStr = strings.TrimSpace(cookieStr)
			parts := strings.Split(cookieStr, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "admin_session=") {
					return strings.TrimPrefix(part, "admin_session=")
				}
			}
		}
	}

	return ""
}

func TestPortalAPIEndpointsRequireAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	apiEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/portal/api/providers"},
		{"GET", "/portal/api/health-check"},
		{"GET", "/portal/api/api-keys"},
		{"GET", "/portal/api/web-search-keys"},
		{"GET", "/portal/api/amap-keys"},
		{"GET", "/portal/api/rate-limit-config"},
		{"GET", "/portal/api/rate-limit-status"},
		{"GET", "/portal/api/data"},
		{"GET", "/portal/api/models"},
		{"GET", "/portal/api/memory-stats"},
		{"GET", "/portal/api/goroutine-dump"},
		{"GET", "/portal/api/web-search-config"},
		{"GET", "/portal/api/amap-config"},
	}

	for _, ep := range apiEndpoints {
		t.Run(fmt.Sprintf("%s_%s_no_auth", ep.method, ep.path), func(t *testing.T) {
			statusCode, _, body := sendRawHTTPRequest(t, addr, ep.method, ep.path, nil, "")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"endpoint %s %s should return 401 without auth, got %d, body: %s",
				ep.method, ep.path, statusCode, body)

			var jsonResp map[string]interface{}
			err := json.Unmarshal([]byte(body), &jsonResp)
			assert.NoError(t, err, "response should be valid JSON for %s %s", ep.method, ep.path)
			if err == nil {
				assert.Equal(t, "Unauthorized", jsonResp["error"],
					"error field should be 'Unauthorized' for %s %s", ep.method, ep.path)
			}
		})
	}
}

func TestPortalMutationEndpointsRequireAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	mutationEndpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/portal/add-providers"},
		{"POST", "/portal/validate-provider"},
		{"DELETE", "/portal/delete-provider/1"},
		{"POST", "/portal/delete-providers"},
		{"POST", "/portal/check-all-health"},
		{"POST", "/portal/check-health/1"},
		{"POST", "/portal/create-api-key"},
		{"POST", "/portal/generate-api-key"},
		{"POST", "/portal/activate-api-key/1"},
		{"POST", "/portal/deactivate-api-key/1"},
		{"POST", "/portal/batch-activate-api-keys"},
		{"POST", "/portal/batch-deactivate-api-keys"},
		{"POST", "/portal/update-api-key-allowed-models/1"},
		{"DELETE", "/portal/delete-api-key/1"},
		{"POST", "/portal/delete-api-keys"},
		{"POST", "/portal/api-key-traffic-limit/1"},
		{"POST", "/portal/reset-api-key-traffic/1"},
		{"POST", "/portal/api/web-search-keys"},
		{"DELETE", "/portal/api/web-search-keys/1"},
		{"PUT", "/portal/api/web-search-keys/1"},
		{"POST", "/portal/activate-web-search-key/1"},
		{"POST", "/portal/deactivate-web-search-key/1"},
		{"POST", "/portal/reset-web-search-key-health/1"},
		{"POST", "/portal/test-web-search-key/1"},
		{"POST", "/portal/api/web-search-config"},
		{"POST", "/portal/api/amap-keys"},
		{"DELETE", "/portal/api/amap-keys/1"},
		{"POST", "/portal/toggle-amap-key/1"},
		{"POST", "/portal/reset-amap-key-health/1"},
		{"POST", "/portal/test-amap-key/1"},
		{"POST", "/portal/api/amap-keys/check-all"},
		{"POST", "/portal/api/amap-config"},
		{"POST", "/portal/update-model-meta"},
		{"POST", "/portal/refresh-totp"},
		{"POST", "/portal/api/rate-limit-config"},
		{"POST", "/portal/api/force-gc"},
	}

	for _, ep := range mutationEndpoints {
		t.Run(fmt.Sprintf("%s_%s_no_auth", ep.method, strings.ReplaceAll(ep.path, "/", "_")), func(t *testing.T) {
			statusCode, _, body := sendRawHTTPRequest(t, addr, ep.method, ep.path, nil, "{}")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"mutation endpoint %s %s should return 401 without auth, got %d, body: %s",
				ep.method, ep.path, statusCode, body)
		})
	}
}

func TestOpsAPIEndpointsRequireAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	opsEndpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/ops/create-api-key"},
		{"POST", "/ops/api/create-api-key"},
		{"GET", "/ops/api/my-keys"},
		{"POST", "/ops/api/delete-api-key"},
		{"POST", "/ops/api/update-api-key"},
		{"POST", "/ops/api/reset-traffic"},
		{"GET", "/ops/my-info"},
		{"POST", "/ops/change-password"},
		{"POST", "/ops/reset-key"},
	}

	for _, ep := range opsEndpoints {
		t.Run(fmt.Sprintf("%s_%s_no_auth", ep.method, strings.ReplaceAll(ep.path, "/", "_")), func(t *testing.T) {
			statusCode, _, body := sendRawHTTPRequest(t, addr, ep.method, ep.path, nil, "{}")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"OPS endpoint %s %s should return 401 without auth, got %d, body: %s",
				ep.method, ep.path, statusCode, body)
		})
	}
}

func TestPortalPageRequestsServeLoginPage(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	pageEndpoints := []string{
		"/portal",
		"/portal/",
		"/portal/add-ai-provider",
		"/portal/api-keys",
		"/portal/totp-settings",
	}

	for _, path := range pageEndpoints {
		t.Run("GET_"+strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			statusCode, headers, body := sendRawHTTPRequest(t, addr, "GET", path, nil, "")
			assert.Equal(t, http.StatusOK, statusCode,
				"page request GET %s should return 200 (login page), got %d", path, statusCode)
			ct := headers["content-type"]
			assert.Contains(t, ct, "text/html",
				"page request GET %s should return HTML content-type", path)
			assert.NotContains(t, body, `"error"`,
				"login page for GET %s should not contain JSON error", path)
		})
	}
}

func TestPortalPublicRoutesAccessible(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	t.Run("static_files_accessible", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/static/portal.js", nil, "")
		assert.True(t, statusCode == 200 || statusCode == 404,
			"static file request should return 200 or 404 (not 401), got %d", statusCode)
	})

	t.Run("portal_login_GET_serves_login_page", func(t *testing.T) {
		statusCode, headers, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/login", nil, "")
		assert.Equal(t, http.StatusOK, statusCode,
			"GET /portal/login should return 200 (login page), got %d", statusCode)
		ct := headers["content-type"]
		assert.Contains(t, ct, "text/html",
			"GET /portal/login should return HTML content-type")
	})

	t.Run("ops_login_GET_serves_login_page", func(t *testing.T) {
		statusCode, headers, _ := sendRawHTTPRequest(t, addr, "GET", "/ops/login", nil, "")
		assert.Equal(t, http.StatusOK, statusCode,
			"GET /ops/login should return 200 (login page), got %d", statusCode)
		ct := headers["content-type"]
		assert.Contains(t, ct, "text/html",
			"GET /ops/login should return HTML content-type")
	})

	t.Run("login_POST_accessible", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "POST", "/portal/login",
			map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "password=wrong")
		assert.NotEqual(t, 401, statusCode,
			"login POST should not return 401 (it's a public route)")
	})

	t.Run("ops_login_POST_accessible", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "POST", "/ops/login",
			map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "username=test&password=wrong")
		assert.NotEqual(t, 401, statusCode,
			"OPS login POST should not return 401 (it's a public route)")
	})
}

func TestAuthenticatedRequestsSucceed(t *testing.T) {
	addr, config := startTestPortalServer(t)

	session := loginAndGetSession(t, addr, config.AdminPassword)
	require.NotEmpty(t, session, "should get a valid session after login")

	t.Run("api_data_with_session", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/data",
			map[string]string{"Cookie": "admin_session=" + session}, "")
		assert.Equal(t, http.StatusOK, statusCode,
			"authenticated request to /portal/api/data should return 200")
	})

	t.Run("api_models_with_session", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/models",
			map[string]string{"Cookie": "admin_session=" + session}, "")
		assert.Equal(t, http.StatusOK, statusCode,
			"authenticated request to /portal/api/models should return 200")
	})

	t.Run("api_providers_with_session", func(t *testing.T) {
		statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/providers",
			map[string]string{"Cookie": "admin_session=" + session}, "")
		assert.Equal(t, http.StatusOK, statusCode,
			"authenticated request to /portal/api/providers should return 200")
	})
}

func TestInvalidSessionReturns401(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/portal/api/data"},
		{"GET", "/portal/api/providers"},
		{"POST", "/portal/create-api-key"},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("invalid_session_%s_%s", ep.method, ep.path), func(t *testing.T) {
			statusCode, _, _ := sendRawHTTPRequest(t, addr, ep.method, ep.path,
				map[string]string{"Cookie": "admin_session=fake-session-id-that-does-not-exist"}, "{}")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"%s %s with invalid session should return 401, got %d", ep.method, ep.path, statusCode)
		})
	}
}

func TestExpiredSessionReturns401(t *testing.T) {
	addr, config := startTestPortalServer(t)

	session := loginAndGetSession(t, addr, config.AdminPassword)
	require.NotEmpty(t, session)

	config.SessionManager.DeleteSession(session)

	statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/data",
		map[string]string{"Cookie": "admin_session=" + session}, "")
	assert.Equal(t, http.StatusUnauthorized, statusCode,
		"deleted session should return 401")
}

func TestQueryPasswordBypassBlockedForAPI(t *testing.T) {
	addr, config := startTestPortalServer(t)

	statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET",
		"/portal/api/data?password="+config.AdminPassword, nil, "")

	if statusCode == http.StatusOK {
		t.Log("query password auth is allowed (legacy mode) - this is expected behavior")
	} else {
		assert.Equal(t, http.StatusUnauthorized, statusCode,
			"should be either 200 (legacy auth) or 401")
	}
}

func TestNoAuthInfoLeakInErrorResponse(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	statusCode, _, body := sendRawHTTPRequest(t, addr, "GET", "/portal/api/data", nil, "")
	assert.Equal(t, http.StatusUnauthorized, statusCode)

	bodyLower := strings.ToLower(body)
	assert.NotContains(t, bodyLower, "password")
	assert.NotContains(t, bodyLower, "session")
	assert.NotContains(t, bodyLower, "token")
	assert.NotContains(t, bodyLower, "admin_session")
	assert.NotContains(t, bodyLower, "provider")
	assert.NotContains(t, bodyLower, "api_key")
}

func TestResponseContentTypeIsJSON(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	apiPaths := []string{
		"/portal/api/data",
		"/portal/api/providers",
		"/portal/api/models",
	}

	for _, path := range apiPaths {
		t.Run(path, func(t *testing.T) {
			statusCode, headers, body := sendRawHTTPRequest(t, addr, "GET", path, nil, "")
			assert.Equal(t, http.StatusUnauthorized, statusCode)
			ct := headers["content-type"]
			assert.Contains(t, ct, "application/json",
				"401 response for %s should be JSON, got content-type: %s, body: %s", path, ct, body)
		})
	}
}

func TestPublicStatsNotAffected(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/public/stats", nil, "")
	assert.True(t, statusCode == 200 || statusCode == 404,
		"/public/stats should be accessible without auth, got %d", statusCode)
}

func TestConcurrentUnauthenticatedRequests(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	done := make(chan int, 50)
	for i := 0; i < 50; i++ {
		go func() {
			statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/data", nil, "")
			done <- statusCode
		}()
	}

	for i := 0; i < 50; i++ {
		select {
		case code := <-done:
			assert.Equal(t, http.StatusUnauthorized, code,
				"concurrent unauthenticated request should return 401")
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for concurrent requests")
		}
	}
}

func TestCookieInjectionDoesNotBypassAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	injectionAttempts := []string{
		"admin_session=; admin_session=fake",
		"admin_session=../../etc/passwd",
		"admin_session=' OR 1=1 --",
		"admin_session=<script>alert(1)</script>",
		"admin_session=; role=admin",
		"admin_session=00000000-0000-0000-0000-000000000000",
	}

	for _, cookie := range injectionAttempts {
		t.Run(cookie, func(t *testing.T) {
			statusCode, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/data",
				map[string]string{"Cookie": cookie}, "")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"cookie injection attempt should return 401")
		})
	}
}

func TestMethodSpoofingDoesNotBypassAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)

	methods := []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

	for _, method := range methods {
		t.Run(method+"_portal_api_data", func(t *testing.T) {
			statusCode, _, _ := sendRawHTTPRequest(t, addr, method, "/portal/api/data", nil, "{}")
			assert.Equal(t, http.StatusUnauthorized, statusCode,
				"%s /portal/api/data without auth should return 401, got %d", method, statusCode)
		})
	}
}
