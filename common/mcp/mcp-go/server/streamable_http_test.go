package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestStreamableHTTPServerInitialize(t *testing.T) {
	mcpServer := NewMCPServer("test-server", "1.0.0")
	testServer := NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

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

	resp, err := http.Post(
		testServer.URL+DefaultStreamableHTTPPath,
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get(mcp.HeaderSessionID))
	require.Equal(
		t,
		mcp.LATEST_PROTOCOL_VERSION,
		resp.Header.Get(mcp.HeaderProtocolVersion),
	)

	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
	require.Equal(t, "2.0", response["jsonrpc"])
	require.EqualValues(t, 1, response["id"])
}
