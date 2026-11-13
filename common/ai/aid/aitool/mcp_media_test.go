package aitool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

type imageMCPClient struct {
	client.MCPClient
	result *mcp.CallToolResult
}

func (c *imageMCPClient) CallTool(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return c.result, nil
}

func mediaPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 11, 9))))
	return buf.Bytes()
}

func TestMCPImagesSurviveToolInvocationWithoutBecomingText(t *testing.T) {
	raw := mediaPNG(t)
	encoded := base64.StdEncoding.EncodeToString(raw)
	for _, wireFormat := range []bool{false, true} {
		t.Run(map[bool]string{false: "SDK_structs", true: "JSON_RPC_maps"}[wireFormat], func(t *testing.T) {
			remote := mcp.NewToolResultImage("current screen", encoded, "image/png")
			if wireFormat {
				wire, err := json.Marshal(remote)
				require.NoError(t, err)
				remote = new(mcp.CallToolResult)
				require.NoError(t, json.Unmarshal(wire, remote))
			}
			var stdout bytes.Buffer
			tool, err := New("screenshot", WithCallback(createToolCallback(&imageMCPClient{result: remote}, "screenshot", "screen-server")))
			require.NoError(t, err)
			result, err := tool.InvokeWithParams(map[string]any{}, WithStdout(&stdout),
				WithRuntimeConfig(&ToolRuntimeConfig{RuntimeID: "capture-1"}),
				WithResultCallback(func(execution *ToolExecutionResult) (*ToolResult, error) {
					return &ToolResult{Name: "screenshot", Success: true, Data: execution}, nil
				}))
			require.NoError(t, err)
			require.Len(t, result.Images, 1)
			data, err := result.Images[0].Bytes(context.Background())
			require.NoError(t, err)
			require.Equal(t, raw, data)
			require.Equal(t, "capture-1", result.Images[0].Source.ToolCallID)
			require.Equal(t, "mcp", result.Images[0].Source.Kind)
			require.Equal(t, "screen-server", result.Images[0].Source.ServerName)
			require.Contains(t, stdout.String(), "current screen")
			require.Contains(t, stdout.String(), "11x9")
			serialized, err := json.Marshal(result)
			require.NoError(t, err)
			for _, text := range []string{stdout.String(), result.String(), result.Dump(), string(serialized)} {
				require.NotContains(t, text, encoded)
				require.NotContains(t, text, `"data":"iVBOR`)
			}
		})
	}
}

func TestMCPImageValidationAndTextCompatibility(t *testing.T) {
	ctx := context.Background()
	encoded := base64.StdEncoding.EncodeToString(mediaPNG(t))
	for _, content := range [][]any{
		{map[string]any{"type": "image", "mimeType": "image/png", "data": "invalid-secret!"}},
		{mcp.NewImageContent(encoded, "image/jpeg")},
		{(*mcp.ImageContent)(nil)},
	} {
		converted, err := convertMCPToolContent(ctx, content, aimedia.Source{})
		require.Error(t, err)
		require.Empty(t, converted.blocks)
		require.Empty(t, converted.text)
		require.NotContains(t, err.Error(), "invalid-secret")
	}
	var many []any
	for n := 0; n <= MaxToolImages; n++ {
		many = append(many, mcp.NewImageContent(encoded, "image/png"))
	}
	_, err := convertMCPToolContent(ctx, many, aimedia.Source{})
	require.ErrorContains(t, err, "image limit")
	converted, err := convertMCPToolContent(ctx, []any{mcp.NewTextContent("hello"), map[string]any{"type": "text", "text": " world"}}, aimedia.Source{})
	require.NoError(t, err)
	require.Equal(t, "hello world", converted.text)
	require.Empty(t, converted.images)

	// A remote screenshot server's filename must never cause the agent process
	// to read a coincidentally named local file.
	fileInfo := `{"delivery":"file","path":"/remote-engine/private.png","storageLocation":"mcp_engine"}`
	converted, err = convertMCPToolContent(ctx, []any{mcp.NewTextContent(fileInfo)}, aimedia.Source{})
	require.NoError(t, err)
	require.Equal(t, fileInfo, converted.text)
}

func TestImageResultUsesExplicitMediaChannelAndCancellation(t *testing.T) {
	ctx := context.Background()
	raw := mediaPNG(t)
	media, err := ImageResult(ctx, raw, "image/png", map[string]any{"operation": "screenshot"})
	require.NoError(t, err)
	tool, err := New("image-tool", WithSimpleCallback(func(InvokeParams, io.Writer, io.Writer) (any, error) {
		return media, nil
	}))
	require.NoError(t, err)
	result, err := tool.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, result.Images, 1)
	require.Equal(t, media.Result, result.Data.(*ToolExecutionResult).Result)
	require.Contains(t, result.String(), "images:")
	require.NotContains(t, result.String(), base64.StdEncoding.EncodeToString(raw))
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = tool.InvokeWithParams(map[string]any{}, WithContext(canceled))
	require.ErrorIs(t, err, context.Canceled)
}

func TestMCPImagesThroughRealHTTPTransport(t *testing.T) {
	ctx := context.Background()
	raw := mediaPNG(t)
	encoded := base64.StdEncoding.EncodeToString(raw)
	const state = `{"state_id":"fresh","window_id":9007199254740993,"related_windows":[{"image_content_index":3}]}`
	remote := server.NewMCPServer("image-fixture", "1")
	remote.AddTool(mcp.NewTool("capture"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := mcp.NewToolResultImage("captured via HTTP", encoded, "image/png")
		result.Content = append(result.Content, mcp.NewTextContent("related popup"), mcp.NewImageContent(encoded, "image/png"))
		result.StructuredContent = json.RawMessage(state)
		return result, nil
	})
	remote.AddTool(mcp.NewTool("inspect"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []any{mcp.NewTextContent("no capture")}, StructuredContent: json.RawMessage(`{"state_id":"text-only"}`)}, nil
	})
	handler := server.NewStreamableHTTPServer(remote, "")
	mux := http.NewServeMux()
	handler.RegisterHandlers(mux)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	defer handler.Shutdown(ctx)
	tools, err := LoadAIToolsFromMCPServer(ctx, &schema.MCPServer{
		Name: "image-fixture", Type: "streamable_http", URL: httpServer.URL + "/mcp",
	}, nil)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	defer tools[0].BridgeMCPClient.Close()
	byName := make(map[string]*Tool)
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	result, err := byName["mcp_image-fixture_capture"].InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, result.Images, 2)
	data, err := result.Images[0].Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, raw, data)
	execution := result.Data.(*ToolExecutionResult)
	envelope := execution.Result.(*mcp.CallToolResult)
	require.Equal(t, state, string(envelope.StructuredContent))
	require.Len(t, envelope.Content, 4)
	require.Equal(t, result.Images[0].ID, envelope.Content[1].(map[string]any)["image_id"])
	require.Equal(t, 0, envelope.Content[1].(map[string]any)["image_index"])
	require.Equal(t, result.Images[1].ID, envelope.Content[3].(map[string]any)["image_id"])
	require.Equal(t, 1, envelope.Content[3].(map[string]any)["image_index"])
	require.NotContains(t, execution.Stdout, encoded)
	require.Contains(t, result.String(), "captured via HTTP")
	require.Contains(t, result.String(), `"state_id":"fresh"`)
	require.NotContains(t, result.String(), encoded)
	textOnly, err := byName["mcp_image-fixture_inspect"].InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Empty(t, textOnly.Images)
	require.Equal(t, `{"state_id":"text-only"}`, string(textOnly.Data.(*ToolExecutionResult).Result.(*mcp.CallToolResult).StructuredContent))
}

func TestMCPImageResourcesAreReadFromTheOriginServer(t *testing.T) {
	ctx := context.Background()
	encoded := base64.StdEncoding.EncodeToString(mediaPNG(t))
	uri := "image-fixture://captures/one"
	var reads atomic.Int32
	remote := server.NewMCPServer("resource-fixture", "1", server.WithResourceCapabilities(true, true))
	remote.AddResource(mcp.NewResource(uri, "capture"), func(_ context.Context, request mcp.ReadResourceRequest) ([]interface{}, error) {
		reads.Add(1)
		if request.Params.URI != uri {
			return nil, fmt.Errorf("unexpected resource URI")
		}
		return []interface{}{mcp.BlobResourceContents{ResourceContents: mcp.ResourceContents{URI: uri, MIMEType: "image/png"}, Blob: encoded}}, nil
	})
	remote.AddTool(mcp.NewTool("capture"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := json.Marshal(map[string]any{"delivery": "file", "path": "/remote-engine/not-a-local-file.png", "resourceUri": uri, "mimeType": "image/png"})
		return &mcp.CallToolResult{
			Content:           []any{mcp.NewTextContent(string(raw)), mcp.NewImageContent(encoded, "image/png")},
			StructuredContent: json.RawMessage(`{"image_content_index":1}`),
		}, nil
	})
	handler := server.NewStreamableHTTPServer(remote, "")
	mux := http.NewServeMux()
	handler.RegisterHandlers(mux)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	defer handler.Shutdown(ctx)
	tools, err := LoadAIToolsFromMCPServer(ctx, &schema.MCPServer{Name: "remote-resource", Type: "streamable_http", URL: httpServer.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer tools[0].BridgeMCPClient.Close()
	result, err := tools[0].InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, result.Images, 2)
	envelope := result.Data.(*ToolExecutionResult).Result.(*mcp.CallToolResult)
	require.Len(t, envelope.Content, 2)
	require.Equal(t, 1, envelope.Content[1].(map[string]any)["image_index"])
	require.Equal(t, int32(1), reads.Load())
	require.NotContains(t, result.String(), encoded)
}
