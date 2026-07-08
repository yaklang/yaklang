package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var filterSyntaxFlowRuleToolOptions = []mcp.ToolOption{
	mcp.WithStringArray("ruleNames", mcp.Description("Exact SyntaxFlow rule names")),
	mcp.WithStringArray("language", mcp.Description("Target languages: java, php, js, go, ...")),
	mcp.WithStringArray("groupNames", mcp.Description("Rule groups")),
	mcp.WithStringArray("severity", mcp.Description("Severity: info, low, middle, high, critical")),
	mcp.WithStringArray("purpose", mcp.Description("Rule purpose tags")),
	mcp.WithStringArray("tag", mcp.Description("Rule tags")),
	mcp.WithString("keyword", mcp.Description("Fuzzy search on rule name/content")),
	mcp.WithNumber("afterId", mcp.Description("Rules with ID > afterId")),
	mcp.WithNumber("beforeId", mcp.Description("Rules with ID < beforeId")),
	mcp.WithString("filterRuleKind", mcp.Description("buildIn | unBuildIn | empty for all")),
	mcp.WithString("filterLibRuleKind", mcp.Description("lib | noLib | empty for all")),
	mcp.WithStringArray("ruleIds", mcp.Description("Rule ID strings")),
	mcp.WithNumberArray("ids", mcp.Description("Rule numeric IDs")),
}

var syntaxFlowRuleInputToolOptions = []mcp.ToolOption{
	mcp.WithString("ruleName", mcp.Description("Unique rule name (index, cannot change on update)"), mcp.Required()),
	mcp.WithString("content", mcp.Description("SyntaxFlow rule source, e.g. `println(* #-> as $sink); $sink`"), mcp.Required()),
	mcp.WithString("language", mcp.Description("Target language: java, php, js, go, ..."), mcp.Required()),
	mcp.WithString("description", mcp.Description("Human-readable rule description")),
	mcp.WithStringArray("tags", mcp.Description("Rule tags")),
	mcp.WithStringArray("groupNames", mcp.Description("Groups to assign")),
}

var filterSyntaxFlowResultToolOptions = []mcp.ToolOption{
	mcp.WithStringArray("taskIDs", mcp.Description("Scan task IDs")),
	mcp.WithStringArray("resultIDs", mcp.Description("Result record IDs")),
	mcp.WithStringArray("ruleNames", mcp.Description("Rule names")),
	mcp.WithStringArray("programNames", mcp.Description("SSA program names produced by ssa_compile")),
	mcp.WithString("keyword", mcp.Description("Fuzzy search")),
	mcp.WithBool("onlyRisk", mcp.Description("Only results with risks")),
	mcp.WithNumber("afterID", mcp.Description("Results with ID > afterID")),
	mcp.WithNumber("beforeID", mcp.Description("Results with ID < beforeID")),
	mcp.WithStringArray("severity", mcp.Description("info, low, middle, high, critical")),
	mcp.WithStringArray("kind", mcp.Description("query | debug | scan")),
}

var filterSyntaxFlowScanTaskToolOptions = []mcp.ToolOption{
	mcp.WithStringArray("programs", mcp.Description("SSA program names to filter scan tasks")),
	mcp.WithStringArray("status", mcp.Description("Task status values")),
	mcp.WithStringArray("taskIds", mcp.Description("Task IDs")),
	mcp.WithString("keyword", mcp.Description("Fuzzy search on program name")),
	mcp.WithStringArray("kind", mcp.Description("debug | scan")),
	mcp.WithNumberArray("projectIds", mcp.Description("SSA project IDs")),
	mcp.WithBool("haveRisk", mcp.Description("Only tasks with risks")),
	mcp.WithNumber("fromId", mcp.Description("Tasks with ID >= fromId")),
	mcp.WithNumber("untilId", mcp.Description("Tasks with ID <= untilId")),
}

func init() {
	AddGlobalToolSet("syntaxflow",
		WithTool(mcp.NewTool("query_syntaxflow_rule",
			mcp.WithDescription("Page SyntaxFlow static-analysis rules from DB; filter by language, severity, keyword, built-in vs custom"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "rule_name", "language", "purpose"},
				mcp.Description("Pagination settings for the query")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("SyntaxFlow rule filter"),
			}, filterSyntaxFlowRuleToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowRule(ctx, req)
		}, "failed to query syntaxflow rule")),

		WithTool(mcp.NewTool("create_syntaxflow_rule",
			mcp.WithDescription("Create a custom SyntaxFlow rule in DB; syntaxFlowInput needs ruleName, language, content"),
			mcp.WithStruct("syntaxFlowInput", []mcp.PropertyOption{
				mcp.Description("SyntaxFlow rule payload"),
				mcp.Required(),
			}, syntaxFlowRuleInputToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.CreateSyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.CreateSyntaxFlowRuleEx(ctx, req)
		}, "failed to create syntaxflow rule")),

		WithTool(mcp.NewTool("update_syntaxflow_rule",
			mcp.WithDescription("Update rule fields by ruleName (identifier; ruleName itself cannot change)"),
			mcp.WithStruct("syntaxFlowInput", []mcp.PropertyOption{
				mcp.Description("Fields to update; ruleName required as identifier"),
				mcp.Required(),
			}, syntaxFlowRuleInputToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.UpdateSyntaxFlowRuleRequest) (any, error) {
			return s.grpcClient.UpdateSyntaxFlowRuleEx(ctx, req)
		}, "failed to update syntaxflow rule")),

		WithTool(mcp.NewTool("query_syntaxflow_result",
			mcp.WithDescription("Page SyntaxFlow hit results after syntaxflow_scan; filter by programNames, ruleNames, taskIDs, onlyRisk"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "rule_name", "program_name"},
				mcp.Description("Pagination settings for the query")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Result filter"),
			}, filterSyntaxFlowResultToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowResultRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowResult(ctx, req)
		}, "failed to query syntaxflow result")),

		WithTool(mcp.NewTool("query_syntaxflow_scan_task",
			mcp.WithDescription("Page SyntaxFlow scan tasks and status/progress; use showDiffRisk for SSA risk diff summary"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "task_id", "status"},
				mcp.Description("Pagination settings for the query")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Scan task filter"),
			}, filterSyntaxFlowScanTaskToolOptions...),
			mcp.WithBool("showDiffRisk", mcp.Description("Include SSA risk diff info")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QuerySyntaxFlowScanTaskRequest) (any, error) {
			return s.grpcClient.QuerySyntaxFlowScanTask(ctx, req)
		}, "failed to query syntaxflow scan task")),

		WithTool(mcp.NewTool("syntaxflow_scan",
			mcp.WithDescription("Start/pause/resume SSA SyntaxFlow batch scan in background (returns status:started). Prerequisite: ssa_compile programs. Then query_syntaxflow_scan_task / query_syntaxflow_result / query_new_risks"),
			mcp.WithString("controlMode", mcp.Description("start | pause | resume | status"), mcp.Default("start")),
			mcp.WithStringArray("programName", mcp.Description("SSA program names from ssa_compile to scan")),
			mcp.WithString("resumeTaskId", mcp.Description("Task id from query_syntaxflow_scan_task to resume paused scan")),
			mcp.WithBool("ignoreLanguage", mcp.Description("If false (default), only rules matching program language run")),
			mcp.WithNumber("concurrency", mcp.Description("Parallel rule workers"), mcp.Default(5)),
			mcp.WithBool("memory", mcp.Description("Keep SSA IR in memory only (no disk project)")),
			mcp.WithNumber("ssaProjectId", mcp.Description("Scan all programs under this SSA project id")),
			mcp.WithStringArray("projectName", mcp.Description("Scan programs under named SSA projects")),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Rule filter; same fields as query_syntaxflow_rule"),
			}, filterSyntaxFlowRuleToolOptions...),
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
