package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("chaos_maker",
		WithTool(mcp.NewTool("query_chaos_maker_rule",
			mcp.WithDescription("Query ChaosMaker fuzzing rules"),
			mcp.WithStruct("request", []mcp.PropertyOption{mcp.Description("Query parameters")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryChaosMakerRuleRequest) (any, error) {
			return s.grpcClient.QueryChaosMakerRule(ctx, req)
		}, "failed to query chaos maker rule")),

		WithTool(mcp.NewTool("import_chaos_maker_rules",
			mcp.WithDescription("Import ChaosMaker rules"),
			mcp.WithString("content", mcp.Description("Rule content (YAML/JSON)"), mcp.Required()),
			mcp.WithString("ruleType", mcp.Description("Rule type: suricata / http-request / icmp")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.ImportChaosMakerRulesRequest) (any, error) {
			_, err := s.grpcClient.ImportChaosMakerRules(ctx, req)
			if err != nil {
				return nil, err
			}
			return "import chaos maker rules success", nil
		}, "failed to import chaos maker rules")),

		WithTool(mcp.NewTool("delete_chaos_maker_rule_by_id",
			mcp.WithDescription("Delete a ChaosMaker rule by ID"),
			mcp.WithNumber("id", mcp.Description("Rule ID"), mcp.Required()),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteChaosMakerRuleByIDRequest) (any, error) {
			_, err := s.grpcClient.DeleteChaosMakerRuleByID(ctx, req)
			if err != nil {
				return nil, err
			}
			return "delete chaos maker rule success", nil
		}, "failed to delete chaos maker rule")),

		WithTool(mcp.NewTool("execute_chaos_maker_rule",
			mcp.WithDescription("Execute a ChaosMaker rule (runs in background)"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Execute request with rule groups and targets"),
				mcp.Required(),
			}),
		), handleExecuteChaosMakerRule),
	)
}

func handleExecuteChaosMakerRule(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.ExecuteChaosMakerRuleRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"groups": len(req.GetGroups()),
		}
		return startBackgroundExecStream(s, "execute_chaos_maker_rule", summary, func(bgCtx context.Context) (execResultReceiver, error) {
			return s.grpcClient.ExecuteChaosMakerRule(bgCtx, &req)
		})
	}
}
