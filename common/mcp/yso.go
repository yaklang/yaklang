package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("yso",
		WithTool(mcp.NewTool("get_all_yso_gadget_options",
			mcp.WithDescription("List all available YSO gadget chain options"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
		}, "failed to get yso gadget options")),

		WithTool(mcp.NewTool("get_all_yso_class_options",
			mcp.WithDescription("List YSO class options for a given gadget chain"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("YSO options request with gadget and class filters"),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassOptions(ctx, req)
		}, "failed to get yso class options")),

		WithTool(mcp.NewTool("get_all_yso_class_generater_options",
			mcp.WithDescription("List YSO class generator options"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("YSO options request"),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassGeneraterOptions(ctx, req)
		}, "failed to get yso class generator options")),

		WithTool(mcp.NewTool("generate_yso_code",
			mcp.WithDescription("Generate YSO exploit code (Java source)"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("YSO generation options (gadget, class, params)"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoCode(ctx, req)
		}, "failed to generate yso code")),

		WithTool(mcp.NewTool("generate_yso_bytes",
			mcp.WithDescription("Generate YSO serialized bytes payload"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("YSO generation options"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoBytes(ctx, req)
		}, "failed to generate yso bytes")),

		WithTool(mcp.NewTool("yso_dump",
			mcp.WithDescription("Dump/analyze YSO serialized object bytes"),
			mcp.WithStruct("object", []mcp.PropertyOption{
				mcp.Description("YSO bytes object to dump"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoBytesObject) (any, error) {
			return s.grpcClient.YsoDump(ctx, req)
		}, "failed to dump yso object")),
	)
}
