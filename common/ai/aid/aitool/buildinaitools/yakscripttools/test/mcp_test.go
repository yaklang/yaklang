package test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 9, 7))))
	encodedImage := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	remote.AddTool(mcp.NewTool("capture"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := mcp.NewToolResultImage("screen capture", encodedImage, "image/png")
		result.StructuredContent = json.RawMessage(`{"window_id":9007199254740993,"state_id":"screen-1","image_content_index":1}`)
		return result, nil
	})
	remote.AddTool(mcp.NewTool("inspect"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []any{mcp.NewTextContent("inspect result")}, StructuredContent: json.RawMessage(`{"state_id":"text-only"}`)}, nil
	})
	remote.AddTool(mcp.NewTool("echo", mcp.WithString("message", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(req.Params.Arguments["message"].(string)), nil
	})
	remote.AddTool(mcp.NewTool("failure"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("EXPECTED_REMOTE_FAILURE"), nil
	})
	const recoveryState = `{"state_id":"latest","image_content_index":1,"task":{"checkpoint":{"id":23}},"error":{"code":"PopupChanged","execution_state":"NotStarted"}}`
	remote.AddTool(mcp.NewTool("recover"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			IsError:           true,
			StructuredContent: json.RawMessage(recoveryState),
			Content: []any{
				mcp.NewTextContent("Popup closed"),
				mcp.NewImageContent(encodedImage, "image/png"),
				mcp.NewTextContent("resume_from 23"),
			},
		}, nil
	})
	entered := make(chan struct{}, 2)
	remote.AddTool(mcp.NewTool("slow"), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
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
	require.NotContains(t, called, "structuredContent")
	inspected := invoke("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "inspect"})
	require.Contains(t, inspected, `"structuredContent":{"state_id":"text-only"}`)
	require.NoError(t, yakit.UpsertMCPServerToolConfig(profile, "fixture", "echo", false))
	_, err = execute("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "echo", "arguments": map[string]any{"message": "blocked"}})
	require.Error(t, err)
	require.NotContains(t, invoke("query_mcp_tools", map[string]any{"server_name": "fixture"}), `"name":"echo"`)
	capture, err := execute("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "capture"})
	require.NoError(t, err)
	require.Len(t, capture.Images, 1)
	require.NotContains(t, capture.Stdout, encodedImage)
	require.Equal(t, 9, capture.Images[0].Width)
	result := capture.Result.(*mcp.CallToolResult)
	require.Equal(t, `{"window_id":9007199254740993,"state_id":"screen-1","image_content_index":1}`, string(result.StructuredContent))
	require.Len(t, result.Content, 2)
	require.Equal(t, capture.Images[0].ID, result.Content[1].(map[string]any)["image_id"])
	require.Equal(t, 0, result.Content[1].(map[string]any)["image_index"])
	require.NotContains(t, result.Content[1], "data")
	failed, err := execute("call_mcp_tool", map[string]any{"server_name": "fixture", "remote_tool": "failure"})
	require.Error(t, err)
	require.Contains(t, failed.Stdout, "EXPECTED_REMOTE_FAILURE")
	invokeMCP := func(ctx context.Context, name string, params map[string]any) (*aitool.ToolResult, error) {
		args := map[string]any{"server_name": "fixture", "remote_tool": name}
		for key, value := range params {
			args[key] = value
		}
		return tools["call_mcp_tool"].InvokeWithParams(args, aitool.WithContext(ctx),
			aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProfileDatabase: profile}), aitool.WithWaitForCallbackOnCancel(true))
	}
	recovered, err := invokeMCP(ctx, "recover", nil)
	require.ErrorContains(t, err, "Popup closed")
	require.False(t, recovered.Success)
	require.Len(t, recovered.Images, 1)
	data, err := recovered.Images[0].Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, pngBytes.Bytes(), data)
	execution := recovered.Data.(*aitool.ToolExecutionResult)
	failure := execution.Result.(*mcp.CallToolResult)
	require.True(t, failure.IsError)
	require.Equal(t, recoveryState, string(failure.StructuredContent))
	require.Equal(t, recovered.Images[0].ID, failure.Content[1].(map[string]any)["image_id"])
	require.Contains(t, recovered.String(), "resume_from 23")
	require.NotContains(t, execution.Stdout, encodedImage)
	require.NotContains(t, recovered.String(), encodedImage)

	timedOut, err := invokeMCP(ctx, "slow", map[string]any{"timeout": 0.25})
	require.ErrorContains(t, err, context.DeadlineExceeded.Error())
	require.False(t, timedOut.Success)
	require.Nil(t, timedOut.Data)
	require.Empty(t, timedOut.Images)
	select {
	case <-entered:
	default:
		t.Fatal("timeout test did not reach the remote tool")
	}
	cancelCtx, cancelCall := context.WithTimeout(ctx, 3*time.Second)
	defer cancelCall()
	go func() {
		select {
		case <-entered:
			cancelCall()
		case <-cancelCtx.Done():
		}
	}()
	cancelled, err := invokeMCP(cancelCtx, "slow", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, cancelled.Success)
	require.Nil(t, cancelled.Data)
	require.Empty(t, cancelled.Images)
	checked, err := invokeMCP(ctx, "inspect", nil)
	require.NoError(t, err)
	require.True(t, checked.Success)
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
