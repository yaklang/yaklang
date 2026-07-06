package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("syntaxflow",
		WithTool(mcp.NewTool("query_syntaxflow_rule",
			mcp.WithDescription("Query SyntaxFlow rules with filters"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "rule_name", "language", "purpose"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("SyntaxFlow rule filter")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowRule(ctx, req)
		}, "failed to query syntaxflow rule")),

		WithTool(mcp.NewTool("create_syntaxflow_rule",
			mcp.WithDescription("Create a new SyntaxFlow rule"),
			mcp.WithStruct("rule", []mcp.PropertyOption{
				mcp.Description("SyntaxFlow rule content"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.CreateSyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.CreateSyntaxFlowRuleEx(ctx, req)
		}, "failed to create syntaxflow rule")),

		WithTool(mcp.NewTool("update_syntaxflow_rule",
			mcp.WithDescription("Update an existing SyntaxFlow rule"),
			mcp.WithStruct("rule", []mcp.PropertyOption{
				mcp.Description("SyntaxFlow rule update payload"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.UpdateSyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.UpdateSyntaxFlowRuleEx(ctx, req)
		}, "failed to update syntaxflow rule")),

		WithTool(mcp.NewTool("delete_syntaxflow_rule",
			mcp.WithDescription("Delete SyntaxFlow rules by filter"),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Filter selecting rules to delete"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteSyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.DeleteSyntaxFlowRule(ctx, req)
		}, "failed to delete syntaxflow rule")),

		WithTool(mcp.NewTool("query_syntaxflow_result",
			mcp.WithDescription("Query SyntaxFlow scan results"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "rule_name", "program_name"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Result filter")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowResultRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowResult(ctx, req)
		}, "failed to query syntaxflow result")),

		WithTool(mcp.NewTool("query_syntaxflow_scan_task",
			mcp.WithDescription("Query SyntaxFlow scan tasks"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "task_id", "status"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Task filter")}),
			mcp.WithBool("showDiffRisk", mcp.Description("Include diff risk info")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowScanTaskRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowScanTask(ctx, req)
		}, "failed to query syntaxflow scan task")),

		WithTool(mcp.NewTool("delete_syntaxflow_scan_task",
			mcp.WithDescription("Delete SyntaxFlow scan tasks by filter"),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Task filter"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteSyntaxFlowScanTaskRequest) (any, error) {
			return s.grpcClient.DeleteSyntaxFlowScanTask(ctx, req)
		}, "failed to delete syntaxflow scan task")),

		WithTool(mcp.NewTool("syntaxflow_scan",
			mcp.WithDescription("Start a SyntaxFlow scan task (runs in background)"),
			mcp.WithString("controlMode", mcp.Description("Control mode: start, pause, resume, status"), mcp.Default("start")),
			mcp.WithStringArray("programName", mcp.Description("SSA program names to scan")),
			mcp.WithString("resumeTaskId", mcp.Description("Task ID to resume")),
			mcp.WithBool("ignoreLanguage", mcp.Description("Ignore language filter on rules")),
			mcp.WithNumber("concurrency", mcp.Description("Scan concurrency"), mcp.Default(5)),
			mcp.WithBool("memory", mcp.Description("Compile data only in memory")),
			mcp.WithNumber("ssaProjectId", mcp.Description("SSA project ID")),
			mcp.WithStringArray("projectName", mcp.Description("Project names to scan")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("SyntaxFlow rule filter")}),
		), handleSyntaxFlowScan),
	)
}

func handleSyntaxFlowScan(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.SyntaxFlowScanRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		if req.ControlMode == "" {
			req.ControlMode = "start"
		}
		summary := map[string]any{
			"controlMode":  req.ControlMode,
			"programName":  req.ProgramName,
			"resumeTaskId": req.ResumeTaskId,
		}
		bgCtx := context.Background()
		stream, err := s.grpcClient.SyntaxFlowScan(bgCtx)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start syntaxflow scan")
		}
		if err := stream.Send(&req); err != nil {
			return nil, utils.Wrap(err, "failed to send syntaxflow scan request")
		}
		storeBackgroundStreamStatus("syntaxflow_scan", summary)
		go func() {
			for {
				rsp, err := stream.Recv()
				if err != nil {
					return
				}
				if rsp != nil && rsp.ExecResult != nil {
					content := handleExecMessage(string(rsp.ExecResult.Message))
					if content != "" {
						appendBackgroundStreamLog("syntaxflow_scan", content)
					}
				}
			}
		}()
		return NewCommonCallToolResult(map[string]any{
			"status":  "started",
			"name":    "syntaxflow_scan",
			"summary": summary,
		})
	}
}
