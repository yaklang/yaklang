package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("plugin_env",
		WithTool(mcp.NewTool("get_all_plugin_env",
			mcp.WithDescription("Get all plugin environment variables"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllPluginEnv(ctx, &ypb.Empty{})
		}, "failed to get all plugin env")),

		WithTool(mcp.NewTool("query_plugin_env",
			mcp.WithDescription("Query plugin environment variables by key"),
			mcp.WithStruct("request", []mcp.PropertyOption{mcp.Description("Query parameters")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryPluginEnvRequest) (any, error) {
			return s.grpcClient.QueryPluginEnv(ctx, req)
		}, "failed to query plugin env")),

		WithTool(mcp.NewTool("set_plugin_env",
			mcp.WithDescription("Set plugin environment variables"),
			mcp.WithStruct("data", []mcp.PropertyOption{
				mcp.Description("Plugin env key-value data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.PluginEnvData) (any, error) {
			_, err := s.grpcClient.SetPluginEnv(ctx, req)
			if err != nil {
				return nil, err
			}
			return "set plugin env success", nil
		}, "failed to set plugin env")),

		WithTool(mcp.NewTool("create_plugin_env",
			mcp.WithDescription("Create plugin environment variables"),
			mcp.WithStruct("data", []mcp.PropertyOption{
				mcp.Description("Plugin env key-value data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.PluginEnvData) (any, error) {
			_, err := s.grpcClient.CreatePluginEnv(ctx, req)
			if err != nil {
				return nil, err
			}
			return "create plugin env success", nil
		}, "failed to create plugin env")),

		WithTool(mcp.NewTool("delete_plugin_env",
			mcp.WithDescription("Delete plugin environment variables"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Delete request with keys"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeletePluginEnvRequest) (any, error) {
			_, err := s.grpcClient.DeletePluginEnv(ctx, req)
			if err != nil {
				return nil, err
			}
			return "delete plugin env success", nil
		}, "failed to delete plugin env")),
	)
}
