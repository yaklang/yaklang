package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Real embedded scripts, CLI argument conversion, invocation-local profiles,
// SQLite persistence and HTTP MCP. No MCP or database functions are mocked.
func TestMCPYakTools(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	defer profile.Close()
	require.NoError(t, profile.AutoMigrate(&schema.MCPServer{}, &schema.MCPServerToolConfig{}, &schema.MCPClientToolConfig{}).Error)
	remote := server.NewMCPServer("tool-fixture", "1")
	remote.AddTool(mcp.NewTool("echo", mcp.WithString("message", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(req.Params.Arguments["message"].(string)), nil
	})
	remote.AddTool(mcp.NewTool("failure"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("EXPECTED_REMOTE_FAILURE"), nil
	})
	handler := server.NewStreamableHTTPServer(remote, "")
	mux := http.NewServeMux()
	handler.RegisterHandlers(mux)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer SECRET_HEADER_VALUE" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	defer handler.Shutdown(context.Background())
	tools := make(map[string]*aitool.Tool)
	for _, name := range []string{"manage_mcp_server", "query_mcp_tools", "call_mcp_tool"} {
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/mcp/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, converted, 1)
		tools[name] = converted[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	execute := func(name string, params map[string]any) (*aitool.ToolExecutionResult, error) {
		cfg := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProfileDatabase: profile})(cfg)
		return tools[name].ExecuteToolWithCapture(ctx, params, cfg)
	}
	invoke := func(name string, params map[string]any) string {
		t.Helper()
		out, err := execute(name, params)
		require.NoError(t, err)
		require.NotEmpty(t, out.Stdout)
		var compact bytes.Buffer
		require.NoError(t, json.Compact(&compact, []byte(out.Stdout)))
		return compact.String()
	}
	added := invoke("manage_mcp_server", map[string]any{"op": "add", "name": "fixture", "type": "streamable_http", "endpoint": httpServer.URL + "/mcp", "headers": map[string]any{"Authorization": "Bearer SECRET_HEADER_VALUE"}})
	require.Contains(t, added, "fixture")
	require.NotContains(t, added, "SECRET_HEADER_VALUE")
	stored, err := yakit.GetMCPServerByName(profile, "fixture")
	require.NoError(t, err)
	require.Equal(t, "Bearer SECRET_HEADER_VALUE", stored.Headers["Authorization"])
	disabled := invoke("manage_mcp_server", map[string]any{"op": "add", "name": "disabled", "type": "stdio", "endpoint": `python3 "/tmp/file with spaces.py"`, "env": map[string]any{"TOKEN": "SECRET_ENV_VALUE"}, "disabled": true})
	require.Contains(t, disabled, `"enable":false`)
	listed := invoke("manage_mcp_server", map[string]any{"op": "list", "limit": 1})
	var page map[string]any
	require.NoError(t, json.Unmarshal([]byte(listed), &page))
	require.Equal(t, float64(2), page["total"])
	require.True(t, page["has_more"].(bool))
	require.NotContains(t, listed, "SECRET_ENV_VALUE")
	require.NotContains(t, listed, "SECRET_HEADER_VALUE")
	require.Contains(t, invoke("manage_mcp_server", map[string]any{"op": "list", "offset": 1, "limit": 1}), `"has_more":false`)
	queried := invoke("query_mcp_tools", map[string]any{"server_name": "fixture"})
	require.Contains(t, queried, `"inputSchema"`)
	require.Contains(t, queried, `"message"`)
	message := "hello from real Yak tool\n引号\""
	called := invoke("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "echo", "arguments": map[string]any{"message": message}})
	require.Contains(t, called, `hello from real Yak tool\n引号\"`)
	require.NoError(t, yakit.UpsertMCPServerToolConfig(profile, "fixture", "echo", false))
	_, err = execute("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "echo", "arguments": map[string]any{"message": "blocked"}})
	require.Error(t, err)
	require.NotContains(t, invoke("query_mcp_tools", map[string]any{"server_name": "fixture"}), `"name":"echo"`)
	failed, err := execute("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "failure"})
	require.Error(t, err)
	require.Contains(t, string(failed.Stdout), "EXPECTED_REMOTE_FAILURE")
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"manage_mcp_server", map[string]any{"op": "invalid"}},
		{"manage_mcp_server", map[string]any{"op": "delete"}},
		{"manage_mcp_server", map[string]any{"op": "add", "name": "missing-transport"}},
		{"manage_mcp_server", map[string]any{"op": "list", "offset": -1}},
		{"manage_mcp_server", map[string]any{"op": "add", "name": "fixture", "type": "stdio", "endpoint": "different"}},
		{"query_mcp_tools", map[string]any{"server_name": "disabled"}},
		{"call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "echo", "arguments": []any{1}}},
	} {
		_, err := execute(tc.name, tc.params)
		require.Error(t, err, "%s %v", tc.name, tc.params)
	}
	require.NoError(t, yakit.UpsertMCPServerToolMetadata(profile, "fixture", "cached", "metadata", "[]"))
	require.Contains(t, invoke("manage_mcp_server", map[string]any{"op": "delete", "name": "fixture"}), `"deleted":true`)
	_, err = yakit.GetMCPServerByName(profile, "fixture")
	require.Error(t, err)
	var count int
	require.NoError(t, profile.Model(&schema.MCPServerToolConfig{}).Where("server_name = ?", "fixture").Count(&count).Error)
	require.Zero(t, count)
	_, err = execute("query_mcp_tools", map[string]any{"server_name": "fixture"})
	require.Error(t, err)
}
