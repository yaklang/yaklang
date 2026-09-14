package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

func TestStreamableHTTPMCPClientSessionStateConcurrentAccess(t *testing.T) {
	client, err := NewStreamableHTTPMCPClient("http://127.0.0.1/mcp")
	require.NoError(t, err)

	client.markInitialized(mcp.LATEST_PROTOCOL_VERSION)
	initialHeader := make(http.Header)
	initialHeader.Set(mcp.HeaderSessionID, "initial-session")
	client.updateSessionFromHeaders(initialHeader)

	const (
		writerCount = 4
		readerCount = 8
		iterations  = 1_000
	)

	errorsCh := make(chan error, readerCount)
	var wg sync.WaitGroup
	for writer := 0; writer < writerCount; writer++ {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				header := make(http.Header)
				header.Set(mcp.HeaderSessionID, fmt.Sprintf("session-%d-%d", writer, iteration))
				client.updateSessionFromHeaders(header)
			}
		}()
	}

	for reader := 0; reader < readerCount; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				req, requestErr := http.NewRequest(http.MethodGet, "http://127.0.0.1/mcp", nil)
				if requestErr != nil {
					errorsCh <- requestErr
					return
				}
				client.applyHeaders(req)

				if got := req.Header.Get(mcp.HeaderProtocolVersion); got != mcp.LATEST_PROTOCOL_VERSION {
					errorsCh <- fmt.Errorf("unexpected protocol version header %q", got)
					return
				}
				if got := req.Header.Get(mcp.HeaderSessionID); got == "" {
					errorsCh <- fmt.Errorf("missing session ID header")
					return
				}
				if !client.snapshotState().initialized {
					errorsCh <- fmt.Errorf("client unexpectedly became uninitialized")
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errorsCh)
	for concurrentErr := range errorsCh {
		require.NoError(t, concurrentErr)
	}
}

func TestStreamableHTTPMCPClient(t *testing.T) {
	observedClientCh := make(chan server.NotificationContext, 1)
	mcpServer := server.NewMCPServer(
		"test-server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCallObserver(func(
			ctx context.Context,
			_ mcp.CallToolRequest,
			_ *mcp.CallToolResult,
			_ error,
			_ time.Duration,
		) {
			scoped := server.ServerFromContext(ctx)
			require.NotNil(t, scoped)
			select {
			case observedClientCh <- scoped.CurrentClientContext():
			default:
			}
		}),
	)

	mcpServer.AddTool(
		mcp.NewTool(
			"test-tool",
			mcp.WithDescription("Test tool"),
			mcp.WithString("message", mcp.Description("Echo message")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			scopedServer := server.ServerFromContext(ctx)
			if scopedServer != nil {
				_ = scopedServer.SendNotificationToClient("test/progress", map[string]interface{}{
					"message": "tool-started",
				})
			}

			return &mcp.CallToolResult{
				Content: []interface{}{
					mcp.TextContent{
						Type: "text",
						Text: request.Params.Arguments["message"].(string),
					},
				},
			}, nil
		},
	)

	testServer := server.NewStreamableHTTPTestServer(mcpServer)
	defer testServer.Close()

	client, err := NewStreamableHTTPMCPClient(
		testServer.URL + server.DefaultStreamableHTTPPath,
	)
	require.NoError(t, err)
	defer client.Close()

	notificationCh := make(chan mcp.JSONRPCNotification, 1)
	client.OnNotification(func(notification mcp.JSONRPCNotification) {
		notificationCh <- notification
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}

	initResult, err := client.Initialize(ctx, initRequest)
	require.NoError(t, err)
	require.Equal(t, "test-server", initResult.ServerInfo.Name)

	time.Sleep(200 * time.Millisecond)

	toolsResult, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.Len(t, toolsResult.Tools, 1)
	require.Equal(t, "test-tool", toolsResult.Tools[0].Name)

	callRequest := mcp.CallToolRequest{}
	callRequest.Params.Name = "test-tool"
	callRequest.Params.Arguments = map[string]interface{}{
		"message": "hello from streamable http",
	}

	callResult, err := client.CallTool(ctx, callRequest)
	require.NoError(t, err)
	require.Len(t, callResult.Content, 1)

	select {
	case observedClient := <-observedClientCh:
		require.Equal(t, "test-client", observedClient.ClientName)
		require.Equal(t, "1.0.0", observedClient.ClientVersion)
		require.NotEmpty(t, observedClient.SessionID)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for streamable http tool observer")
	}

	select {
	case notification := <-notificationCh:
		require.Equal(t, "test/progress", notification.Method)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for streamable http notification")
	}
}
