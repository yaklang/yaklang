package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var filterFingerprintToolOptions = []mcp.ToolOption{
	mcp.WithStringArray("ruleName", mcp.Description("Exact rule names to match")),
	mcp.WithStringArray("groupName", mcp.Description("Rules that belong to any of these groups")),
	mcp.WithStringArray("vendor", mcp.Description("Rules whose CPE vendor is in this list")),
	mcp.WithStringArray("product", mcp.Description("Rules whose CPE product is in this list")),
	mcp.WithNumberArray("includeId", mcp.Description("Rules with these database IDs")),
	mcp.WithString("keyword", mcp.Description("Fuzzy search across rule name, match expression, and CPE")),
}

var fingerprintRuleToolOptions = []mcp.ToolOption{
	mcp.WithString("ruleName",
		mcp.Description("Unique rule name (identifier), e.g. nginx-default-page"),
		mcp.Required(),
	),
	mcp.WithString("matchExpression",
		mcp.Description(`Fingerprint match DSL on HTTP response. Fields: body, header, title, server, header_<Name>. Operators: =, ==, !=, !==, ~=. Logic: &&, ||. Example: body="nginx" && header="X-Powered-By"`),
		mcp.Required(),
	),
	mcp.WithString("webPath",
		mcp.Description("Optional URL path to fetch before matching, e.g. / or /login"),
	),
	mcp.WithString("extInfo",
		mcp.Description("Optional extra metadata"),
	),
	mcp.WithStringArray("groupName",
		mcp.Description("Optional group labels for organizing rules in UI"),
	),
	mcp.WithStruct("cpe", []mcp.PropertyOption{
		mcp.Description("CPE product metadata reported when matched"),
	},
		mcp.WithString("vendor", mcp.Description("CPE vendor, e.g. nginx")),
		mcp.WithString("product", mcp.Description("CPE product name, e.g. nginx")),
		mcp.WithString("version", mcp.Description("CPE version or * wildcard")),
		mcp.WithString("part", mcp.Description("CPE part: a (app), h (hardware), o (OS)")),
	),
}

func init() {
	AddGlobalToolSet("fingerprint",
		WithTool(mcp.NewTool("query_fingerprint",
			mcp.WithDescription("Page HTTP service fingerprint rules used by port_scan and crawlers; matchExpression runs on response body/headers/title"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "name", "type"},
				mcp.Description("Pagination settings for the query")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Fingerprint filter; fields are combined with AND"),
			}, filterFingerprintToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryFingerprintRequest) (any, error) {
			return s.grpcClient.QueryFingerprint(ctx, req)
		}, "failed to query fingerprint")),

		WithTool(mcp.NewTool("create_fingerprint",
			mcp.WithDescription("Create custom fingerprint rule; rule.ruleName + rule.matchExpression required"),
			mcp.WithStruct("rule", []mcp.PropertyOption{
				mcp.Description("Fingerprint rule data"),
				mcp.Required(),
			}, fingerprintRuleToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.CreateFingerprintRequest) (any, error) {
			return s.grpcClient.CreateFingerprint(ctx, req)
		}, "failed to create fingerprint")),

		WithTool(mcp.NewTool("update_fingerprint",
			mcp.WithDescription("Update fingerprint by id or ruleName; only fields in rule struct are changed"),
			mcp.WithNumber("id", mcp.Description("Fingerprint rule ID")),
			mcp.WithString("ruleName", mcp.Description("Fingerprint rule name")),
			mcp.WithStruct("rule", []mcp.PropertyOption{
				mcp.Description("Fields to update"),
			},
				mcp.WithString("ruleName", mcp.Description("New rule name")),
				mcp.WithString("matchExpression", mcp.Description(`Match DSL; fields: body, header, title, server, header_<Name>; operators: =, ==, !=, !==, ~=; logic: &&, ||`)),
				mcp.WithString("webPath", mcp.Description("URL path to fetch before matching")),
				mcp.WithString("extInfo", mcp.Description("Extra metadata")),
				mcp.WithStringArray("groupName", mcp.Description("Group labels")),
				mcp.WithStruct("cpe", []mcp.PropertyOption{mcp.Description("CPE metadata")},
					mcp.WithString("vendor", mcp.Description("CPE vendor")),
					mcp.WithString("product", mcp.Description("CPE product")),
					mcp.WithString("version", mcp.Description("CPE version")),
					mcp.WithString("part", mcp.Description("CPE part")),
				),
			),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.UpdateFingerprintRequest) (any, error) {
			return s.grpcClient.UpdateFingerprint(ctx, req)
		}, "failed to update fingerprint")),

		WithTool(mcp.NewTool("delete_fingerprint",
			mcp.WithDescription("Delete fingerprint rules matching filter (same fields as query_fingerprint)"),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Fingerprint filter; same fields as query_fingerprint"),
				mcp.Required(),
			}, filterFingerprintToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteFingerprintRequest) (any, error) {
			return s.grpcClient.DeleteFingerprint(ctx, req)
		}, "failed to delete fingerprint")),
	)
}
