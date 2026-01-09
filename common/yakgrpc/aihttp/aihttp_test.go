package aihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMUSTPASS_NewAIAgentHTTPGateway(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway(
		WithRoutePrefix("/api/v1/ai"),
	)
	require.NoError(t, err)
	require.NotNil(t, gw)
	require.Equal(t, "/api/v1/ai", gw.routePrefix)
	require.False(t, gw.IsAuthEnabled())
}

func TestMUSTPASS_GatewayWithJWTAuth(t *testing.T) {
	secret := "test-secret-key"
	gw, err := NewAIAgentHTTPGateway(
		WithJWTAuth(secret),
	)
	require.NoError(t, err)
	require.True(t, gw.IsAuthEnabled())
	require.Equal(t, AuthTypeJWT, gw.GetAuthType())
}

func TestMUSTPASS_GatewayWithTOTPAuth(t *testing.T) {
	secret := "totp-secret"
	gw, err := NewAIAgentHTTPGateway(
		WithTOTP(secret),
	)
	require.NoError(t, err)
	require.True(t, gw.IsAuthEnabled())
	require.Equal(t, AuthTypeTOTP, gw.GetAuthType())
}

func TestMUSTPASS_GetSetting(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp SettingResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Setting)
}

func TestMUSTPASS_PostSetting(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	newSetting := &ypb.AIStartParams{
		AIService:    "test-service",
		ReviewPolicy: "yolo",
		ForgeName:    "test-forge",
	}

	body, _ := json.Marshal(newSetting)
	req := httptest.NewRequest(http.MethodPost, "/agent/setting", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify setting was updated
	currentSetting := gw.GetDefaultSetting()
	require.Equal(t, "test-service", currentSetting.AIService)
	require.Equal(t, "yolo", currentSetting.ReviewPolicy)
}

func TestMUSTPASS_CreateRun_MissingQuery(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	reqBody := CreateRunRequest{
		TaskID: "test-task",
		Query:  "", // Empty query
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMUSTPASS_CreateRun_MissingTaskID(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	reqBody := CreateRunRequest{
		TaskID: "", // Empty task ID
		Query:  "test query",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMUSTPASS_GetRun_NotFound(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/run/non-existent-id", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestMUSTPASS_CancelRun_NotFound(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/agent/run/non-existent-id/cancel", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestMUSTPASS_RunManager(t *testing.T) {
	rm := NewRunManager()

	// Create session
	ctx := context.Background()
	session := rm.CreateSession(ctx, "test-task")
	require.NotNil(t, session)
	require.NotEmpty(t, session.RunID)
	require.Equal(t, "test-task", session.TaskID)
	require.Equal(t, RunStatusPending, session.Status)

	// Get session
	retrieved, ok := rm.GetSession(session.RunID)
	require.True(t, ok)
	require.Equal(t, session.RunID, retrieved.RunID)

	// Cancel session
	ok = rm.CancelSession(session.RunID)
	require.True(t, ok)
	require.Equal(t, RunStatusCancelled, session.Status)
	require.NotNil(t, session.EndTime)

	// List sessions
	sessions := rm.ListSessions()
	require.Len(t, sessions, 1)

	// Delete session
	rm.DeleteSession(session.RunID)
	_, ok = rm.GetSession(session.RunID)
	require.False(t, ok)
}

func TestMUSTPASS_RunSession_Events(t *testing.T) {
	rm := NewRunManager()
	ctx := context.Background()
	session := rm.CreateSession(ctx, "test-task")

	// Add events
	event1 := &ypb.AIOutputEvent{
		Type:      "test",
		Timestamp: 100,
		EventUUID: "event-1",
	}
	event2 := &ypb.AIOutputEvent{
		Type:      "test",
		Timestamp: 200,
		EventUUID: "event-2",
	}

	session.AddEvent(event1)
	session.AddEvent(event2)

	// Get all events
	events := session.GetEvents()
	require.Len(t, events, 2)

	// Get events since timestamp
	eventsSince := session.GetEventsSince(150)
	require.Len(t, eventsSince, 1)
	require.Equal(t, "event-2", eventsSince[0].EventUUID)
}

func TestMUSTPASS_RunSession_Subscribe(t *testing.T) {
	rm := NewRunManager()
	ctx := context.Background()
	session := rm.CreateSession(ctx, "test-task")

	// Subscribe
	subscriberID := "sub-1"
	ch := session.Subscribe(subscriberID)
	require.NotNil(t, ch)

	// Add event - should be received by subscriber
	go func() {
		time.Sleep(10 * time.Millisecond)
		session.AddEvent(&ypb.AIOutputEvent{
			Type:      "test",
			EventUUID: "event-1",
		})
	}()

	select {
	case event := <-ch:
		require.Equal(t, "event-1", event.EventUUID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Unsubscribe
	session.Unsubscribe(subscriberID)
}

func TestMUSTPASS_JWTAuth_MissingHeader(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway(
		WithJWTAuth("test-secret"),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMUSTPASS_JWTAuth_InvalidFormat(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway(
		WithJWTAuth("test-secret"),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMUSTPASS_JWTAuth_ValidToken(t *testing.T) {
	secret := "test-secret"
	gw, err := NewAIAgentHTTPGateway(
		WithJWTAuth(secret),
	)
	require.NoError(t, err)

	// Generate valid token
	token, err := GenerateJWTToken(secret, nil, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMUSTPASS_TOTPAuth_MissingHeader(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway(
		WithTOTP("totp-secret"),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMUSTPASS_TOTPAuth_ValidCode(t *testing.T) {
	secret := "totp-secret"
	gw, err := NewAIAgentHTTPGateway(
		WithTOTP(secret),
	)
	require.NoError(t, err)

	// Get current TOTP code
	code := GetCurrentTOTPCode(secret)

	req := httptest.NewRequest(http.MethodGet, "/agent/setting", nil)
	req.Header.Set("X-TOTP-Code", code)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMUSTPASS_ConvertAIParamsToYPB(t *testing.T) {
	params := &AIParams{
		ForgeName:         "test-forge",
		ReviewPolicy:      "ai",
		AIService:         "openai",
		ReActMaxIteration: 10,
		ForgeParams: map[string]string{
			"key1": "value1",
		},
	}

	base := &ypb.AIStartParams{
		UseDefaultAIConfig: true,
	}

	result := ConvertAIParamsToYPB(params, base)

	require.Equal(t, "test-forge", result.ForgeName)
	require.Equal(t, "ai", result.ReviewPolicy)
	require.Equal(t, "openai", result.AIService)
	require.Equal(t, int64(10), result.ReActMaxIteration)
	require.Len(t, result.ForgeParams, 1)
}

func TestMUSTPASS_CORS(t *testing.T) {
	gw, err := NewAIAgentHTTPGateway()
	require.NoError(t, err)

	// Test OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/agent/setting", nil)
	w := httptest.NewRecorder()

	gw.GetHTTPRouteHandler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMUSTPASS_GenerateJWTToken(t *testing.T) {
	secret := "test-secret"
	claims := map[string]interface{}{
		"user_id": "123",
		"role":    "admin",
	}

	token, err := GenerateJWTToken(secret, claims, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestMUSTPASS_GetCurrentTOTPCode(t *testing.T) {
	secret := "test-totp-secret"
	code := GetCurrentTOTPCode(secret)
	require.NotEmpty(t, code)
	require.Len(t, code, 6) // TOTP codes are typically 6 digits
}
