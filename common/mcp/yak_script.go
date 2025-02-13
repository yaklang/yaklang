package mcp

import (
	"context"
	"encoding/json"

	"github.com/mitchellh/mapstructure"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *MCPServer) handleQueryYakScriptTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var req ypb.QueryYakScriptRequest
	err := mapstructure.Decode(request.Params.Arguments, &req)
	if err != nil {
		return nil, utils.Wrap(err, "invalid argument")
	}
	rsp, err := s.grpcClient.QueryYakScript(ctx, &req)
	if err != nil {
		return nil, utils.Wrap(err, "failed to query yak script")
	}
	rspBytes, err := json.Marshal(rsp.Data)
	if err != nil {
		return nil, utils.Wrap(err, "failed to marshal response")
	}
	return &mcp.CallToolResult{
		Content: []interface{}{
			mcp.TextContent{
				Type: "text",
				Text: "Result:",
			},
			mcp.TextContent{
				Type: "text",
				Text: string(rspBytes),
			},
		},
	}, nil
}
