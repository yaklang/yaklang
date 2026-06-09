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
	AddGlobalToolSet("global_hotpatch",
		WithTool(
			mcp.NewTool("get_global_hotpatch_config",
				mcp.WithDescription("Get current global hotpatch (全局热加载) configuration, including enabled state, version and active template"),
			),
			handleGetGlobalHotPatchConfig,
		),
		WithTool(
			mcp.NewTool("enable_global_hotpatch",
				mcp.WithDescription("Enable global hotpatch (全局热加载) with a HotPatchTemplate. Takes effect on new MITM requests and WebFuzzer tasks."),
				mcp.WithString("templateName",
					mcp.Description("Name of the global HotPatchTemplate to enable"),
					mcp.Required(),
				),
				mcp.WithString("templateType",
					mcp.Description("Template type, should be \"global\""),
					mcp.Default("global"),
				),
				mcp.WithNumber("expectedVersion",
					mcp.Description("Optional optimistic lock version from get_global_hotpatch_config; 0 means skip"),
				),
			),
			handleEnableGlobalHotPatch,
		),
		WithTool(
			mcp.NewTool("disable_global_hotpatch",
				mcp.WithDescription("Disable global hotpatch (全局热加载). New MITM requests and WebFuzzer tasks will no longer run the global layer."),
				mcp.WithNumber("expectedVersion",
					mcp.Description("Optional optimistic lock version from get_global_hotpatch_config; 0 means skip"),
				),
			),
			handleDisableGlobalHotPatch,
		),
		WithTool(
			mcp.NewTool("reset_global_hotpatch_config",
				mcp.WithDescription("Reset global hotpatch (全局热加载) to default disabled state and clear active templates"),
			),
			handleResetGlobalHotPatchConfig,
		),
		WithTool(
			mcp.NewTool("query_hotpatch_template_list",
				mcp.WithDescription("List available HotPatchTemplate names, optionally filtered by type (fuzzer/mitm/httpflow-analyze/global)"),
				mcp.WithString("type",
					mcp.Description("Template type filter"),
					mcp.Enum("fuzzer", "mitm", "httpflow-analyze", "global"),
				),
			),
			handleQueryHotPatchTemplateList,
		),
	)
}

func handleGetGlobalHotPatchConfig(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rsp, err := s.grpcClient.GetGlobalHotPatchConfig(ctx, &ypb.Empty{})
		if err != nil {
			return nil, utils.Wrap(err, "failed to get global hotpatch config")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleEnableGlobalHotPatch(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			TemplateName    string `mapstructure:"templateName"`
			TemplateType    string `mapstructure:"templateType"`
			ExpectedVersion int64  `mapstructure:"expectedVersion"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		if args.TemplateName == "" {
			return nil, utils.Error("templateName is required")
		}
		templateType := args.TemplateType
		if templateType == "" {
			templateType = "global"
		}

		req := &ypb.SetGlobalHotPatchConfigRequest{
			ExpectedVersion: args.ExpectedVersion,
			Config: &ypb.GlobalHotPatchConfig{
				Enabled: true,
				Items: []*ypb.GlobalHotPatchTemplateRef{{
					Name:    args.TemplateName,
					Type:    templateType,
					Enabled: true,
				}},
			},
		}
		rsp, err := s.grpcClient.SetGlobalHotPatchConfig(ctx, req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to enable global hotpatch")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleDisableGlobalHotPatch(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ExpectedVersion int64 `mapstructure:"expectedVersion"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}

		req := &ypb.SetGlobalHotPatchConfigRequest{
			ExpectedVersion: args.ExpectedVersion,
			Config: &ypb.GlobalHotPatchConfig{
				Enabled: false,
			},
		}
		rsp, err := s.grpcClient.SetGlobalHotPatchConfig(ctx, req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to disable global hotpatch")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleResetGlobalHotPatchConfig(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rsp, err := s.grpcClient.ResetGlobalHotPatchConfig(ctx, &ypb.Empty{})
		if err != nil {
			return nil, utils.Wrap(err, "failed to reset global hotpatch config")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleQueryHotPatchTemplateList(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Type string `mapstructure:"type"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}

		rsp, err := s.grpcClient.QueryHotPatchTemplateList(ctx, &ypb.QueryHotPatchTemplateListRequest{
			Type: args.Type,
		})
		if err != nil {
			return nil, utils.Wrap(err, "failed to query hotpatch template list")
		}
		return NewCommonCallToolResult(rsp)
	}
}
