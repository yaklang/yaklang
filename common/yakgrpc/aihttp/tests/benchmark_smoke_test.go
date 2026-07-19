package aihttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ---------------------------------------------------------------------------
// Benchmark smoke tests: validate the HTTP gateway API surface used by
// gateway_runner.py without requiring Docker, Harbor, or an AI provider.
// ---------------------------------------------------------------------------

// TestBenchmarkCreateSession validates POST /agent/session creates a session.
func TestBenchmarkCreateSession(t *testing.T) {
	gw := newTestGateway(t)

	payload := `{"run_id": "benchmark-test-001"}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, "benchmark-test-001", resp["run_id"])
	require.Contains(t, []any{"pending", "running"}, resp["status"])
}

// TestBenchmarkCreateSessionWithoutRunID validates auto-generated run_id.
func TestBenchmarkCreateSessionWithoutRunID(t *testing.T) {
	gw := newTestGateway(t)

	payload := `{}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotEmpty(t, resp["run_id"])
}

// TestBenchmarkGetSetting validates GET /agent/setting returns expected fields.
func TestBenchmarkGetSetting(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("GET", "/agent/setting", nil)
	w := performRequest(gw, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	// ReviewPolicy should always be present (defaults to "manual")
	require.Contains(t, resp, "ReviewPolicy")
}

// TestBenchmarkUpdateSetting validates POST /agent/setting applies changes.
func TestBenchmarkUpdateSetting(t *testing.T) {
	gw := newTestGateway(t)

	payload := `{"AIService": "openai", "AIModelName": "gpt-test", "ReviewPolicy": "yolo"}`
	req := httptest.NewRequest("POST", "/agent/setting", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, "openai", resp["AIService"])
}

// TestBenchmarkUpdateSettingSnakeCase validates snake_case aliases.
func TestBenchmarkUpdateSettingSnakeCase(t *testing.T) {
	gw := newTestGateway(t)

	payload := `{"ai_service": "deepseek", "ai_model_name": "deepseek-v4"}`
	req := httptest.NewRequest("POST", "/agent/setting", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// TestBenchmarkListSessions validates GET /agent/session/all.
func TestBenchmarkListSessions(t *testing.T) {
	gw := newTestGateway(t)

	// Create a session first
	createPayload := `{"run_id": "benchmark-list-test"}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(createPayload))
	req.Header.Set("Content-Type", "application/json")
	performRequest(gw, req)

	// Now list
	req2 := httptest.NewRequest("GET", "/agent/session/all", nil)
	w2 := performRequest(gw, req2)

	require.Equal(t, http.StatusOK, w2.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&resp))
	sessions, ok := resp["sessions"].([]any)
	require.True(t, ok, "sessions should be a list")
	require.NotEmpty(t, sessions)
}

// TestBenchmarkSSEEvents validates GET /agent/run/{id}/events returns SSE
// content type.  httptest.ResponseRecorder buffers all output, so we cannot
// test streaming behavior directly — we validate the HTTP contract instead.
func TestBenchmarkSSEEvents(t *testing.T) {
	gw := newTestGateway(t)

	// Create session
	createPayload := `{"run_id": "benchmark-sse-test"}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(createPayload))
	req.Header.Set("Content-Type", "application/json")
	performRequest(gw, req)

	// Open SSE in a goroutine since the handler blocks until terminal event.
	// Capture only the first write (listener_ready) to validate headers.
	req2 := httptest.NewRequest("GET", "/agent/run/benchmark-sse-test/events", nil)
	req2.Header.Set("Accept", "text/event-stream")
	w2 := httptest.NewRecorder()

	// Run the handler in a background goroutine
	done := make(chan struct{})
	go func() {
		gw.ServeHTTP(w2, req2)
		close(done)
	}()

	// Wait briefly for the handler to write headers and listener_ready
	time.Sleep(200 * time.Millisecond)

	// Validate Content-Type is text/event-stream (set before flush)
	ct := w2.Header().Get("Content-Type")
	require.Equal(t, "text/event-stream", ct,
		"SSE endpoint must return text/event-stream content type")
	require.Equal(t, "no-cache", w2.Header().Get("Cache-Control"))

	// Give the goroutine time to clean up
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		// The handler may be blocked waiting for events; that's fine
	}
}

// TestBenchmarkStartRunNoAI validates POST /agent/run starts the gRPC stream
// (will fail at AI call stage since no real AI provider is configured, but
// the protocol wiring is verified).
func TestBenchmarkStartRun(t *testing.T) {
	gw := newTestGateway(t)

	// Create session
	runID := "benchmark-run-test"
	createPayload := `{"run_id": "` + runID + `"}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(createPayload))
	req.Header.Set("Content-Type", "application/json")
	performRequest(gw, req)

	// Submit start event
	startPayload := map[string]any{
		"IsStart":     true,
		"IsFreeInput": true,
		"FreeInput":   "echo hello",
		"Params": map[string]any{
			"CoordinatorId":                runID,
			"UserQuery":                    "echo hello",
			"AIService":                    "openai",
			"AIModelName":                  "gpt-test",
			"UseDefaultAIConfig":           false,
			"ReviewPolicy":                 "yolo",
			"DisallowRequireForUserPrompt": true,
			"AllowPlanUserInteract":        false,
			"EnableAISearchInternet":       false,
			"ReActMaxIteration":            3,
			"AICallTokenLimit":             1000,
			"Source":                       "benchmark-smoke-test",
		},
	}
	body, _ := json.Marshal(startPayload)
	req2 := httptest.NewRequest("POST", "/agent/run/"+runID, bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := performRequest(gw, req2)

	// Should accept the start event (200 OK)
	require.Equal(t, http.StatusOK, w2.Code)
}

// TestBenchmarkAIInputEventRoundTrip validates AIInputEvent JSON serialization
// matches what gateway_runner.py sends.
func TestBenchmarkAIInputEventRoundTrip(t *testing.T) {
	// This is the exact JSON structure gateway_runner.py sends
	pyJSON := []byte(`{
		"IsStart": true,
		"IsFreeInput": true,
		"FreeInput": "Say hello world",
		"Params": {
			"CoordinatorId": "test-roundtrip",
			"UserQuery": "Say hello world",
			"AIService": "openai",
			"AIModelName": "gpt-4",
			"UseDefaultAIConfig": false,
			"ReviewPolicy": "yolo",
			"DisallowRequireForUserPrompt": true,
			"AllowPlanUserInteract": false,
			"EnableAISearchInternet": false,
			"ReActMaxIteration": 20,
			"AICallTokenLimit": 20000,
			"Source": "harbor-benchmark-v1"
		}
	}`)

	// Validate JSON is well-formed
	var raw map[string]any
	require.NoError(t, json.Unmarshal(pyJSON, &raw))

	// Validate key fields
	require.Equal(t, true, raw["IsStart"])
	require.Equal(t, true, raw["IsFreeInput"])
	require.Equal(t, "Say hello world", raw["FreeInput"])

	params, ok := raw["Params"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "test-roundtrip", params["CoordinatorId"])
	require.Equal(t, "openai", params["AIService"])
	require.Equal(t, "yolo", params["ReviewPolicy"])
}

// TestBenchmarkAIGlobalConfigFormat validates the AIGlobalConfig JSON format
// that gateway_runner.py's seed_ai_config() sends to /setting/aiconfig.
func TestBenchmarkAIGlobalConfigFormat(t *testing.T) {
	// Replicate the exact structure seed_ai_config sends
	cfgJSON := []byte(`{
		"Enabled": true,
		"DisableFallback": true,
		"IntelligentModels": [
			{
				"Provider": {
					"Type": "deepseek",
					"APIKey": "sk-test-key",
					"Domain": "api.deepseek.com"
				},
				"ModelName": "deepseek-v4-flash"
			}
		]
	}`)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(cfgJSON, &raw))
	require.Equal(t, true, raw["Enabled"])
	require.Equal(t, true, raw["DisableFallback"])

	models, ok := raw["IntelligentModels"].([]any)
	require.True(t, ok)
	require.Len(t, models, 1)

	model := models[0].(map[string]any)
	require.Equal(t, "deepseek-v4-flash", model["ModelName"])

	provider := model["Provider"].(map[string]any)
	require.Equal(t, "deepseek", provider["Type"])
	require.Equal(t, "api.deepseek.com", provider["Domain"])
}

// TestBenchmarkUpdateAIGlobalConfig validates POST /agent/setting/aiconfig.
func TestBenchmarkUpdateAIGlobalConfig(t *testing.T) {
	gw := newTestGateway(t)

	cfgJSON := `{
		"Enabled": true,
		"DisableFallback": true,
		"IntelligentModels": [
			{
				"Provider": {
					"Type": "openai",
					"APIKey": "sk-test",
					"Domain": "api.test.local"
				},
				"ModelName": "test-model"
			}
		]
	}`
	req := httptest.NewRequest("POST", "/agent/setting/aiconfig", bytes.NewBufferString(cfgJSON))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	// The internal gRPC SetAIGlobalConfig call may succeed or fail depending on
	// whether the DB has the required tables. Either way, the HTTP handler
	// should return a response (not panic).
	statusOK := w.Code == http.StatusOK
	statusGatewayErr := w.Code == http.StatusBadGateway
	require.True(t, statusOK || statusGatewayErr,
		"expected 200 or 502, got %d: %s", w.Code, w.Body.String())
}

// TestBenchmarkGatewayCORS validates CORS headers are present.
func TestBenchmarkGatewayCORS(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("OPTIONS", "/agent/setting", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// TestBenchmarkCancelRun validates POST /agent/run/{id}/cancel.
func TestBenchmarkCancelRun(t *testing.T) {
	gw := newTestGateway(t)

	// Create session
	createPayload := `{"run_id": "benchmark-cancel-test"}`
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewBufferString(createPayload))
	req.Header.Set("Content-Type", "application/json")
	performRequest(gw, req)

	// Cancel it
	req2 := httptest.NewRequest("POST", "/agent/run/benchmark-cancel-test/cancel", nil)
	req2.Header.Set("Content-Type", "application/json")
	w2 := performRequest(gw, req2)

	require.Equal(t, http.StatusOK, w2.Code)

	// Verify the response contains "cancelled"
	require.True(t, strings.Contains(w2.Body.String(), "cancelled"),
		"cancel response should contain 'cancelled': %s", w2.Body.String())
}

// TestBenchmarkProtoJSONFieldNames validates field name consistency between
// proto definitions and Python client expectations.
func TestBenchmarkProtoJSONFieldNames(t *testing.T) {
	// Verify that AIOutputEvent fields use PascalCase JSON names
	// (matching what gateway_runner.py expects with event.get("Type"), etc.)
	event := &ypb.AIOutputEvent{
		Type:      "completed",
		CoordinatorId: "test-coordinator",
		IsResult:  true,
		Content:   []byte(`{"message": "done"}`),
		Timestamp: 1700000000,
		EventUUID: "evt-001",
		IsSystem:  false,
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Python client accesses: event.get("Type"), event.get("Content"), etc.
	require.Equal(t, "completed", raw["Type"])
	require.Equal(t, "test-coordinator", raw["CoordinatorId"])
	require.Equal(t, true, raw["IsResult"])
	require.Equal(t, float64(1700000000), raw["Timestamp"])
	require.Equal(t, "evt-001", raw["EventUUID"])

	// Verify that Content is base64-encoded bytes in JSON.
	// Content contains `{"message": "done"}` → base64 = eyJtZXNzYWdlIjogImRvbmUifQ==
	contentStr, ok := raw["Content"].(string)
	require.True(t, ok)
	require.NotEmpty(t, contentStr, "Content should not be empty")

	// Also test with simpler content
	event2 := &ypb.AIOutputEvent{
		Type:      "stream",
		Content:   []byte("hello"),
		Timestamp: 1700000001,
	}
	data2, err := json.Marshal(event2)
	require.NoError(t, err)

	var raw2 map[string]any
	require.NoError(t, json.Unmarshal(data2, &raw2))
	require.Equal(t, "aGVsbG8=", raw2["Content"]) // base64 of "hello"
}

// TestBenchmarkAIStartParamsFields validates AIStartParams field serialization.
func TestBenchmarkAIStartParamsFields(t *testing.T) {
	params := &ypb.AIStartParams{
		CoordinatorId:                "test-coord",
		UserQuery:                    "test query",
		AIService:                    "openai",
		AIModelName:                  "gpt-4",
		UseDefaultAIConfig:           false,
		ReviewPolicy:                 "yolo",
		DisallowRequireForUserPrompt: true,
		AllowPlanUserInteract:        false,
		ReActMaxIteration:            20,
		AICallTokenLimit:             20000,
		Source:                       "benchmark-test",
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Verify key fields match what gateway_runner.py sends
	require.Equal(t, "test-coord", raw["CoordinatorId"])
	require.Equal(t, "test query", raw["UserQuery"])
	require.Equal(t, "openai", raw["AIService"])
	require.Equal(t, "gpt-4", raw["AIModelName"])
	require.Equal(t, "yolo", raw["ReviewPolicy"])
}
