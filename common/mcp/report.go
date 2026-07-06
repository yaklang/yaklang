package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("report",
		WithTool(mcp.NewTool("query_reports",
			mcp.WithDescription("Query scan reports"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "title", "from"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Report query filter")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryReportsRequest) (any, error) {
			return s.grpcClient.QueryReports(ctx, req)
		}, "failed to query reports")),

		WithTool(mcp.NewTool("query_report",
			mcp.WithDescription("Query a single report by ID"),
			mcp.WithNumber("id", mcp.Description("Report ID"), mcp.Required()),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryReportRequest) (any, error) {
			return s.grpcClient.QueryReport(ctx, req)
		}, "failed to query report")),

		WithTool(mcp.NewTool("delete_report",
			mcp.WithDescription("Delete reports by filter"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Delete report request"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteReportRequest) (any, error) {
			_, err := s.grpcClient.DeleteReport(ctx, req)
			if err != nil {
				return nil, err
			}
			return "delete report success", nil
		}, "failed to delete report")),

		WithTool(mcp.NewTool("generate_ssa_report",
			mcp.WithDescription("Generate an SSA static analysis report"),
			mcp.WithString("taskID", mcp.Description("SSA scan task ID")),
			mcp.WithString("reportName", mcp.Description("Report name")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Risk filter for report content")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.GenerateSSAReportRequest) (any, error) {
			return s.grpcClient.GenerateSSAReport(ctx, req)
		}, "failed to generate ssa report")),
	)
}
