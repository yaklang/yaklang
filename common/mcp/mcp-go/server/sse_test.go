package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

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
			t.Errorf(
				"Expected baseURL http://localhost:8080, got %s",
				sseServer.baseURL,
			)
		}
	})

	t.Run("Can send and receive messages", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0",
			WithResourceCapabilities(true, true),
		)
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		// Connect to SSE endpoint
		sseResp, err := http.Get(fmt.Sprintf("%s/sse", testServer.URL))
		if err != nil {
			t.Fatalf("Failed to connect to SSE endpoint: %v", err)
		}
		defer sseResp.Body.Close()

		// Read the endpoint event
		buf := make([]byte, 1024)
		n, err := sseResp.Body.Read(buf)
		if err != nil {
			t.Fatalf("Failed to read SSE response: %v", err)
		}

		endpointEvent := string(buf[:n])
		if !strings.Contains(endpointEvent, "event: endpoint") {
			t.Fatalf("Expected endpoint event, got: %s", endpointEvent)
		}

		// Extract message endpoint URL
		messageURL := strings.TrimSpace(
			strings.Split(strings.Split(endpointEvent, "data: ")[1], "\n")[0],
		)

		// Send initialize request
		initRequest := map[string]interface{}{
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
		}

		requestBody, err := json.Marshal(initRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		resp, err := http.Post(
			messageURL,
			"application/json",
			bytes.NewBuffer(requestBody),
		)
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status 202, got %d", resp.StatusCode)
		}

		// Verify response
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
		mcpServer := NewMCPServer("test", "1.0.0",
			WithResourceCapabilities(true, true),
		)
		testServer := NewTestServer(mcpServer)
		defer testServer.Close()

		numSessions := 3
		var wg sync.WaitGroup
		wg.Add(numSessions)

		for i := 0; i < numSessions; i++ {
			go func(sessionNum int) {
				defer wg.Done()

				// Connect to SSE endpoint
				sseResp, err := http.Get(fmt.Sprintf("%s/sse", testServer.URL))
				if err != nil {
					t.Errorf(
						"Session %d: Failed to connect to SSE endpoint: %v",
						sessionNum,
						err,
					)
					return
				}
				defer sseResp.Body.Close()

				// Read the endpoint event
				buf := make([]byte, 1024)
				n, err := sseResp.Body.Read(buf)
				if err != nil {
					t.Errorf(
						"Session %d: Failed to read SSE response: %v",
						sessionNum,
						err,
					)
					return
				}

				endpointEvent := string(buf[:n])
				messageURL := strings.TrimSpace(
					strings.Split(strings.Split(endpointEvent, "data: ")[1], "\n")[0],
				)

				// Send initialize request
				initRequest := map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      sessionNum,
					"method":  "initialize",
					"params": map[string]interface{}{
						"protocolVersion": "2024-11-05",
						"clientInfo": map[string]interface{}{
							"name": fmt.Sprintf(
								"test-client-%d",
								sessionNum,
							),
							"version": "1.0.0",
						},
					},
				}

				requestBody, err := json.Marshal(initRequest)
				if err != nil {
					t.Errorf(
						"Session %d: Failed to marshal request: %v",
						sessionNum,
						err,
					)
					return
				}

				resp, err := http.Post(
					messageURL,
					"application/json",
					bytes.NewBuffer(requestBody),
				)
				if err != nil {
					t.Errorf(
						"Session %d: Failed to send message: %v",
						sessionNum,
						err,
					)
					return
				}
				defer resp.Body.Close()

				var response map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					t.Errorf(
						"Session %d: Failed to decode response: %v",
						sessionNum,
						err,
					)
					return
				}

				if response["id"].(float64) != float64(sessionNum) {
					t.Errorf(
						"Session %d: Expected id %d, got %v",
						sessionNum,
						sessionNum,
						response["id"],
					)
				}
			}(i)
		}

		// Wait with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All sessions completed successfully
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for sessions to complete")
		}
	})
}
