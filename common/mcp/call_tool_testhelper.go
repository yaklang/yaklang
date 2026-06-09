package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

// CallBuiltinTool invokes a registered builtin MCP tool handler directly.
// It is intended for integration tests that need to exercise tool logic end-to-end.
func CallBuiltinTool(s *MCPServer, ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	tw := GetBuiltinToolByName(name)
	if tw == nil {
		return nil, utils.Errorf("builtin tool not found: %s", name)
	}
	if s == nil {
		return nil, utils.Error("mcp server is nil")
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	if arguments != nil {
		req.Params.Arguments = arguments
	}
	return tw.handler(s)(ctx, req)
}
