package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var ysoGadgetClassToolOptions = []mcp.ToolOption{
	mcp.WithString("gadget", mcp.Description("YSO gadget chain; list via get_all_yso_gadget_options (default URLDNS)")),
	mcp.WithString("class", mcp.Description("YSO payload class; list via get_all_yso_class_options for the gadget")),
}

var ysoGeneratorOptionsToolOptions = []mcp.ToolOption{
	mcp.WithStructArray("options", []mcp.PropertyOption{
		mcp.Description("Class generator params; keys from get_all_yso_class_generater_options"),
	},
		mcp.WithString("key", mcp.Description("Generator option key, e.g. domain, cmd")),
		mcp.WithString("value", mcp.Description("Generator option value")),
		mcp.WithString("type", mcp.Description("Option type from generator metadata")),
	),
}

func init() {
	AddGlobalToolSet("yso",
		WithTool(mcp.NewTool("get_all_yso_gadget_options",
			mcp.WithDescription("List available YSO gadget chain names"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
		}, "failed to get yso gadget options")),

		WithTool(mcp.NewTool("get_all_yso_class_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List YSO payload classes for a gadget chain"),
			}, ysoGadgetClassToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassOptions(ctx, req)
		}, "failed to get yso class options")),

		WithTool(mcp.NewTool("get_all_yso_class_generater_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List generator option keys for gadget+class pair"),
			}, ysoGadgetClassToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassGeneraterOptions(ctx, req)
		}, "failed to get yso class generator options")),

		WithTool(mcp.NewTool("generate_yso_bytes",
			append([]mcp.ToolOption{
				mcp.WithDescription("Generate YSO serialized Java object bytes"),
			}, append(ysoGadgetClassToolOptions, ysoGeneratorOptionsToolOptions...)...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoBytes(ctx, req)
		}, "failed to generate yso bytes")),

		WithTool(mcp.NewTool("yso_dump",
			mcp.WithDescription("Deserialize and inspect YSO/Java serialized bytes"),
			mcp.WithString("data", mcp.Description("Base64-encoded serialized bytes from generate_yso_bytes")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoBytesObject) (any, error) {
			return s.grpcClient.YsoDump(ctx, req)
		}, "failed to dump yso object")),
	)
}
