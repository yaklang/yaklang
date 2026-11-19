package aireact

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReActMCPFailureReachesTimelineWithRecoveryImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 13, 8))))
	encoded := base64.StdEncoding.EncodeToString(raw.Bytes())
	remote := server.NewMCPServer("desktop-fixture", "1")
	remote.AddTool(mcp.NewTool("click"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reply := mcp.NewToolResultImage("Popup closed before input", encoded, "image/png")
		reply.Content = append(reply.Content, mcp.NewTextContent("resume_from 23"))
		reply.StructuredContent = json.RawMessage(`{"state_id":"fresh","task":{"checkpoint":{"id":23}}}`)
		reply.IsError = true
		return reply, nil
	})
	handler := server.NewStreamableHTTPServer(remote, "")
	mux := http.NewServeMux()
	handler.RegisterHandlers(mux)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	defer handler.Shutdown(context.Background())
	tools, err := aitool.LoadAIToolsFromMCPServer(ctx, &schema.MCPServer{
		Name: "desktop-fixture", Type: "streamable_http", URL: httpServer.URL + "/mcp",
	}, nil)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	tool := tools[0]
	defer tool.BridgeMCPClient.Close()

	toolErrors := make(chan string, 4)
	react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithImageInputMode("native", true), aicommon.WithTools(tool),
		aicommon.WithDisallowMCPServers(false), aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event != nil && event.Type == schema.EVENT_TOOL_CALL_ERROR {
				toolErrors <- string(event.Content)
			}
		}))
	require.NoError(t, err)
	result, directlyAnswer, err := react.ExecuteToolRequiredAndCallWithoutRequired(ctx, tool.Name, aitool.InvokeParams{})
	require.NoError(t, err, "Agent receives failed tool calls as inspectable results")
	require.False(t, directlyAnswer)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "Popup closed before input")
	require.Len(t, result.Images, 1)
	require.FileExists(t, result.Images[0].Path)
	prompt := react.config.Timeline.DumpRecentOrdinaryWithImages(20000, false)
	require.Contains(t, prompt.Text, "resume_from 23")
	require.Contains(t, prompt.Text, `"state_id": "fresh"`)
	require.Contains(t, prompt.Text, `"isError": true`)
	require.NotContains(t, prompt.Text, encoded)
	require.Len(t, prompt.Images, 1)
	recovered, err := prompt.Images[0].Image.Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, raw.Bytes(), recovered)
	select {
	case message := <-toolErrors:
		require.Contains(t, message, "Popup closed before input")
	case <-ctx.Done():
		t.Fatal("MCP failure did not emit tool_call_error")
	}
}
