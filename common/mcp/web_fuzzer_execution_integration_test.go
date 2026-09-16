package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExecuteWebFuzzerTabWaitsForThePushedExecution(t *testing.T) {
	yakit.CallPostInitDatabase()
	srv, err := mcp.NewMCPServer(mcp.WithEnableAllToolSets(), mcp.WithDatabaseProvider(nil, func() *gorm.DB {
		return consts.GetGormProjectDatabase()
	}))
	require.NoError(t, err)
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)

	pageID := "mcp-execute-" + uuid.NewString()
	_, err = mcp.InvokeBuiltinTool(context.Background(), srv, "create_web_fuzzer_tab", map[string]any{
		"pageId":  pageID,
		"request": "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = mcp.InvokeBuiltinTool(context.Background(), srv, "delete_web_fuzzer_tabs", map[string]any{"pageIds": []string{pageID}})
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)
	stream, err := client.DuplexConnection(ctx)
	require.NoError(t, err)
	pushes := make(chan *ypb.DuplexConnectionResponse, 1)
	go func() {
		for {
			response, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			if response.GetMessageType() == yakit.ServerPushType_WebFuzzerExecution {
				pushes <- response
				return
			}
		}
	}()

	type executionResult struct {
		result *rawmcp.CallToolResult
		err    error
	}
	completed := make(chan executionResult, 1)
	go func() {
		result, callErr := mcp.InvokeBuiltinTool(context.Background(), srv, "execute_web_fuzzer_tab", map[string]any{
			"pageId": pageID,
		})
		completed <- executionResult{result: result, err: callErr}
	}()

	var push yakit.WebFuzzerExecutionPush
	select {
	case response := <-pushes:
		require.NoError(t, json.Unmarshal(response.GetData(), &push))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Web Fuzzer execution push")
	}
	require.Equal(t, pageID, push.PageID)
	require.NotEmpty(t, push.ExecutionID)
	task := &schema.WebFuzzerTask{
		FuzzerIndex:          push.ExecutionID,
		FuzzerTabIndex:       pageID,
		HTTPFlowTotal:        1,
		HTTPFlowSuccessCount: 1,
		Ok:                   true,
		Reason:               "normal exit",
	}
	require.NoError(t, db.Create(task).Error)
	t.Cleanup(func() { _ = db.Delete(task).Error })

	select {
	case call := <-completed:
		require.NoError(t, call.err)
		payload := decodeToolResult(t, call.result)
		require.Equal(t, "completed", payload["operation"])
		require.EqualValues(t, task.ID, payload["taskId"])
		require.Equal(t, push.ExecutionID, payload["executionId"])
	case <-time.After(2 * time.Second):
		t.Fatal("execute_web_fuzzer_tab did not return after the matching task was persisted")
	}
}

func decodeToolResult(t *testing.T, result *rawmcp.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(rawmcp.TextContent)
	require.True(t, ok)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	return payload
}
