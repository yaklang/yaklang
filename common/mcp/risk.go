package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var filterRisksToolOptions = []mcp.ToolOption{
	mcp.WithPaging("pagination",
		[]string{"id", "created_at", "updated_at", "hash", "ip", "url", "port", "host", "title", "risk_type", "severity"},
		mcp.Description("Pagination settings for the query"),
	),
	mcp.WithString("search", mcp.Description("Fuzzy search keyword")),
	mcp.WithString("network", mcp.Description("Filter by IP/host network")),
	mcp.WithString("ports", mcp.Description("Filter by ports")),
	mcp.WithString("riskType", mcp.Description("Filter by risk type")),
	mcp.WithString("token", mcp.Description("Filter by reverse connection token")),
	mcp.WithBool("waitingVerified", mcp.Description("Filter by waiting-verified status")),
	mcp.WithString("severity", mcp.Description("Filter by severity: info, low, middle, high, critical")),
	mcp.WithString("tags", mcp.Description("Filter by tags")),
	mcp.WithString("title", mcp.Description("Filter by title")),
	mcp.WithString("runtimeId", mcp.Description("Filter by single runtime ID")),
	mcp.WithStringArray("runtimeIds", mcp.Description("Filter by runtime IDs")),
	mcp.WithString("isRead", mcp.Description("Filter by read status")),
	mcp.WithNumber("fromId", mcp.Description("Filter risks with ID >= fromId")),
	mcp.WithNumber("untilId", mcp.Description("Filter risks with ID <= untilId")),
	mcp.WithNumber("beforeCreatedAt", mcp.Description("Filter risks created before unix timestamp")),
	mcp.WithNumber("afterCreatedAt", mcp.Description("Filter risks created after unix timestamp")),
	mcp.WithNumberArray("ids", mcp.Description("Filter by risk IDs")),
	mcp.WithStringArray("ssaProgramNames", mcp.Description("Filter by SSA program names")),
}

func init() {
	AddGlobalToolSet("risk",
		WithTool(mcp.NewTool("query_risks",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query vulnerability/risk records with flexible filters"),
			}, filterRisksToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryRisksRequest) (any, error) {
			return s.grpcClient.QueryRisks(ctx, req)
		}, "failed to query risks")),

		WithTool(mcp.NewTool("query_risk",
			mcp.WithDescription("Query a single risk record by ID, hash, or filter"),
			mcp.WithNumber("id", mcp.Description("Risk ID")),
			mcp.WithString("hash", mcp.Description("Risk hash")),
			mcp.WithNumberArray("ids", mcp.Description("Risk IDs")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Filter same as query_risks"),
			}, filterRisksToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryRiskRequest) (any, error) {
			return s.grpcClient.QueryRisk(ctx, req)
		}, "failed to query risk")),

		WithTool(mcp.NewTool("delete_risk",
			mcp.WithDescription("Delete risk records by ID, hash, or filter"),
			mcp.WithNumber("id", mcp.Description("Risk ID")),
			mcp.WithString("hash", mcp.Description("Risk hash")),
			mcp.WithNumberArray("ids", mcp.Description("Risk IDs")),
			mcp.WithBool("deleteAll", mcp.Description("Delete all risks")),
			mcp.WithBool("deleteRepetition", mcp.Description("Delete duplicate risks only")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Filter same as query_risks"),
			}, filterRisksToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteRiskRequest) (any, error) {
			_, err := s.grpcClient.DeleteRisk(ctx, req)
			if err != nil {
				return nil, err
			}
			return "delete risk success", nil
		}, "failed to delete risk")),

		WithTool(mcp.NewTool("query_new_risks",
			mcp.WithDescription("Query new/unread risk records after a given ID"),
			mcp.WithNumber("afterId", mcp.Description("Return risks with ID greater than this value")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryNewRiskRequest) (any, error) {
			return s.grpcClient.QueryNewRisk(ctx, req)
		}, "failed to query new risks")),

		WithTool(mcp.NewTool("set_tag_for_risk",
			mcp.WithDescription("Set tags for a risk record"),
			mcp.WithNumber("id", mcp.Description("Risk ID"), mcp.Required()),
			mcp.WithString("hash", mcp.Description("Risk hash (alternative to id)")),
			mcp.WithStringArray("tags", mcp.Description("Tags to set"), mcp.Required()),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.SetTagForRiskRequest) (any, error) {
			_, err := s.grpcClient.SetTagForRisk(ctx, req)
			if err != nil {
				return nil, err
			}
			return "set tag for risk success", nil
		}, "failed to set tag for risk")),
	)
}
