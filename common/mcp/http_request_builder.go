package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("http_builder",
		WithTool(mcp.NewTool("http_request_builder",
			mcp.WithDescription("Build HTTP requests from parameters (method, path, headers, body)"),
			mcp.WithStruct("params", []mcp.PropertyOption{
				mcp.Description("HTTP request builder parameters"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.HTTPRequestBuilderParams) (any, error) {
			return s.grpcClient.HTTPRequestBuilder(ctx, req)
		}, "failed to build http request")),

		WithTool(mcp.NewTool("debug_plugin",
			mcp.WithDescription("Execute a Yak plugin in debug mode"),
			mcp.WithString("pluginName", mcp.Description("Plugin name"), mcp.Required()),
			mcp.WithString("pluginType", mcp.Description("Plugin type")),
			mcp.WithStruct("execParams", []mcp.PropertyOption{mcp.Description("Plugin execution parameters as key-value pairs")}),
		), handleDebugPlugin),
	)
}

func handleDebugPlugin(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.DebugPluginRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		stream, err := s.grpcClient.DebugPlugin(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to debug plugin")
		}
		results, err := collectExecResultStream(ctx, s, stream, collectExecStreamOptions{})
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] debug plugin completed with no output",
			})
		}
		return NewCommonCallToolResult(results)
	}
}
