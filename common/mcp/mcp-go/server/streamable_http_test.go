package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func initializeRequestBody(t *testing.T) []byte {
	t.Helper()

	requestBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	return requestBody
}

func doInitializeRequest(t *testing.T, testServer *httptest.Server, contentType string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPost,
		testServer.URL+DefaultStreamableHTTPPath,
		bytes.NewBuffer(initializeRequestBody(t)),
	)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestStreamableHTTPServerInitialize(t *testing.T) {
	mcpServer := NewMCPServer("test-server", "1.0.0")
	testServer := NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

	resp := doInitializeRequest(t, testServer, "application/json")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get(mcp.HeaderSessionID))
	require.Equal(
		t,
		mcp.LATEST_PROTOCOL_VERSION,
		resp.Header.Get(mcp.HeaderProtocolVersion),
	)
	require.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
	require.Equal(t, "2.0", response["jsonrpc"])
	require.EqualValues(t, 1, response["id"])
}

func TestStreamableHTTPServerInitializeAcceptsJSONCharset(t *testing.T) {
	mcpServer := NewMCPServer("test-server", "1.0.0")
	testServer := NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

	resp := doInitializeRequest(t, testServer, "application/json; charset=utf-8")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get(mcp.HeaderSessionID))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestStreamableHTTPServerInitializeRejectsNonJSONContentType(t *testing.T) {
	mcpServer := NewMCPServer("test-server", "1.0.0")
	testServer := NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

	resp := doInitializeRequest(t, testServer, "text/plain")
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, resp.Header.Get(mcp.HeaderSessionID))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Content-Type must be application/json")
}

func TestStreamableHTTPServerInitializeRejectsMissingContentType(t *testing.T) {
	mcpServer := NewMCPServer("test-server", "1.0.0")
	testServer := NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

	resp := doInitializeRequest(t, testServer, "")
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, resp.Header.Get(mcp.HeaderSessionID))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Content-Type must be application/json")
}
