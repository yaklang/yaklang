package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestMUSTPASS_MCPServer_AddTool_SendsToolsListChangedAfterInit(t *testing.T) {
	server := NewMCPServer("test-server", "1.0.0")
	notificationCh, unsubscribe := server.SubscribeNotifications(4)
	defer unsubscribe()
	require.NotNil(t, notificationCh)

	initMessage := []byte(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"clientInfo": {"name": "test-client", "version": "1.0.0"}
		}
	}`)
	resp := server.HandleMessage(context.Background(), initMessage)
	require.NotNil(t, resp)

	server.AddTool(mcp.NewTool("dynamic_tool"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []any{mcp.TextContent{Type: "text", Text: "ok"}},
		}, nil
	})

	select {
	case n := <-notificationCh:
		assert.Equal(t, "notifications/tools/list_changed", n.Notification.Method)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tools/list_changed notification")
	}
}
