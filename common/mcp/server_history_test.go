package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	mcpsdk "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMCPToolCallHistoryFromMockServer(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	require.NoError(t, db.AutoMigrate(&schema.MCPToolCallHistory{}).Error)
	defer func() {
		require.NoError(t, db.Unscoped().Where("id > 0").Delete(&schema.MCPToolCallHistory{}).Error)
	}()

	srv, err := NewMCPServer(WithDatabases(db, nil))
	require.NoError(t, err)
	defer srv.Close()
	srv.server.AddTool(mcpsdk.NewTool("mock_success"), func(
		_ context.Context,
		_ mcpsdk.CallToolRequest,
	) (*mcpsdk.CallToolResult, error) {
		return mcpsdk.NewToolResultText("success-result"), nil
	})
	srv.server.AddTool(mcpsdk.NewTool("mock_error_result"), func(
		_ context.Context,
		_ mcpsdk.CallToolRequest,
	) (*mcpsdk.CallToolResult, error) {
		return mcpsdk.NewToolResultError("mock tool rejected input"), nil
	})
	srv.server.AddTool(mcpsdk.NewTool("mock_handler_error"), func(
		_ context.Context,
		_ mcpsdk.CallToolRequest,
	) (*mcpsdk.CallToolResult, error) {
		return nil, errors.New("mock handler failed")
	})
	srv.server.AddTool(mcpsdk.NewTool("mock_empty_result"), func(
		_ context.Context,
		_ mcpsdk.CallToolRequest,
	) (*mcpsdk.CallToolResult, error) {
		return nil, nil
	})

	ctx := srv.server.WithContext(context.Background(), mcpserver.NotificationContext{
		ClientID:  "mock-client-id",
		SessionID: "mock-session-id",
	})
	initialize := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcpsdk.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "Mock Agent",
				"version": "1.2.3",
			},
		},
	}
	initializeJSON, err := json.Marshal(initialize)
	require.NoError(t, err)
	require.NotNil(t, srv.server.HandleMessage(ctx, initializeJSON))

	callTool := func(id int, name, testCase string) {
		t.Helper()
		request := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "tools/call",
			"params": map[string]any{
				"name": name,
				"arguments": map[string]any{
					"case": testCase,
				},
			},
		}
		requestJSON, marshalErr := json.Marshal(request)
		require.NoError(t, marshalErr)
		require.NotNil(t, srv.server.HandleMessage(ctx, requestJSON))
	}

	callTool(2, "mock_success", "success")
	callTool(3, "mock_error_result", "error-result")
	callTool(4, "mock_handler_error", "handler-error")
	callTool(5, "mock_empty_result", "empty-result")

	paginator, histories, err := yakit.QueryMCPToolCallHistories(db, &ypb.QueryMCPToolCallHistoryRequest{
		Keyword: "Mock Agent",
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 10,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 4, paginator.TotalRecord)
	require.Len(t, histories, 4)

	byTool := make(map[string]*schema.MCPToolCallHistory, len(histories))
	for _, summary := range histories {
		// The list query must not load potentially large payload fields.
		require.Empty(t, summary.Arguments)
		require.Empty(t, summary.Result)
		require.Equal(t, "Mock Agent", summary.ClientName)
		require.Equal(t, "1.2.3", summary.ClientVersion)
		require.Equal(t, "mock-client-id", summary.ClientID)
		require.Equal(t, "mock-session-id", summary.SessionID)
		byTool[summary.ToolName] = summary
	}

	require.True(t, byTool["mock_success"].Success)
	require.False(t, byTool["mock_error_result"].Success)
	require.Equal(t, "mock tool rejected input", byTool["mock_error_result"].ErrorMessage)
	require.False(t, byTool["mock_handler_error"].Success)
	require.Equal(t, "mock handler failed", byTool["mock_handler_error"].ErrorMessage)
	require.False(t, byTool["mock_empty_result"].Success)
	require.Equal(t, "MCP tool returned an empty result", byTool["mock_empty_result"].ErrorMessage)

	detail, err := yakit.GetMCPToolCallHistory(db, int64(byTool["mock_success"].ID))
	require.NoError(t, err)
	require.JSONEq(t, `{"case":"success"}`, detail.Arguments)
	require.Contains(t, detail.Result, "success-result")
}
