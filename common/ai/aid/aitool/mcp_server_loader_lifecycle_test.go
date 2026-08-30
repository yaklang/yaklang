package aitool

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

func newLoaderLifecycleFixture(t *testing.T) (*schema.MCPServer, <-chan struct{}) {
	t.Helper()
	mcpServer := server.NewMCPServer("loader-lifecycle", "1.0.0")
	mcpServer.AddTool(mcp.NewTool("echo"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
	})
	sseServer := server.NewSSEServer(mcpServer, "")
	mux := http.NewServeMux()
	sseServer.RegisterHandlers(mux)
	disconnected := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			defer close(disconnected)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sseServer.Shutdown(ctx)
		httpServer.CloseClientConnections()
		httpServer.Close()
	})
	return &schema.MCPServer{Name: "loader-lifecycle", Type: "sse", URL: httpServer.URL + "/sse"}, disconnected
}

func requireMCPDisconnected(t *testing.T, disconnected <-chan struct{}) {
	t.Helper()
	select {
	case <-disconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE request stayed open after client cleanup")
	}
}

func TestLoadAIToolsFromMCPServer_FailureClosesConnection(t *testing.T) {
	config, disconnected := newLoaderLifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tools, err := LoadAIToolsFromMCPServer(ctx, config, []string{"not-allowed"})
	require.Nil(t, tools)
	require.ErrorContains(t, err, "no tools loaded")
	requireMCPDisconnected(t, disconnected)
	require.NoError(t, ctx.Err())
}

func TestLoadAIToolsFromMCPServer_CloseIsIdempotent(t *testing.T) {
	config, disconnected := newLoaderLifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tools, err := LoadAIToolsFromMCPServer(ctx, config, nil)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	result, err := tools[0].Callback(ctx, InvokeParams{}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "ok", result)

	const closerCount = 8
	errors := make(chan error, closerCount)
	var wg sync.WaitGroup
	for i := 0; i < closerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- tools[0].BridgeMCPClient.Close()
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	requireMCPDisconnected(t, disconnected)
	require.NoError(t, ctx.Err(), "closing the client must not cancel its caller")
}

func TestLoadAIToolsFromMCPServer_CancelStopsSSEStartup(t *testing.T) {
	started := make(chan struct{})
	disconnected := make(chan struct{})
	release := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(started)
		defer close(disconnected)
		select {
		case <-r.Context().Done():
		case <-release:
			// Unblock an uncancellable legacy startup on test failure as well.
			_, _ = io.WriteString(w, "event: endpoint\ndata: /message\n\n")
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(func() {
		close(release)
		httpServer.CloseClientConnections()
		httpServer.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		_, err := LoadAIToolsFromMCPServer(ctx, &schema.MCPServer{
			Name: "startup-lifecycle", Type: "sse", URL: httpServer.URL,
		}, nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE request did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "create mcp client failed")
	case <-time.After(5 * time.Second):
		t.Fatal("SSE startup ignored caller cancellation")
	}
	requireMCPDisconnected(t, disconnected)
}
