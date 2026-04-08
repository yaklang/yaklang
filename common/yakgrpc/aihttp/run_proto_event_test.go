package aihttp

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

func newInternalTestGateway(t *testing.T, opts ...GatewayOption) *AIAgentHTTPGateway {
	t.Helper()
	defaultOpts := []GatewayOption{
		WithRoutePrefix("/agent"),
		WithHost("127.0.0.1"),
		WithPort(0),
	}
	defaultOpts = append(defaultOpts, opts...)
	gw, err := NewAIAgentHTTPGateway(defaultOpts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		gw.Shutdown()
	})
	return gw
}

func TestHandlePushEventRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-push-accepted"
	gw.runManager.Create(runID, nil)

	body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "hello",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/events/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "accepted", resp.GetType())
	require.True(t, resp.GetIsSystem())
	require.True(t, resp.GetIsResult())
	require.NotEmpty(t, resp.GetEventUUID())
	require.NotZero(t, resp.GetTimestamp())
}

func TestHandlePushEventRejectsLegacyPayload(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-push-legacy"
	gw.runManager.Create(runID, nil)

	legacyBody := []byte(`{"type":"free_input","free_input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/events/push", bytes.NewReader(legacyBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCancelRunRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-cancelled"
	gw.runManager.Create(runID, nil)

	body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&ypb.AIInputEvent{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, string(RunStatusCancelled), resp.GetType())
	require.True(t, resp.GetIsSystem())
	require.True(t, resp.GetIsResult())
	require.NotZero(t, resp.GetTimestamp())
	_, ok := gw.runManager.Get(runID)
	require.False(t, ok)
}

func TestEnsureReusableSessionRecreatesCancelledSession(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-recreate-after-cancel"

	original := gw.runManager.Create(runID, nil)
	original.Cancel()

	recreated, created, err := gw.ensureReusableSession(runID)
	require.NoError(t, err)
	require.True(t, created)
	require.NotSame(t, original, recreated)
	require.NoError(t, recreated.ctx.Err())
}

func TestHandleSSEEventsRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-sse-completed"
	session := gw.runManager.Create(runID, nil)
	session.Complete(nil)

	req := httptest.NewRequest(http.MethodGet, "/agent/run/"+runID+"/events", nil)
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var payloads []string
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payloads = append(payloads, strings.TrimPrefix(line, "data: "))
		}
	}
	require.NoError(t, scanner.Err())
	require.Len(t, payloads, 2)

	var ready ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal([]byte(payloads[0]), &ready))
	require.Equal(t, "listener_ready", ready.GetType())
	require.True(t, ready.GetIsSystem())

	var done ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal([]byte(payloads[1]), &done))
	require.Equal(t, string(RunStatusCompleted), done.GetType())
	require.True(t, done.GetIsResult())
}
