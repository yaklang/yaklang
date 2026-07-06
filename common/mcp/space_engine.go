package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("space_engine",
		WithTool(mcp.NewTool("get_space_engine_status",
			mcp.WithDescription("Get space engine (FOFA/Hunter/Quake etc.) status"),
			mcp.WithStruct("request", []mcp.PropertyOption{mcp.Description("Space engine status request")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.GetSpaceEngineStatusRequest) (any, error) {
			return s.grpcClient.GetSpaceEngineStatus(ctx, req)
		}, "failed to get space engine status")),

		WithTool(mcp.NewTool("get_space_engine_account_status_v2",
			mcp.WithDescription("Get space engine account status with API config"),
			mcp.WithStruct("config", []mcp.PropertyOption{
				mcp.Description("Third party application config"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.ThirdPartyApplicationConfig) (any, error) {
			return s.grpcClient.GetSpaceEngineAccountStatusV2(ctx, req)
		}, "failed to get space engine account status")),

		WithTool(mcp.NewTool("fetch_port_asset_from_space_engine",
			mcp.WithDescription("Fetch port assets from space engine (runs in background)"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Fetch request with engine type and query"),
				mcp.Required(),
			}),
		), handleFetchPortAssetFromSpaceEngine),
	)
}

func handleFetchPortAssetFromSpaceEngine(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.FetchPortAssetFromSpaceEngineRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"type": req.GetType(),
		}
		return startBackgroundExecStream(s, "fetch_port_asset_from_space_engine", summary, func(bgCtx context.Context) (execResultReceiver, error) {
			return s.grpcClient.FetchPortAssetFromSpaceEngine(bgCtx, &req)
		})
	}
}
