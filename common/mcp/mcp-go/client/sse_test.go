package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

func TestSSEMCPClient(t *testing.T) {
	// Create MCP server with capabilities
	mcpServer := server.NewMCPServer(
		"test-server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
	)

	// Add a test tool
	mcpServer.AddTool(mcp.NewTool(
		"test-tool",
		mcp.WithDescription("Test tool"),
		mcp.WithString("parameter-1", mcp.Description("A string tool parameter")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []interface{}{
				mcp.TextContent{
					Type: "text",
					Text: "Input parameter: " + request.Params.Arguments["parameter-1"].(string),
				},
			},
		}, nil
	})

	// Initialize
	testServer := server.NewTestServer(mcpServer)
	defer testServer.Close()

	t.Run("Can create client", func(t *testing.T) {
		client, err := NewSSEMCPClient(testServer.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		if client.baseURL == nil {
			t.Error("Base URL should not be nil")
		}
	})

	t.Run("Can initialize and make requests", func(t *testing.T) {
		client, err := NewSSEMCPClient(testServer.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Start the client
		if err := client.Start(ctx); err != nil {
			t.Fatalf("Failed to start client: %v", err)
		}

		// Initialize
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}

		result, err := client.Initialize(ctx, initRequest)
		if err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		if result.ServerInfo.Name != "test-server" {
			t.Errorf(
				"Expected server name 'test-server', got '%s'",
				result.ServerInfo.Name,
			)
		}

		// Test Ping
		if err := client.Ping(ctx); err != nil {
			t.Errorf("Ping failed: %v", err)
		}

		// Test ListTools
		toolsRequest := mcp.ListToolsRequest{}
		_, err = client.ListTools(ctx, toolsRequest)
		if err != nil {
			t.Errorf("ListTools failed: %v", err)
		}
	})

	// t.Run("Can handle notifications", func(t *testing.T) {
	// 	client, err := NewSSEMCPClient(testServer.URL + "/sse")
	// 	if err != nil {
	// 		t.Fatalf("Failed to create client: %v", err)
	// 	}
	// 	defer client.Close()

	// 	notificationReceived := make(chan mcp.JSONRPCNotification, 1)
	// 	client.OnNotification(func(notification mcp.JSONRPCNotification) {
	// 		notificationReceived <- notification
	// 	})

	// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 	defer cancel()

	// 	if err := client.Start(ctx); err != nil {
	// 		t.Fatalf("Failed to start client: %v", err)
	// 	}

	// 	// Initialize first
	// 	initRequest := mcp.InitializeRequest{}
	// 	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	// 	initRequest.Params.ClientInfo = mcp.Implementation{
	// 		Name:    "test-client",
	// 		Version: "1.0.0",
	// 	}

	// 	_, err = client.Initialize(ctx, initRequest)
	// 	if err != nil {
	// 		t.Fatalf("Failed to initialize: %v", err)
	// 	}

	// 	// Subscribe to a resource to test notifications
	// 	subRequest := mcp.SubscribeRequest{}
	// 	subRequest.Params.URI = "test://resource"
	// 	if err := client.Subscribe(ctx, subRequest); err != nil {
	// 		t.Fatalf("Failed to subscribe: %v", err)
	// 	}

	// 	select {
	// 	case <-notificationReceived:
	// 		// Success
	// 	case <-time.After(time.Second):
	// 		t.Error("Timeout waiting for notification")
	// 	}
	// })

	t.Run("Handles errors properly", func(t *testing.T) {
		client, err := NewSSEMCPClient(testServer.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Start(ctx); err != nil {
			t.Fatalf("Failed to start client: %v", err)
		}

		// Try to make a request without initializing
		toolsRequest := mcp.ListToolsRequest{}
		_, err = client.ListTools(ctx, toolsRequest)
		if err == nil {
			t.Error("Expected error when making request before initialization")
		}
	})

	t.Run("CallTool", func(t *testing.T) {
		client, err := NewSSEMCPClient(testServer.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Start(ctx); err != nil {
			t.Fatalf("Failed to start client: %v", err)
		}

		// Initialize
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}

		_, err = client.Initialize(ctx, initRequest)
		if err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		request := mcp.CallToolRequest{}
		request.Params.Name = "test-tool"
		request.Params.Arguments = map[string]interface{}{
			"parameter-1": "value1",
		}

		result, err := client.CallTool(ctx, request)
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}

		if len(result.Content) != 1 {
			t.Errorf("Expected 1 content item, got %d", len(result.Content))
		}
	})
	// t.Run("Handles context cancellation", func(t *testing.T) {
	// 	client, err := NewSSEMCPClient(testServer.URL + "/sse")
	// 	if err != nil {
	// 		t.Fatalf("Failed to create client: %v", err)
	// 	}
	// 	defer client.Close()

	// 	if err := client.Start(context.Background()); err != nil {
	// 		t.Fatalf("Failed to start client: %v", err)
	// 	}

	// 	ctx, cancel := context.WithCancel(context.Background())
	// 	cancel() // Cancel immediately

	// 	toolsRequest := mcp.ListToolsRequest{}
	// 	_, err = client.ListTools(ctx, toolsRequest)
	// 	if err == nil {
	// 		t.Error("Expected error when context is cancelled")
	// 	}
	// })

	t.Run("CleanupPendingRequests notifies all waiters", func(t *testing.T) {
		client, err := NewSSEMCPClient("http://localhost:9999/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// Simulate pending requests by manually adding channels to responses map
		numRequests := 5
		errorChans := make([]chan RPCResponse, numRequests)

		client.mu.Lock()
		for i := 0; i < numRequests; i++ {
			id := int64(i + 1)
			ch := make(chan RPCResponse, 1)
			errorChans[i] = ch
			client.responses[id] = ch
		}
		client.initialized = true
		client.mu.Unlock()

		t.Logf("Added %d simulated pending requests", numRequests)

		// Verify pending requests exist
		client.mu.RLock()
		pendingBefore := len(client.responses)
		client.mu.RUnlock()
		if pendingBefore != numRequests {
			t.Errorf("Expected %d pending requests, got %d", numRequests, pendingBefore)
		}

		// Call cleanup (simulating connection loss)
		client.cleanupPendingRequests()

		// Verify all requests received error notification
		for i, ch := range errorChans {
			select {
			case resp := <-ch:
				if resp.Error == nil {
					t.Errorf("Request %d: Expected error, got nil", i+1)
				} else if *resp.Error != "SSE connection lost" {
					t.Errorf("Request %d: Expected 'SSE connection lost', got '%s'", i+1, *resp.Error)
				} else {
					t.Logf("Request %d: Got expected error", i+1)
				}
			case <-time.After(1 * time.Second):
				t.Errorf("Request %d: Timeout waiting for error notification", i+1)
			}
		}

		// Verify responses map is cleaned up
		client.mu.RLock()
		responsesAfter := len(client.responses)
		client.mu.RUnlock()
		if responsesAfter != 0 {
			t.Errorf("Expected responses map to be empty, got %d pending requests", responsesAfter)
		}

		// Verify isDisconnected flag is set
		if !client.isDisconnected.Load() {
			t.Error("isDisconnected should be true after cleanup")
		}
	})

	t.Run("Server shutdown triggers cleanup", func(t *testing.T) {
		// Create a dedicated server for this test
		mcpServer2 := server.NewMCPServer(
			"test-server-shutdown",
			"1.0.0",
		)
		testServer2 := server.NewTestServer(mcpServer2)

		client, err := NewSSEMCPClient(testServer2.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Start client and establish connection
		if err := client.Start(ctx); err != nil {
			t.Fatalf("Failed to start client: %v", err)
		}

		// Initialize
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}

		_, err = client.Initialize(ctx, initRequest)
		if err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// Verify client is connected
		if client.isDisconnected.Load() {
			t.Error("Client should not be disconnected initially")
		}

		// Add a mock pending request
		client.mu.Lock()
		mockCh := make(chan RPCResponse, 1)
		client.responses[999] = mockCh
		client.mu.Unlock()

		// Close the server to simulate connection loss
		testServer2.CloseClientConnections()
		testServer2.Close()

		// Wait for the pending request to receive error
		select {
		case resp := <-mockCh:
			if resp.Error == nil {
				t.Error("Expected error from pending request")
			} else if *resp.Error != "SSE connection lost" {
				t.Errorf("Expected 'SSE connection lost', got '%s'", *resp.Error)
			} else {
				t.Log("✅ Pending request received connection lost error")
			}
		case <-time.After(3 * time.Second):
			t.Error("Timeout waiting for pending request to be notified")
		}

		// Give cleanup time to complete
		time.Sleep(200 * time.Millisecond)

		// Verify isDisconnected is set
		if !client.isDisconnected.Load() {
			t.Error("isDisconnected should be true after server shutdown")
		}

		// Verify responses map is cleaned
		client.mu.RLock()
		responsesCount := len(client.responses)
		client.mu.RUnlock()
		if responsesCount != 0 {
			t.Errorf("Expected responses map to be empty, got %d", responsesCount)
		}

		t.Log("✅ Server shutdown handled correctly")
	})

	t.Run("Client.Close triggers cleanup", func(t *testing.T) {
		// Create a dedicated server for this test
		mcpServer3 := server.NewMCPServer(
			"test-server-close",
			"1.0.0",
		)
		testServer3 := server.NewTestServer(mcpServer3)
		defer testServer3.Close()

		client, err := NewSSEMCPClient(testServer3.URL + "/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Start client and establish connection
		if err := client.Start(ctx); err != nil {
			t.Fatalf("Failed to start client: %v", err)
		}

		// Initialize
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}

		_, err = client.Initialize(ctx, initRequest)
		if err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// Verify client is connected
		if client.isDisconnected.Load() {
			t.Error("Client should not be disconnected initially")
		}

		// Add multiple mock pending requests
		numRequests := 3
		mockChannels := make([]chan RPCResponse, numRequests)
		for i := 0; i < numRequests; i++ {
			mockChannels[i] = make(chan RPCResponse, 1)
			client.mu.Lock()
			client.responses[int64(1000+i)] = mockChannels[i]
			client.mu.Unlock()
		}

		t.Logf("Added %d mock pending requests", numRequests)

		// Call Close() - this should trigger cleanup
		err = client.Close()
		if err != nil {
			t.Errorf("Close() returned error: %v", err)
		}

		// Verify all pending requests received notification
		for i, ch := range mockChannels {
			select {
			case resp := <-ch:
				if resp.Error == nil {
					t.Errorf("Request %d: Expected error, got nil", i+1)
				} else if *resp.Error != "SSE connection lost" {
					t.Errorf("Request %d: Expected 'SSE connection lost', got '%s'", i+1, *resp.Error)
				} else {
					t.Logf("✅ Request %d: Received connection lost error", i+1)
				}
			case <-time.After(1 * time.Second):
				t.Errorf("Request %d: Timeout waiting for notification", i+1)
			}
		}

		// Verify isDisconnected is set
		if !client.isDisconnected.Load() {
			t.Error("isDisconnected should be true after Close()")
		}

		// Verify responses map is cleaned
		client.mu.RLock()
		responsesCount := len(client.responses)
		client.mu.RUnlock()
		if responsesCount != 0 {
			t.Errorf("Expected responses map to be empty, got %d", responsesCount)
		}

		// Verify Close() is idempotent - calling again should not error
		err = client.Close()
		if err != nil {
			t.Errorf("Second Close() returned error: %v", err)
		}

		t.Log("✅ Client.Close() handled correctly")
	})

	t.Run("CleanupPendingRequests with concurrent access", func(t *testing.T) {
		client, err := NewSSEMCPClient("http://localhost:9999/sse")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// Simulate multiple goroutines waiting for responses
		numRequests := 10
		errorChans := make([]chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			errorChans[i] = make(chan error, 1)
		}

		// Add requests concurrently
		for i := 0; i < numRequests; i++ {
			go func(idx int) {
				id := int64(idx + 1)
				ch := make(chan RPCResponse, 1)

				client.mu.Lock()
				client.responses[id] = ch
				client.mu.Unlock()

				// Wait for response
				resp := <-ch
				if resp.Error != nil {
					errorChans[idx] <- errors.New(*resp.Error)
				} else {
					errorChans[idx] <- nil
				}
			}(i)
		}

		// Wait for all requests to be registered
		time.Sleep(100 * time.Millisecond)

		client.mu.RLock()
		pendingBefore := len(client.responses)
		client.mu.RUnlock()
		t.Logf("Pending requests before cleanup: %d", pendingBefore)

		// Call cleanup
		client.cleanupPendingRequests()

		// Verify all goroutines received notification
		receivedCount := 0
		timeout := time.After(2 * time.Second)
		for receivedCount < numRequests {
			select {
			case err := <-errorChans[receivedCount]:
				if err == nil {
					t.Errorf("Request %d: Expected error, got nil", receivedCount+1)
				} else if err.Error() == "SSE connection lost" {
					t.Logf("Request %d: Received expected error", receivedCount+1)
				} else {
					t.Errorf("Request %d: Wrong error: %v", receivedCount+1, err)
				}
				receivedCount++
			case <-timeout:
				t.Fatalf("Timeout: only %d/%d requests notified", receivedCount, numRequests)
			}
		}

		// Verify cleanup
		client.mu.RLock()
		responsesAfter := len(client.responses)
		client.mu.RUnlock()
		if responsesAfter != 0 {
			t.Errorf("Expected responses map to be empty, got %d", responsesAfter)
		}

		if !client.isDisconnected.Load() {
			t.Error("isDisconnected should be true")
		}
	})
}
