package aitool

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// startSessionTestMCPServer 启动一个暴露 toolCount 个工具的 SSE MCP server，返回其 sseURL。
func startSessionTestMCPServer(t *testing.T, toolCount int) string {
	t.Helper()
	mcpServer := server.NewMCPServer("Session Test MCP Server", "1.0.0")
	for i := 0; i < toolCount; i++ {
		name := fmt.Sprintf("tool_%d", i)
		tool := mcp.NewTool(name, mcp.WithDescription("session test tool "+name))
		mcpServer.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []any{mcp.TextContent{Type: "text", Text: "ok"}}}, nil
		})
	}

	port := utils.GetRandomAvailableTCPPort()
	hostPort := utils.HostPort("127.0.0.1", port)
	baseURL := fmt.Sprintf("http://%s", hostPort)
	sseServer := server.NewSSEServer(mcpServer, baseURL)

	started := make(chan struct{})
	go func() {
		close(started)
		if err := sseServer.Start(hostPort); err != nil && err != http.ErrServerClosed {
			log.Errorf("session test SSE server error: %v", err)
		}
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, utils.WaitConnect(hostPort, 5))
	return baseURL + "/sse"
}

// TestLoadAIToolsFromMCPServer_Allowlist 验证 client 侧白名单过滤：
// server 暴露 10 个工具，会话只允许 6 个，最终只能加载这 6 个。
func TestLoadAIToolsFromMCPServer_Allowlist(t *testing.T) {
	sseURL := startSessionTestMCPServer(t, 10)

	srv := &schema.MCPServer{
		Name:   "session-allowlist",
		Type:   "sse",
		URL:    sseURL,
		Enable: true,
	}
	allowed := []string{"tool_0", "tool_1", "tool_2", "tool_3", "tool_4", "tool_5"}

	tools, err := LoadAIToolsFromMCPServer(context.Background(), srv, allowed)
	require.NoError(t, err)
	require.Len(t, tools, 6, "only allowlisted tools should be loaded")

	got := make(map[string]bool, len(tools))
	for _, tool := range tools {
		got[tool.Name] = true
	}
	for _, name := range allowed {
		require.True(t, got[fmt.Sprintf("mcp_%s_%s", srv.Name, name)], "allowed tool %s missing", name)
	}
	// disallowed tools must not leak even though the server exposed them
	for _, name := range []string{"tool_6", "tool_7", "tool_8", "tool_9"} {
		require.False(t, got[fmt.Sprintf("mcp_%s_%s", srv.Name, name)], "disallowed tool %s leaked", name)
	}
}

// TestLoadAIToolsFromMCPServer_NoAllowlist 验证白名单为空时加载全部工具。
func TestLoadAIToolsFromMCPServer_NoAllowlist(t *testing.T) {
	sseURL := startSessionTestMCPServer(t, 4)
	srv := &schema.MCPServer{
		Name:   "session-noallow",
		Type:   "sse",
		URL:    sseURL,
		Enable: true,
	}

	tools, err := LoadAIToolsFromMCPServer(context.Background(), srv, nil)
	require.NoError(t, err)
	require.Len(t, tools, 4)
}
