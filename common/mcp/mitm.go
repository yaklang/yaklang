package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("mitm",
		WithTool(mcp.NewTool("get_mitm_filter",
			mcp.WithDescription("Get MITM traffic filter configuration"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetMITMFilter(ctx, &ypb.Empty{})
		}, "failed to get mitm filter")),

		WithTool(mcp.NewTool("set_mitm_filter",
			mcp.WithDescription("Set MITM traffic filter configuration"),
			mcp.WithStruct("filterData", []mcp.PropertyOption{
				mcp.Description("MITM filter data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.SetMITMFilterRequest) (any, error) {
			return s.grpcClient.SetMITMFilter(ctx, req)
		}, "failed to set mitm filter")),

		WithTool(mcp.NewTool("reset_mitm_filter",
			mcp.WithDescription("Reset MITM traffic filter to defaults"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.ResetMITMFilter(ctx, &ypb.Empty{})
		}, "failed to reset mitm filter")),

		WithTool(mcp.NewTool("get_mitm_hijack_filter",
			mcp.WithDescription("Get MITM hijack filter configuration"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetMITMHijackFilter(ctx, &ypb.Empty{})
		}, "failed to get mitm hijack filter")),

		WithTool(mcp.NewTool("set_mitm_hijack_filter",
			mcp.WithDescription("Set MITM hijack filter configuration"),
			mcp.WithStruct("filterData", []mcp.PropertyOption{
				mcp.Description("MITM hijack filter data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.SetMITMFilterRequest) (any, error) {
			return s.grpcClient.SetMITMHijackFilter(ctx, req)
		}, "failed to set mitm hijack filter")),

		WithTool(mcp.NewTool("reset_mitm_hijack_filter",
			mcp.WithDescription("Reset MITM hijack filter to defaults"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.ResetMITMHijackFilter(ctx, &ypb.Empty{})
		}, "failed to reset mitm hijack filter")),

		WithTool(mcp.NewTool("query_mitm_replacer_rules",
			mcp.WithDescription("Query MITM content replacer rules"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "rule_name"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Replacer rule filter")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryMITMReplacerRulesRequest) (any, error) {
			return s.grpcClient.QueryMITMReplacerRules(ctx, req)
		}, "failed to query mitm replacer rules")),

		WithTool(mcp.NewTool("get_current_rules",
			mcp.WithDescription("Get currently active MITM replacer rules"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentRules(ctx, &ypb.Empty{})
		}, "failed to get current mitm rules")),

		WithTool(mcp.NewTool("set_current_rules",
			mcp.WithDescription("Set currently active MITM replacer rules"),
			mcp.WithStruct("rules", []mcp.PropertyOption{
				mcp.Description("MITM content replacers"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.MITMContentReplacers) (any, error) {
			_, err := s.grpcClient.SetCurrentRules(ctx, req)
			if err != nil {
				return nil, err
			}
			return "set current mitm rules success", nil
		}, "failed to set current mitm rules")),

		WithTool(mcp.NewTool("export_mitm_replacer_rules",
			mcp.WithDescription("Export all MITM replacer rules"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.ExportMITMReplacerRules(ctx, &ypb.Empty{})
		}, "failed to export mitm replacer rules")),

		WithTool(mcp.NewTool("import_mitm_replacer_rules",
			mcp.WithDescription("Import MITM replacer rules"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Import payload with rules data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.ImportMITMReplacerRulesRequest) (any, error) {
			_, err := s.grpcClient.ImportMITMReplacerRules(ctx, req)
			if err != nil {
				return nil, err
			}
			return "import mitm replacer rules success", nil
		}, "failed to import mitm replacer rules")),

		WithTool(mcp.NewTool("download_mitm_cert",
			mcp.WithDescription("Download MITM CA certificate"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.DownloadMITMCert(ctx, &ypb.Empty{})
		}, "failed to download mitm cert")),

		WithTool(mcp.NewTool("download_mitm_gm_cert",
			mcp.WithDescription("Download MITM GM/TLS certificate"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.DownloadMITMGMCert(ctx, &ypb.Empty{})
		}, "failed to download mitm gm cert")),

		WithTool(mcp.NewTool("install_mitm_certificate",
			mcp.WithDescription("Install MITM CA certificate to system trust store"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.InstallMITMCertificate(ctx, &ypb.Empty{})
		}, "failed to install mitm certificate")),

		WithTool(mcp.NewTool("query_mitm_extracted_aggregate",
			mcp.WithDescription("Query MITM extracted data aggregates"),
			mcp.WithStruct("request", []mcp.PropertyOption{mcp.Description("Aggregate query parameters")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryMITMExtractedAggregateRequest) (any, error) {
			return s.grpcClient.QueryMITMExtractedAggregate(ctx, req)
		}, "failed to query mitm extracted aggregate")),

		WithTool(mcp.NewTool("query_mitm_rule_extracted_data",
			mcp.WithDescription("Query MITM rule extracted data records"),
			mcp.WithStruct("request", []mcp.PropertyOption{mcp.Description("Extracted data query parameters")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryMITMRuleExtractedDataRequest) (any, error) {
			return s.grpcClient.QueryMITMRuleExtractedData(ctx, req)
		}, "failed to query mitm rule extracted data")),

		WithTool(mcp.NewTool("delete_mitm_rule_extracted_data",
			mcp.WithDescription("Delete MITM rule extracted data records"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Delete parameters"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteMITMRuleExtractedDataRequest) (any, error) {
			_, err := s.grpcClient.DeleteMITMRuleExtractedData(ctx, req)
			if err != nil {
				return nil, err
			}
			return "delete mitm rule extracted data success", nil
		}, "failed to delete mitm rule extracted data")),

		WithTool(mcp.NewTool("start_mitm_v2",
			mcp.WithDescription("Start MITM v2 proxy (runs in background, returns listen address)"),
			mcp.WithString("host", mcp.Description("Listen host"), mcp.Default("127.0.0.1")),
			mcp.WithNumber("port", mcp.Description("Listen port"), mcp.Required()),
			mcp.WithString("downstreamProxy", mcp.Description("Downstream proxy URL")),
			mcp.WithBool("enableHttp2", mcp.Description("Enable HTTP/2")),
			mcp.WithBool("filterWebsocket", mcp.Description("Filter websocket traffic")),
			mcp.WithNumber("maxContentLength", mcp.Description("Max content length")),
			mcp.WithBool("disableSystemProxy", mcp.Description("Do not use system proxy env vars")),
		), handleStartMITMV2),
	)
}

func handleStartMITMV2(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.MITMV2Request
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"host": req.GetHost(),
			"port": req.GetPort(),
		}
		bgCtx := context.Background()
		stream, err := s.grpcClient.MITMV2(bgCtx)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start mitm v2")
		}
		if err := stream.Send(&req); err != nil {
			return nil, utils.Wrap(err, "failed to send mitm v2 request")
		}
		storeBackgroundStreamStatus("start_mitm_v2", summary)
		go func() {
			for {
				rsp, err := stream.Recv()
				if err != nil {
					return
				}
				if rsp != nil && rsp.HaveMessage && rsp.Message != nil {
					content := handleExecMessage(string(rsp.Message.Message))
					if content != "" {
						appendBackgroundStreamLog("start_mitm_v2", content)
					}
				}
			}
		}()
		return NewCommonCallToolResult(map[string]any{
			"status":  "started",
			"name":    "start_mitm_v2",
			"summary": summary,
		})
	}
}
