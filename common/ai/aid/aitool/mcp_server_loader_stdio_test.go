package aitool

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMCPConfiguredStdioHelper(t *testing.T) {
	if os.Getenv("YAK_MCP_CONFIGURED_STDIO_CHILD") != "true" {
		return
	}
	s := server.NewMCPServer("configured-stdio", "1")
	s.AddTool(mcp.NewTool("env"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(os.Getenv("YAK_MCP_CONFIGURED_STDIO_TOKEN")), nil
	})
	if err := server.NewStdioServer(s).Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestLoadAIToolsFromMCPServerStdioQuotedCommandAndEnv(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "MCP server with spaces")
	require.NoError(t, os.Symlink(executable, path))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, err := LoadAIToolsFromMCPServer(ctx, &schema.MCPServer{
		Name: "stdio-config", Type: "stdio", Enable: true,
		Command: strconv.Quote(path) + " -test.run=^TestMCPConfiguredStdioHelper$",
		Envs: schema.MapStringAny{
			"YAK_MCP_CONFIGURED_STDIO_CHILD": "true",
			"YAK_MCP_CONFIGURED_STDIO_TOKEN": "configured-token-with-spaces",
		},
	}, []string{"env"})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	defer tools[0].BridgeMCPClient.Close()
	result, err := tools[0].Callback(ctx, InvokeParams{}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "configured-token-with-spaces", result)
}
