//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func testBrowserTransformExternalAdapter(
	t *testing.T,
	requestEnabled bool,
	responseEnabled bool,
	execute func(browserTransformCall) (browserTransformResult, error),
) *browserTransformExternalAdapter {
	t.Helper()
	caller := &fakeBrowserTransformCaller{
		profileID:  "profile-adapter",
		requestOn:  requestEnabled,
		responseOn: responseEnabled,
		origin:     "https://example.test",
		methods:    []string{"POST"},
		urlPattern: "https://example.test/api/*",
		execute:    execute,
	}
	runtime, err := prepareBrowserTransform(
		context.Background(),
		caller,
		"browser-adapter",
		"profile-adapter",
		5*time.Second,
	)
	require.NoError(t, err)
	adapter := &browserTransformExternalAdapter{
		runtime:     runtime,
		token:       "adapter-test-token",
		host:        "127.0.0.1",
		port:        45678,
		endpoint:    "http://127.0.0.1:45678",
		timeout:     5 * time.Second,
		startedAt:   time.Now(),
		concurrency: make(chan struct{}, browserTransformAdapterMaxConcurrency),
	}
	adapter.running.Store(true)
	return adapter
}

func browserTransformAdapterRequest(
	t *testing.T,
	adapter *browserTransformExternalAdapter,
	payload map[string]interface{},
	token string,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/v1/transform", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	adapter.secureHeaders(http.HandlerFunc(adapter.handleTransform)).ServeHTTP(response, request)
	return response
}

func TestBrowserTransformAdapterRequestUsesPinnedProfile(t *testing.T) {
	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		require.Equal(t, "profile-adapter", input.ProfileID)
		require.Equal(t, "request", input.Direction)
		require.Equal(t, "https://example.test/api/login", input.Packet.URL)
		require.JSONEq(t, "{\"password\":\"plain\"}", string(mustDecodeBase64(t, input.Packet.BodyBase64)))
		return browserTransformResult{
			ProfileID:  input.ProfileID,
			Direction:  input.Direction,
			URL:        input.Packet.URL,
			BodyBase64: encodedTransformBody("{\"password\":\"cipher\"}"),
		}, nil
	})
	plain := []byte("POST /api/login HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"password\":\"plain\"}")
	response := browserTransformAdapterRequest(t, adapter, map[string]interface{}{
		"version":      1,
		"direction":    "request",
		"https":        true,
		"packetBase64": base64.StdEncoding.EncodeToString(plain),
	}, adapter.token)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var result browserTransformAdapterResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, uint64(1), result.Sequence)
	require.True(t, result.Applied)
	require.Equal(t, "profile-adapter", result.ProfileID)
	wire, err := base64.StdEncoding.DecodeString(result.PacketBase64)
	require.NoError(t, err)
	require.JSONEq(t, "{\"password\":\"cipher\"}", string(lowhttp.GetHTTPPacketBody(wire)))
	require.Equal(t, uint64(1), adapter.requestCount.Load())
	require.Zero(t, adapter.failureCount.Load())
	require.Equal(t, []string{"POST"}, adapter.status(true).GetMethods())
	require.Equal(t, "https://example.test/api/*", adapter.status(true).GetUrlPattern())
}

func TestBrowserTransformAdapterRejectsUnauthorizedAndBrowserOrigin(t *testing.T) {
	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		return browserTransformResult{}, errors.New("must not execute")
	})
	packet := base64.StdEncoding.EncodeToString([]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"))
	payload := map[string]interface{}{
		"version": 1, "direction": "request", "packetBase64": packet,
	}
	require.Equal(t, http.StatusUnauthorized, browserTransformAdapterRequest(t, adapter, payload, "wrong").Code)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/v1/transform", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+adapter.token)
	request.Header.Set("Origin", "https://example.test")
	response := httptest.NewRecorder()
	adapter.handleTransform(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Zero(t, adapter.requestCount.Load())
}

func TestBrowserTransformAdapterResponseRequiresWireRequest(t *testing.T) {
	adapter := testBrowserTransformExternalAdapter(t, false, true, func(input browserTransformCall) (browserTransformResult, error) {
		return browserTransformResult{}, errors.New("must not execute")
	})
	responsePacket := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"payload\":\"cipher\"}")
	response := browserTransformAdapterRequest(t, adapter, map[string]interface{}{
		"version":      1,
		"direction":    "response",
		"https":        true,
		"packetBase64": base64.StdEncoding.EncodeToString(responsePacket),
	}, adapter.token)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "requestBase64 is required")
	require.Zero(t, adapter.requestCount.Load())
}

func TestBrowserTransformAdapterFailsClosedOnBrowserError(t *testing.T) {
	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		return browserTransformResult{}, errors.New("document is stale")
	})
	plain := []byte("POST /api/login HTTP/1.1\r\nHost: example.test\r\n\r\nplain-secret")
	response := browserTransformAdapterRequest(t, adapter, map[string]interface{}{
		"version":      1,
		"direction":    "request",
		"https":        true,
		"packetBase64": base64.StdEncoding.EncodeToString(plain),
	}, adapter.token)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.NotContains(t, response.Body.String(), "plain-secret")
	require.Contains(t, response.Body.String(), "document is stale")
	require.Equal(t, uint64(1), adapter.requestCount.Load())
	require.Equal(t, uint64(1), adapter.failureCount.Load())
}

func TestBrowserTransformAdapterBypassesRoutesOutsidePinnedProfile(t *testing.T) {
	called := false
	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		called = true
		return browserTransformResult{}, errors.New("must not execute")
	})
	adapter.runtime.urlPattern = "/api/*"
	plain := []byte("POST /api/login HTTP/1.1\r\nHost: other.test\r\n\r\nplain-secret")
	response := browserTransformAdapterRequest(t, adapter, map[string]interface{}{
		"version":      1,
		"direction":    "request",
		"https":        true,
		"packetBase64": base64.StdEncoding.EncodeToString(plain),
	}, adapter.token)
	require.Equal(t, http.StatusOK, response.Code)
	var result browserTransformAdapterResult
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.False(t, result.Applied)
	require.Equal(t, "route_mismatch", result.BypassReason)
	require.Equal(t, base64.StdEncoding.EncodeToString(plain), result.PacketBase64)
	require.False(t, called)
	require.Zero(t, adapter.requestCount.Load())
	require.Equal(t, uint64(1), adapter.bypassCount.Load())
	require.Equal(t, "https://example.test", adapter.status(false).GetOrigin())
	require.Equal(t, uint64(1), adapter.status(false).GetBypassCount())
}

func TestBrowserTransformAdapterExplicitPatternMayCrossProfileOrigin(t *testing.T) {
	called := false
	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		called = true
		return browserTransformResult{
			ProfileID:  input.ProfileID,
			Direction:  input.Direction,
			URL:        input.Packet.URL,
			BodyBase64: encodedTransformBody("wire"),
		}, nil
	})
	adapter.runtime.urlPattern = "https://other.test/api/*"
	plain := []byte("POST /api/login HTTP/1.1\r\nHost: other.test\r\n\r\nplain")
	response := browserTransformAdapterRequest(t, adapter, map[string]interface{}{
		"version":      1,
		"direction":    "request",
		"https":        true,
		"packetBase64": base64.StdEncoding.EncodeToString(plain),
	}, adapter.token)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, called)
}

func TestBrowserTransformAdapterStrictPayloadAndLoopback(t *testing.T) {
	_, err := normalizeBrowserTransformAdapterHost("0.0.0.0")
	require.EqualError(t, err, "browser transform adapter must listen on a loopback address")
	require.Equal(t, "127.0.0.1", requireHost(t, "localhost"))
	require.Equal(t, "::1", requireHost(t, "[::1]"))
	require.Equal(t, 10*time.Second, browserTransformAdapterTimeout(0))
	require.Equal(t, 2*time.Second, browserTransformAdapterTimeout(500))
	require.Equal(t, 60*time.Second, browserTransformAdapterTimeout(120_000))

	adapter := testBrowserTransformExternalAdapter(t, true, false, func(input browserTransformCall) (browserTransformResult, error) {
		return browserTransformResult{}, errors.New("must not execute")
	})
	raw := "{\"version\":1,\"direction\":\"request\",\"packetBase64\":\"" +
		base64.StdEncoding.EncodeToString([]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")) +
		"\",\"profileId\":\"attacker-selected\"}"
	request := httptest.NewRequest(http.MethodPost, "/v1/transform", strings.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+adapter.token)
	response := httptest.NewRecorder()
	adapter.handleTransform(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "unknown field")

	oversized := httptest.NewRequest(http.MethodPost, "/v1/transform", strings.NewReader("{}"))
	oversized.Header.Set("Content-Type", "application/json")
	oversized.Header.Set("Authorization", "Bearer "+adapter.token)
	oversized.ContentLength = browserTransformAdapterMaxPayloadBytes + 1
	oversizedResponse := httptest.NewRecorder()
	adapter.handleTransform(oversizedResponse, oversized)
	require.Equal(t, http.StatusRequestEntityTooLarge, oversizedResponse.Code)
	require.Contains(t, oversizedResponse.Body.String(), "payload_too_large")
}

func requireHost(t *testing.T, raw string) string {
	t.Helper()
	host, err := normalizeBrowserTransformAdapterHost(raw)
	require.NoError(t, err)
	return host
}
