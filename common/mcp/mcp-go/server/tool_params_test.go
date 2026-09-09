package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestToolCallInvalidParameterTypes(t *testing.T) {
	srv := NewMCPServer("parameter-test", "1")
	srv.AddTool(mcp.NewTool("test-tool"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("invalid parameters must not reach the tool handler")
		return nil, nil
	})
	for _, params := range []string{
		`{"name":"test-tool","arguments":[]}`,
		`{"name":"test-tool","arguments":"invalid"}`,
		`{"name":42,"arguments":{}}`,
		`"invalid"`,
	} {
		t.Run(params, func(t *testing.T) {
			response := srv.HandleMessage(context.Background(), []byte(
				`{"jsonrpc":"2.0","id":"invalid-params","method":"tools/call","params":`+params+`}`,
			))
			failure, ok := response.(mcp.JSONRPCError)
			require.True(t, ok)
			require.Equal(t, "invalid-params", failure.ID)
			require.Equal(t, mcp.INVALID_PARAMS, failure.Error.Code)
		})
	}

	// Parameter error classification must not change malformed JSON or version errors.
	for _, tt := range []struct {
		message string
		code    int
	}{
		{`{"jsonrpc":`, mcp.PARSE_ERROR},
		{`{"jsonrpc":"1.0","id":1,"method":"tools/call","params":{}}`, mcp.INVALID_REQUEST},
	} {
		failure, ok := srv.HandleMessage(context.Background(), []byte(tt.message)).(mcp.JSONRPCError)
		require.True(t, ok)
		require.Equal(t, tt.code, failure.Error.Code)
	}
}
