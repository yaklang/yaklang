package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var ysoToolOptions = []mcp.ToolOption{
	mcp.WithString("gadget", mcp.Description("YSO gadget chain name (default URLDNS)")),
	mcp.WithString("class", mcp.Description("YSO class name")),
}

func init() {
	AddGlobalToolSet("yso",
		WithTool(mcp.NewTool("get_all_yso_gadget_options",
			mcp.WithDescription("List all available YSO gadget chain options"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
		}, "failed to get yso gadget options")),

		WithTool(mcp.NewTool("get_all_yso_class_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List YSO class options for a given gadget chain"),
			}, ysoToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassOptions(ctx, req)
		}, "failed to get yso class options")),

		WithTool(mcp.NewTool("get_all_yso_class_generater_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List YSO class generator options"),
			}, ysoToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassGeneraterOptions(ctx, req)
		}, "failed to get yso class generator options")),

		WithTool(mcp.NewTool("generate_yso_code",
			append([]mcp.ToolOption{
				mcp.WithDescription("Generate YSO exploit code (Java source)"),
			}, ysoToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoCode(ctx, req)
		}, "failed to generate yso code")),

		WithTool(mcp.NewTool("generate_yso_bytes",
			append([]mcp.ToolOption{
				mcp.WithDescription("Generate YSO serialized bytes payload"),
			}, ysoToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoBytes(ctx, req)
		}, "failed to generate yso bytes")),

		WithTool(mcp.NewTool("yso_dump",
			mcp.WithDescription("Dump/analyze YSO serialized object bytes"),
			mcp.WithString("data", mcp.Description("Base64 or raw serialized bytes")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoBytesObject) (any, error) {
			return s.grpcClient.YsoDump(ctx, req)
		}, "failed to dump yso object")),
	)
}
