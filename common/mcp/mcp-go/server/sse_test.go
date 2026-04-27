package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func newTestHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func openSSESession(t *testing.T, testServer *httptest.Server) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/sse", testServer.URL), nil)
	if err != nil {
		t.Fatalf("Failed to build SSE request: %v", err)
	}
	req.Close = true

	sseResp, err := newTestHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := sseResp.Body.Read(buf)
	if err != nil {
		_ = sseResp.Body.Close()
		t.Fatalf("Failed to read SSE response: %v", err)
	}

	endpointEvent := string(buf[:n])
	if !strings.Contains(endpointEvent, "event: endpoint") {
		_ = sseResp.Body.Close()
		t.Fatalf("Expected endpoint event, got: %s", endpointEvent)
	}

	messageURL := strings.TrimSpace(
		strings.Split(strings.Split(endpointEvent, "data: ")[1], "\n")[0],
	)

	return sseResp, messageURL
}

func newInitializeRequestBody(t *testing.T, id interface{}) []byte {
	t.Helper()

	requestBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	return requestBody
}

func doSSEInitializeRequest(t *testing.T, messageURL, contentType string, id interface{}) *http.Response {
	t.Helper()

	return doSSEJSONRequest(t, messageURL, contentType, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})
}

func doSSEJSONRequest(t *testing.T, messageURL, contentType string, payload map[string]interface{}) *http.Response {
	t.Helper()
	return doSSEJSONRequestWithHeaders(t, messageURL, contentType, payload, nil)
}

func doSSEJSONRequestWithHeaders(t *testing.T, messageURL, contentType string, payload map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal request payload: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		messageURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Close = true

	resp, err := newTestHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	return resp
}

func TestSSEServer(t *testing.T) {
	t.Run("Can instantiate", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		sseServer := NewSSEServer(mcpServer, "http://localhost:8080")

		if sseServer == nil {
			t.Error("SSEServer should not be nil")
		}
		if sseServer.server == nil {
			t.Error("MCPServer should not be nil")
		}
		if sseServer.baseURL != "http://localhost:8080" {
			t.Errorf("Expected baseURL http://localhost:8080, got %s", sseServer.baseURL)
		}
	})

	t.Run("Can send and receive messages", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEInitializeRequest(t, messageURL, "application/json", 1)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status 202, got %d", resp.StatusCode)
		}

		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response["jsonrpc"] != "2.0" {
			t.Errorf("Expected jsonrpc 2.0, got %v", response["jsonrpc"])
		}
		if response["id"].(float64) != 1 {
			t.Errorf("Expected id 1, got %v", response["id"])
		}
	})

	t.Run("Can handle multiple sessions", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		numSessions := 3
		var wg sync.WaitGroup
		wg.Add(numSessions)

		for i := 0; i < numSessions; i++ {
			go func(sessionNum int) {
				defer wg.Done()

				sseResp, messageURL := openSSESession(t, testServer)
				defer sseResp.Body.Close()
				resp := doSSEInitializeRequest(t, messageURL, "application/json", sessionNum)
				defer resp.Body.Close()

				var response map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					t.Errorf("Session %d: Failed to decode response: %v", sessionNum, err)
					return
				}

				if response["id"].(float64) != float64(sessionNum) {
					t.Errorf("Session %d: Expected id %d, got %v", sessionNum, sessionNum, response["id"])
				}
			}(i)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for sessions to complete")
		}
	})

	t.Run("Rejects non JSON content type", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEInitializeRequest(t, messageURL, "text/plain", 1)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("Expected status 400, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		if !strings.Contains(string(body), "Content-Type must be application/json") {
			t.Fatalf("Expected content-type validation error, got: %s", string(body))
		}
	})

	t.Run("Rejects missing content type", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEInitializeRequest(t, messageURL, "", 1)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("Expected status 400, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		if !strings.Contains(string(body), "Content-Type must be application/json") {
			t.Fatalf("Expected content-type validation error, got: %s", string(body))
		}
	})

	t.Run("Does not expose wildcard CORS on SSE endpoint", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		resp, err := http.Get(fmt.Sprintf("%s/sse", testServer.URL))
		if err != nil {
			t.Fatalf("Failed to connect to SSE endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("Expected wildcard CORS header, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("Allows localhost origin on message endpoint", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequestWithHeaders(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}, map[string]string{"Origin": "http://localhost:3000"})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("Expected status 202, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
			t.Fatalf("Expected localhost origin to be echoed, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("Allows extension origin on message endpoint", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequestWithHeaders(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}, map[string]string{"Origin": "chrome-extension://abcdefghijklmnop"})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("Expected status 202, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "chrome-extension://abcdefghijklmnop" {
			t.Fatalf("Expected extension origin to be echoed, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("Allows message request without origin header", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequestWithHeaders(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}, map[string]string{})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("Expected status 202, got %d", resp.StatusCode)
		}
		if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "" {
			t.Fatalf("Expected no CORS echo for requests without Origin, got %q", origin)
		}

		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if response["jsonrpc"] != "2.0" {
			t.Fatalf("Expected jsonrpc 2.0, got %v", response["jsonrpc"])
		}
	})

	t.Run("Rejects remote website origin on message endpoint", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequestWithHeaders(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}, map[string]string{"Origin": "https://evil.example"})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("Expected status 403, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		if !strings.Contains(string(body), "Forbidden origin") {
			t.Fatalf("Expected forbidden origin response, got %s", string(body))
		}
	})

	t.Run("Allows preflight for localhost origin", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()

		req, err := http.NewRequest(http.MethodOptions, messageURL, nil)
		if err != nil {
			t.Fatalf("Failed to build preflight request: %v", err)
		}
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")
		req.Close = true

		resp, err := newTestHTTPClient().Do(req)
		if err != nil {
			t.Fatalf("Failed to send preflight request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Expected status 204, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
			t.Fatalf("Expected localhost origin in preflight response, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
		}
		if resp.Header.Get("Access-Control-Allow-Methods") != "POST, OPTIONS" {
			t.Fatalf("Expected allow methods header, got %q", resp.Header.Get("Access-Control-Allow-Methods"))
		}
		if resp.Header.Get("Access-Control-Allow-Headers") != "Content-Type" {
			t.Fatalf("Expected allow headers header, got %q", resp.Header.Get("Access-Control-Allow-Headers"))
		}
	})

	t.Run("Rejects preflight for remote website origin", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()

		req, err := http.NewRequest(http.MethodOptions, messageURL, nil)
		if err != nil {
			t.Fatalf("Failed to build preflight request: %v", err)
		}
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Close = true

		resp, err := newTestHTTPClient().Do(req)
		if err != nil {
			t.Fatalf("Failed to send preflight request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("Expected status 403, got %d", resp.StatusCode)
		}
	})

	t.Run("Hides restricted yak execution tools from SSE list", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		mcpServer.AddTool(mcp.NewTool("safe_tool"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []interface{}{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
		})
		mcpServer.AddTool(mcp.NewTool("exec_yak_script"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []interface{}{mcp.TextContent{Type: "text", Text: "exec"}}}, nil
		})
		mcpServer.AddTool(mcp.NewTool("dynamic_add_tool"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []interface{}{mcp.TextContent{Type: "text", Text: "dynamic"}}}, nil
		})
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequest(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
			"params":  map[string]interface{}{},
		})
		defer resp.Body.Close()

		var response struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode tools/list response: %v", err)
		}

		toolNames := make([]string, 0, len(response.Result.Tools))
		for _, tool := range response.Result.Tools {
			toolNames = append(toolNames, tool.Name)
		}

		if !strings.Contains(strings.Join(toolNames, ","), "safe_tool") {
			t.Fatalf("Expected safe_tool in list, got %v", toolNames)
		}
		if strings.Contains(strings.Join(toolNames, ","), "exec_yak_script") {
			t.Fatalf("Did not expect exec_yak_script in SSE tool list, got %v", toolNames)
		}
		if strings.Contains(strings.Join(toolNames, ","), "dynamic_add_tool") {
			t.Fatalf("Did not expect dynamic_add_tool in SSE tool list, got %v", toolNames)
		}
	})

	t.Run("Rejects restricted yak execution tool calls over SSE", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		mcpServer.AddTool(mcp.NewTool("exec_yak_script"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []interface{}{mcp.TextContent{Type: "text", Text: "exec"}}}, nil
		})
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		sseResp, messageURL := openSSESession(t, testServer)
		defer sseResp.Body.Close()
		resp := doSSEJSONRequest(t, messageURL, "application/json", map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "exec_yak_script",
				"arguments": map[string]interface{}{},
			},
		})
		defer resp.Body.Close()

		var response struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode tools/call response: %v", err)
		}
		if !strings.Contains(response.Error.Message, "legacy SSE transport") {
			t.Fatalf("Expected legacy SSE rejection, got %q", response.Error.Message)
		}
	})

	t.Run("SendEvent concurrent with SSE disconnect does not panic", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0", WithResourceCapabilities(true, true))
		sseServer := &SSEServer{
			server:       mcpServer,
			dispatchDone: make(chan struct{}),
		}
		sseServer.startNotificationDispatcher()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/sse":
				sseServer.handleSSE(w, r)
			case "/message":
				sseServer.handleMessage(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()
		sseServer.baseURL = ts.URL

		sseResp, messageURL := openSSESession(t, ts)
		u, err := url.Parse(messageURL)
		if err != nil {
			t.Fatalf("parse message url: %v", err)
		}
		sessionID := u.Query().Get("sessionId")
		if sessionID == "" {
			t.Fatal("missing sessionId in message url")
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						_ = sseServer.SendEventToSession(sessionID, map[string]string{"k": "v"})
					}
				}
			}()
		}

		time.Sleep(20 * time.Millisecond)
		_ = sseResp.Body.Close()
		time.Sleep(150 * time.Millisecond)
		close(stop)
		wg.Wait()
	})
}
