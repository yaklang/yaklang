package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

func TestMCPServerListToolsJSONFormat(t *testing.T) {
	log.SetLevel(log.FatalLevel)

	s, err := NewMCPServer()
	require.NoError(t, err)

	testServer := mcpserver.NewTestServer(s.server)
	defer testServer.Close()

	println(testServer.URL)
	time.Sleep(10 * time.Hour)
	sseResp, messageURL := openMcpSSESession(t, testServer.URL)
	defer sseResp.Body.Close()

	initializeResp := postMcpJSONRequest(t, messageURL, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{
				"name":    "test-list-tools-json-client",
				"version": "1.0.0",
			},
		},
	})
	defer initializeResp.Body.Close()
	require.Equal(t, http.StatusAccepted, initializeResp.StatusCode)

	listResp := postMcpJSONRequest(t, messageURL, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	defer listResp.Body.Close()
	require.Equal(t, http.StatusAccepted, listResp.StatusCode)

	rawJSON, err := io.ReadAll(listResp.Body)
	// println(string(rawJSON))
	require.NoError(t, err)
	require.True(t, json.Valid(rawJSON), "tools/list response should be valid JSON: %s", string(rawJSON))

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description,omitempty"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rawJSON, &response))

	require.Equal(t, "2.0", response.JSONRPC)
	require.Equal(t, 2, response.ID)
	require.Nil(t, response.Error)
	require.NotEmpty(t, response.Result.Tools)

	expectedToolNames := collectExpectedSSEToolNames()
	actualToolNames := make(map[string]struct{}, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		require.NotEmpty(t, tool.Name)
		require.NotNil(t, tool.InputSchema)

		schemaType, ok := tool.InputSchema["type"].(string)
		require.True(t, ok)
		require.Equal(t, "object", schemaType)

		actualToolNames[tool.Name] = struct{}{}
	}

	for name := range expectedToolNames {
		_, ok := actualToolNames[name]
		require.Truef(t, ok, "missing tool from SSE tools/list response JSON: %s", name)
	}

	_, hasDynamicAddTool := actualToolNames["dynamic_add_tool"]
	require.False(t, hasDynamicAddTool, "dynamic_add_tool should be hidden over SSE")

	_, hasExecYakScript := actualToolNames["exec_yak_script"]
	require.False(t, hasExecYakScript, "exec_yak_script should be hidden over SSE")
}

func collectExpectedSSEToolNames() map[string]struct{} {
	expected := collectExpectedGlobalToolNames()
	delete(expected, "dynamic_add_tool")
	delete(expected, "exec_yak_script")
	return expected
}

func collectExpectedGlobalToolNames() map[string]struct{} {
	expected := make(map[string]struct{}, len(globalTools))
	for name := range globalTools {
		expected[name] = struct{}{}
	}
	for _, toolSet := range globalToolSets {
		for name := range toolSet.Tools {
			expected[name] = struct{}{}
		}
	}
	return expected
}

func openMcpSSESession(t *testing.T, baseURL string) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/sse", nil)
	require.NoError(t, err)
	req.Close = true

	resp, err := newMcpTestHTTPClient().Do(req)
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err)

	endpointEvent := string(buf[:n])
	require.Contains(t, endpointEvent, "event: endpoint")

	messageURL := strings.TrimSpace(strings.Split(strings.Split(endpointEvent, "data: ")[1], "\n")[0])
	return resp, messageURL
}

func postMcpJSONRequest(t *testing.T, messageURL string, payload map[string]any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, messageURL, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Close = true

	resp, err := newMcpTestHTTPClient().Do(req)
	require.NoError(t, err)
	return resp
}

func newMcpTestHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}
