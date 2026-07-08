package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var ysoGadgetClassToolOptions = []mcp.ToolOption{
	mcp.WithString("gadget", mcp.Description("Gadget chain name from get_all_yso_gadget_options, e.g. URLDNS, CommonsCollections1")),
	mcp.WithString("class", mcp.Description("Payload class for the gadget from get_all_yso_class_options, e.g. URLDNS, RuntimeExec")),
}

var ysoGeneratorOptionsToolOptions = []mcp.ToolOption{
	mcp.WithStructArray("options", []mcp.PropertyOption{
		mcp.Description("Class generator params; keys from get_all_yso_class_generater_options"),
	},
		mcp.WithString("key", mcp.Description("Option key from get_all_yso_class_generater_options, e.g. domain, cmd, file")),
		mcp.WithString("value", mcp.Description("Option value, e.g. DNSLog domain or shell command")),
		mcp.WithString("type", mcp.Description("Option type from generator metadata")),
	),
}

func init() {
	AddGlobalToolSet("yso",
		WithTool(mcp.NewTool("get_all_yso_gadget_options",
			mcp.WithDescription("List YSO Java deserialization gadget chain names; first step before class/generator selection"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
		}, "failed to get yso gadget options")),

		WithTool(mcp.NewTool("get_all_yso_class_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List payload classes for a gadget chain; call after get_all_yso_gadget_options"),
			}, ysoGadgetClassToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassOptions(ctx, req)
		}, "failed to get yso class options")),

		WithTool(mcp.NewTool("get_all_yso_class_generater_options",
			append([]mcp.ToolOption{
				mcp.WithDescription("List required generator keys/types for gadget+class; call before generate_yso_bytes options"),
			}, ysoGadgetClassToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GetAllYsoClassGeneraterOptions(ctx, req)
		}, "failed to get yso class generator options")),

		WithTool(mcp.NewTool("generate_yso_bytes",
			append([]mcp.ToolOption{
				mcp.WithDescription("Generate Java serialized exploit bytes (base64 in response). Workflow: gadget options → class options → generator options → this tool → yso_dump"),
			}, append(ysoGadgetClassToolOptions, ysoGeneratorOptionsToolOptions...)...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoOptionsRequerstWithVerbose) (any, error) {
			return s.grpcClient.GenerateYsoBytes(ctx, req)
		}, "failed to generate yso bytes")),

		WithTool(mcp.NewTool("yso_dump",
			mcp.WithDescription("Parse and inspect Java serialization structure from generate_yso_bytes output; useful to verify gadget chain before sending"),
			mcp.WithString("data", mcp.Description("Base64 Bytes field from generate_yso_bytes response")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YsoBytesObject) (any, error) {
			return s.grpcClient.YsoDump(ctx, req)
		}, "failed to dump yso object")),
	)
}
