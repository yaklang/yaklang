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

func TestGetSetting(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest("GET", "/agent/setting", nil)
	w := performRequest(gw, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotEmpty(t, resp["ReviewPolicy"])
}

func TestListAIProviders(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("POST", "/agent/setting/providers/get", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.ListAIProvidersResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
}

func TestQueryAIFocus(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("POST", "/agent/setting/aifocus/get", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.QueryAIFocusResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Data)
}

func TestQueryAIProviders(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("POST", "/agent/setting/providers/query", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.QueryAIProvidersResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
}
