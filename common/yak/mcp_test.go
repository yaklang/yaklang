package yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestMCPStandardLibraryClientInYak(t *testing.T) {
	s := server.NewMCPServer("yak-standard-library", "1.0")
	s.AddTool(mcp.NewTool("echo"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(request.Params.Arguments["message"].(string)), nil
	})
	httpServer := server.NewStreamableHTTPTestServer(s)
	defer httpServer.Close()
	engine := NewScriptEngine(1)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{"endpoint": httpServer.URL + "/mcp"})
		return nil
	})
	_, err := engine.ExecuteExWithContext(context.Background(), `
client = mcp.NewClient("streamable_http", endpoint, mcp.headers({}), mcp.timeout(5))~
defer client.Close()
die(client.Ping())
page = client.ListTools()~
assert len(page.Tools) == 1 && page.Tools[0].Name == "echo"
result = client.CallTool("echo", {"message":"hello from Yak\n保留原文"})~
assert !result.IsError
assert "hello from Yak" in json.dumps(result)
`, nil)
	require.NoError(t, err)
}
