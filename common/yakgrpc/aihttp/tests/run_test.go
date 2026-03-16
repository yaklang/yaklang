package aihttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGetRunNotFound(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("GET", "/agent/run/nonexistent-id", nil)
	w := performRequest(gw, req)
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestRunNotFound(t *testing.T) {
	gw := newTestGateway(t)

	body, _ := json.Marshal(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "hello"})
	req := httptest.NewRequest("POST", "/agent/run/nonexistent-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPushEventNotFound(t *testing.T) {
	gw := newTestGateway(t)

	body, _ := json.Marshal(&ypb.AIInputEvent{IsInteractiveMessage: true})
	req := httptest.NewRequest("POST", "/agent/run/no-such-id/events/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
