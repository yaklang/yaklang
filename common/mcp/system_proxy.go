package mcp

import (
	"context"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("system_proxy",
		WithTool(
			mcp.NewTool("get_system_proxy",
				mcp.WithDescription("Get system proxy"),
			),
			handleGetSystemProxy,
		),
		WithTool(
			mcp.NewTool("set_system_proxy",
				mcp.WithDescription("Get system proxy"),
				mcp.WithString("httpProxy", mcp.Description("Proxy address"), mcp.Required()),
				mcp.WithBool("enable", mcp.Description("Enable or disable proxy"), mcp.Default(true), mcp.Required()),
			),
			handleSetSystemProxy,
		),
	)
}

func handleGetSystemProxy(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		rsp, err := s.grpcClient.GetSystemProxy(ctx, &ypb.Empty{})
		if err != nil {
			return nil, utils.Wrap(err, "failed to get system proxy")
		}
		return NewCommonCallToolResult((rsp))
	}
}

func handleSetSystemProxy(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.SetSystemProxyRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.SetSystemProxy(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to set system proxy")
		}
		return NewCommonCallToolResult((rsp))
	}
}
