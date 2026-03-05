package aihttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

func TestCreateSessionValidation(t *testing.T) {
	gw := newTestGateway(t)

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader([]byte(`{`)))
		req.Header.Set("Content-Type", "application/json")
		w := performRequest(gw, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestCreateSession(t *testing.T) {
	gw := newTestGateway(t)

	body, _ := json.Marshal(aihttp.CreateSessionRequest{})
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp aihttp.CreateSessionResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotEmpty(t, resp.RunID)
}

func TestListAllSessions(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest("GET", "/agent/session/all", nil)
	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)
}
