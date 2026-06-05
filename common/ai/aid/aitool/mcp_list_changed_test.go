package aitool

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSSEMCPServer_DeliversToolsListChangedToClient(t *testing.T) {
	mcpServer := server.NewMCPServer("Notify Test", "1.0.0")
	mcpServer.AddTool(
		mcp.NewTool("seed_tool"),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
		},
	)

	port := utils.GetRandomAvailableTCPPort()
	hostPort := utils.HostPort("127.0.0.1", port)
	baseURL := fmt.Sprintf("http://%s", hostPort)
	sseSrv := server.NewSSEServer(mcpServer, baseURL)
	go func() {
		_ = sseSrv.Start(hostPort)
	}()
	require.NoError(t, utils.WaitConnect(hostPort, 5))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mcpClient, err := client.NewSSEMCPClient(baseURL+"/sse", nil)
	require.NoError(t, err)
	require.NoError(t, mcpClient.Start(ctx))

	notifyMethod := make(chan string, 1)
	mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		select {
		case notifyMethod <- notification.Method:
		default:
		}
	})

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	mcpServer.AddTool(
		mcp.NewTool("late_tool"),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "late"}}}, nil
		},
	)

	select {
	case method := <-notifyMethod:
		assert.Equal(t, "notifications/tools/list_changed", method)
	case <-ctx.Done():
		t.Fatal("timeout waiting for tools/list_changed notification on SSE client")
	}
}

func TestRefreshAIToolsFromMCPServer_IncludesDynamicallyAddedTool(t *testing.T) {
	serverName := "test_refresh_" + utils.RandStringBytes(8)
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	defer func() {
		var old schema.MCPServer
		if err := db.Where("name = ?", serverName).First(&old).Error; err == nil {
			db.Unscoped().Delete(&old)
		}
		_ = yakit.DeleteMCPServerToolConfigs(db, serverName)
	}()

	mcpServer := server.NewMCPServer("Refresh Test", "1.0.0")
	mcpServer.AddTool(
		mcp.NewTool("tool_alpha", mcp.WithString("message", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "a"}}}, nil
		},
	)

	port := utils.GetRandomAvailableTCPPort()
	hostPort := utils.HostPort("127.0.0.1", port)
	baseURL := fmt.Sprintf("http://%s", hostPort)
	sseURL := baseURL + "/sse"
	sseSrv := server.NewSSEServer(mcpServer, baseURL)
	go func() { _ = sseSrv.Start(hostPort) }()
	require.NoError(t, utils.WaitConnect(hostPort, 5))

	require.NoError(t, yakit.CreateMCPServer(db, &schema.MCPServer{
		Name: serverName, Type: "sse", URL: sseURL, Enable: true,
	}))

	ctx := context.Background()
	mcpClient, err := client.NewSSEMCPClient(sseURL, nil)
	require.NoError(t, err)
	require.NoError(t, mcpClient.Start(ctx))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	cfg, err := yakit.GetMCPServerByName(db, serverName)
	require.NoError(t, err)

	mcpServer.AddTool(
		mcp.NewTool("tool_beta", mcp.WithString("x", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "b"}}}, nil
		},
	)

	refreshed, err := refreshAIToolsFromMCPServer(ctx, db, cfg, mcpClient)
	require.NoError(t, err)
	require.Len(t, refreshed, 2)
	names := []string{refreshed[0].Name, refreshed[1].Name}
	assert.Contains(t, names, fmt.Sprintf("mcp_%s_tool_alpha", serverName))
	assert.Contains(t, names, fmt.Sprintf("mcp_%s_tool_beta", serverName))
}

func TestMCPToolsListChangedState_ApplyRefresh(t *testing.T) {
	serverName := "test_apply_refresh_" + utils.RandStringBytes(8)
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	defer func() {
		var old schema.MCPServer
		if err := db.Where("name = ?", serverName).First(&old).Error; err == nil {
			db.Unscoped().Delete(&old)
		}
		_ = yakit.DeleteMCPServerToolConfigs(db, serverName)
	}()

	mcpServer := server.NewMCPServer("Apply Refresh Test", "1.0.0")
	mcpServer.AddTool(
		mcp.NewTool("tool_alpha", mcp.WithString("message", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "alpha"}}}, nil
		},
	)
	mcpServer.AddTool(
		mcp.NewTool("tool_beta", mcp.WithString("x", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "beta"}}}, nil
		},
	)

	port := utils.GetRandomAvailableTCPPort()
	hostPort := utils.HostPort("127.0.0.1", port)
	baseURL := fmt.Sprintf("http://%s", hostPort)
	sseURL := baseURL + "/sse"
	sseSrv := server.NewSSEServer(mcpServer, baseURL)
	go func() { _ = sseSrv.Start(hostPort) }()
	require.NoError(t, utils.WaitConnect(hostPort, 5))
	require.NoError(t, yakit.CreateMCPServer(db, &schema.MCPServer{
		Name: serverName, Type: "sse", URL: sseURL, Enable: true,
	}))

	ctx := context.Background()
	mcpClient, err := client.NewSSEMCPClient(sseURL, nil)
	require.NoError(t, err)
	require.NoError(t, mcpClient.Start(ctx))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	cfg, err := yakit.GetMCPServerByName(db, serverName)
	require.NoError(t, err)

	var (
		mu        sync.Mutex
		refreshed []*Tool
	)
	handler := func(_ string, tools []*Tool, _ []string) {
		mu.Lock()
		defer mu.Unlock()
		refreshed = tools
	}
	state := newMCPToolsListChangedState(nil)
	state.applyRefresh(db, cfg, mcpClient, handler)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, refreshed, 2)
	names := []string{refreshed[0].Name, refreshed[1].Name}
	assert.Contains(t, names, fmt.Sprintf("mcp_%s_tool_alpha", serverName))
	assert.Contains(t, names, fmt.Sprintf("mcp_%s_tool_beta", serverName))
}
