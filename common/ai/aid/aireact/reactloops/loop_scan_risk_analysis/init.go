package loop_scan_risk_analysis

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/output_example.txt
var scanRiskOutputExample string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			// 工厂侧选项对齐 loop_internet_research / loop_write_python_script：Init 只做环境与 gate，主流程在 LoopAction + ReAct 首轮。
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithEnableSelfReflection(false),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(scanRiskOutputExample),
				reactloops.WithInitTask(buildScanRiskAnalysisInitTask(r)),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					return action.ActionType == schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS
				}),
				makeScanRiskAnalysisAction(r),
			}
			preset = append(opts, preset...)

			return reactloops.NewReActLoop(
				schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS,
				r,
				preset...,
			)
		},
		reactloops.WithLoopDescription("Scan risk analysis mode: load/merge risks, false-positive triage, then write Markdown that only surfaces (1) false-positive findings and (2) PoC-worthiness per non-false-positive group; full detail stays in analysis_summary.json. No automatic PoC generation."),
		reactloops.WithLoopDescriptionZh("扫描风险分析模式：加载合并后做误报分诊；对外 Markdown 仅展示「误报结论」与「哪些 risk 更值得后续 PoC（如 ssapoc）」，其余明细在 analysis_summary.json；不主动生成 PoC。"),
		reactloops.WithVerboseName("Scan Risk Analysis"),
		reactloops.WithVerboseNameZh("扫描风险分析"),
		reactloops.WithLoopUsagePrompt("Use when the user wants merged SyntaxFlow scan risk analysis and false-positive triage without automatic PoC. Entry requires project_name=<slug> (or interactive clarification); the pipeline runs sf_project_scan_check-equivalent project check then analyzes the latest matching scan task."),
		reactloops.WithLoopOutputExample(`{"@action": "scan_risk_analysis", "human_readable_thought": "project_name=go-sec-code — merged risks, FP triage, final report"}`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS, err)
	}
}

func scanRiskOutputDir(workDir, scanID string) string {
	return filepath.Join(workDir, "scan_risk_analysis", strings.TrimSpace(scanID))
}

// buildScanRiskAnalysisInitTask 对齐 loop_write_python_script：解析 project_name、项目检查、写入 loop 变量并约束首轮 ReAct 仅可选 scan_risk_analysis；流水线在 LoopAction 中执行。
func buildScanRiskAnalysisInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := task.GetUserInput()
		r.AddToTimeline("[SCAN_RISK_START]", "扫描风险分析开始，用户输入: "+utils.ShrinkTextBlock(userInput, 300))
		scanRiskEmitThoughtStream(r, task.GetIndex(), "扫描风险分析：初始化（解析 project_name 与工作目录）…")

		workDir := ""
		if cfg, ok := r.GetConfig().(interface{ GetOrCreateWorkDir() string }); ok {
			workDir = cfg.GetOrCreateWorkDir()
			log.Infof("[ScanRisk] workdir=%s", workDir)
		}
		if workDir == "" {
			tmp, err := os.MkdirTemp("", "scan-risk-analysis-*")
			if err != nil {
				op.Failed(fmt.Sprintf("create temp workdir failed: %v", err))
				return
			}
			workDir = tmp
			log.Warnf("[ScanRisk] no AI workdir from config, using temp: %s", workDir)
		}

		trimInput := strings.TrimSpace(userInput)
		projectName := parseStrictProjectNameLine(trimInput)
		if projectName == "" {
			projectName = parseOptionalPlainProjectSlug(trimInput)
		}
		if projectName == "" {
			ctx := task.GetContext()
			if utils.IsNil(ctx) {
				ctx = r.GetConfig().GetContext()
			}
			select {
			case <-ctx.Done():
				op.Failed("任务已取消，未完成项目名称输入")
				return
			default:
			}
			hint := extractProjectNameForScanAnalysis(trimInput)
			q := buildProjectNameClarificationPrompt(hint)
			r.AddToTimeline("[SCAN_RISK_NEED_PROJECT]", "该模式要求必须先提供 project_name=...；已主动发起交互提问")
			scanRiskEmitThoughtStream(r, task.GetIndex(), "缺少固定格式的 project_name=…；已弹出询问，请按提示回复一行后再继续。")
			if hs := sanitizeProjectNameToken(hint); hs != "" {
				q = q + "\n\n建议直接复制填写：`project_name=" + hs + "`"
			}
			answer := r.AskForClarification(ctx, q, nil)
			if trimmed := strings.TrimSpace(answer); trimmed != "" {
				r.AddToTimeline("[SCAN_RISK_PROJECT_REPLY]", "用户补充（截断）: "+utils.ShrinkTextBlock(trimmed, 500))
			}
			projectName = parseInteractiveProjectNameReply(answer)
			if projectName == "" {
				op.Failed("未解析到项目名称。请仅回复一行：project_name=<程序或仓库标识>（示例：project_name=go-sec-code）")
				return
			}
		}

		r.AddToTimeline("[SCAN_RISK_PROJECT]", fmt.Sprintf("已接收项目名称 project_name=%q，开始执行项目扫描检查", projectName))
		scanRiskEmitThoughtStream(r, task.GetIndex(), "已接收 project_name=%q：正在执行项目扫描检查并匹配最新 SyntaxFlow 扫描任务…", projectName)
		scanID, report, pickErr := resolveScanIDByProjectName(projectName, true, 20)
		if pickErr != nil {
			op.Failed(fmt.Sprintf("项目检查或扫描任务定位失败（project_name=%q）: %v", projectName, pickErr))
			return
		}
		r.AddToTimeline("[SCAN_RISK_PROJECT_CHECK]", utils.ShrinkTextBlock(report, 8000))
		if pinned := r.EmitFileArtifactWithExt("sf_project_scan_check_report", ".md", report); pinned != "" {
			r.AddToTimeline("[SCAN_RISK_PROJECT_CHECK_FILE]", "项目检查报告已钉选: "+pinned)
		}
		via := fmt.Sprintf("project_name=%q -> sf_project_scan_check -> latest task", projectName)
		r.AddToTimeline("[SCAN_RISK_RESOLVED]", fmt.Sprintf("已解析 scan_id=`%s`（%s）", scanID, via))
		scanRiskEmitThoughtStream(r, task.GetIndex(), "已解析 scan_id=%s；下一阶段请输出 JSON，且 action 键值必须为 scan_risk_analysis（见输出示例），以启动合并、误报分诊与报告流水线（本模式不自动生成 PoC）。", scanID)
		loop.Set("scan_id", scanID)
		loop.Set("scan_risk_workdir", workDir)
		loop.Set("scan_risk_project_name", projectName)
		op.NextAction(schema.AI_REACT_LOOP_NAME_SCAN_RISK_ANALYSIS)
		op.Continue()
	}
}

func executeScanRiskAnalysisPipeline(r aicommon.AIInvokeRuntime, op *reactloops.LoopActionHandlerOperator, workDir, scanID, thoughtTaskKey string) {
	outDir := scanRiskOutputDir(workDir, scanID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		op.Fail(fmt.Sprintf("创建输出目录失败: %v", err))
		return
	}
	r.AddToTimeline("[SCAN_RISK_DIR]", "扫描风险分析输出目录: "+outDir)
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "扫描风险流水线开始：输出目录 %s", outDir)

	state := newState(scanID, workDir)

	r.AddToTimeline("[SCAN_RISK_PHASE1]", "开始阶段：加载扫描任务并合并风险（load+merge）")
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 1/4] 加载扫描任务并按特征合并风险…")
	if err := state.loadAndMerge(); err != nil {
		op.Fail(err)
		return
	}
	r.AddToTimeline("[SCAN_RISK_PHASE1_DONE]", fmt.Sprintf(
		"合并完成：分组数=%d，原始风险条数=%d", len(state.Groups), len(state.RawRisks),
	))
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 1/4] 完成：合并分组 %d 个，原始风险 %d 条。", len(state.Groups), len(state.RawRisks))

	r.AddToTimeline("[SCAN_RISK_PHASE2]", "开始阶段：生成 SSA 扫描报告（与 gRPC GenerateSSAReport 同源）")
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 2/4] 生成 SSA 扫描报告 Markdown（source_ssa_report.md）…")
	if err := state.generateSourceSSAReport(r.GetConfig().GetContext()); err != nil {
		op.Fail(err)
		return
	}
	r.AddToTimeline("[SCAN_RISK_PHASE2_DONE]", fmt.Sprintf("SSA 报告已生成：report_id=%d path=%s", state.SourceSSAReportID, state.SourceSSAReportPath))
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 2/4] SSA 报告已写入：%s", state.SourceSSAReportPath)
	if strings.TrimSpace(state.SourceSSAReportMarkdown) != "" {
		if pinned := r.EmitFileArtifactWithExt("ssa_scan_source_report", ".md", state.SourceSSAReportMarkdown); pinned != "" {
			r.AddToTimeline("[SCAN_RISK_SSA_REPORT_FILE]", "SSA 原始报告已钉选: "+pinned)
		}
	}

	r.AddToTimeline("[SCAN_RISK_PHASE3]", "开始阶段：误报分诊（基于 SSA 报告 + AI）")
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 3/4] 误报分诊：结构化评分 + 基于 SSA 报告的 AI 判定…")
	if err := state.analyzeFalsePositive(r.GetConfig().GetContext(), r); err != nil {
		op.Fail(err)
		return
	}
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 3/4] 误报分诊完成。")

	r.AddToTimeline("[SCAN_RISK_PHASE4]", "开始阶段：生成 Markdown 报告与 JSON 摘要（不生成 PoC）")
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 4/4] 生成 Markdown 报告与 JSON 摘要（本模式不自动生成 PoC）…")
	if err := state.generateReports(); err != nil {
		op.Fail(err)
		return
	}

	summaryPath := filepath.Join(outDir, "analysis_summary.json")
	manifestPath := filepath.Join(outDir, "poc_manifest.json")
	fpReportPath := filepath.Join(outDir, "false_positive_report.md")
	pocReportPath := filepath.Join(outDir, "poc_generation_report.md")
	sourceReportPath := filepath.Join(outDir, "source_ssa_report.md")
	r.AddToTimeline("[SCAN_RISK_ARTIFACTS]", fmt.Sprintf(
		"工件已写入：\n- %s（SSA 原始报告 Markdown）\n- %s（仅误报+PoC价值）\n- %s\n- %s\n- %s\n- %s",
		sourceReportPath, filepath.Join(outDir, "analysis_report.md"), summaryPath, manifestPath, fpReportPath, pocReportPath,
	))
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "[阶段 4/4] 报告与摘要已写入磁盘。")

	reportPath := filepath.Join(outDir, "analysis_report.md")
	reportBytes, readErr := os.ReadFile(reportPath)
	pinnedReport := ""
	if readErr != nil {
		r.AddToTimeline("[SCAN_RISK_DONE]", fmt.Sprintf(
			"扫描风险分析结束，但读取报告失败: %v\n输出目录: %s", readErr, outDir,
		))
		r.EmitResult(map[string]any{
			"kind":       "scan_risk_analysis",
			"success":    false,
			"scan_id":    state.ScanID,
			"output_dir": outDir,
			"error":      readErr.Error(),
		})
		op.Exit()
		return
	}
	if len(reportBytes) == 0 {
		r.AddToTimeline("[SCAN_RISK_DONE]", "扫描风险分析结束，但 analysis_report.md 为空。\n输出目录: "+outDir)
		r.EmitResult(map[string]any{
			"kind":       "scan_risk_analysis",
			"success":    false,
			"scan_id":    state.ScanID,
			"output_dir": outDir,
			"error":      "empty analysis_report.md",
		})
		op.Exit()
		return
	}

	reportText := string(reportBytes)
	pinnedReport = r.EmitFileArtifactWithExt("scan_risk_analysis_report", ".md", reportText)
	if pinnedReport == "" {
		log.Warnf("[ScanRisk] EmitFileArtifactWithExt returned empty path")
	}

	r.AddToTimeline("[SCAN_RISK_DONE]", "扫描风险分析全部完成。报告预览:\n"+utils.ShrinkTextBlock(reportText, 2000))
	scanRiskEmitThoughtStream(r, thoughtTaskKey, "扫描风险分析全部完成；报告已钉选，正在向会话推送 Markdown 摘要…")
	log.Infof("[ScanRisk] finished scan_id=%s groups=%d report_bytes=%d pinned=%q",
		state.ScanID, len(state.Groups), len(reportBytes), pinnedReport)

	summaryMarkdown := scanRiskUserSummaryMarkdown(state, outDir, reportPath, pinnedReport, fpReportPath, pocReportPath, sourceReportPath)
	scanRiskDeliverFinalMarkdown(r, thoughtTaskKey, summaryMarkdown)
	r.EmitResult(map[string]any{
		"kind":                  "scan_risk_analysis",
		"success":               true,
		"scan_id":               state.ScanID,
		"output_dir":            outDir,
		"source_ssa_report":     sourceReportPath,
		"source_ssa_report_id":  state.SourceSSAReportID,
		"report_path":           reportPath,
		"false_positive_report": fpReportPath,
		"poc_generation_report": pocReportPath,
		"summary_path":          summaryPath,
		"poc_manifest":          manifestPath,
		"pinned_artifact":       pinnedReport,
		"markdown":              summaryMarkdown,
		"report_preview":        utils.ShrinkTextBlock(reportText, 4000),
	})

	op.Exit()
}

func scanRiskUserSummaryMarkdown(state *AnalysisState, outDir, reportPath, pinnedReport, fpReportPath, pocReportPath, sourceReportPath string) string {
	var sb strings.Builder
	sb.WriteString("## 扫描风险分析结果\n\n")
	sb.WriteString(fmt.Sprintf("`scan_id`: `%s`\n\n", state.ScanID))
	sb.WriteString(fmt.Sprintf("- **输出目录**: `%s`\n", outDir))
	sb.WriteString(fmt.Sprintf("- **SSA 原始报告（GenerateSSAReport 同源）**: `%s`\n", sourceReportPath))
	sb.WriteString(fmt.Sprintf("- **总报告**: `%s`\n", reportPath))
	sb.WriteString(fmt.Sprintf("- **误报分诊**: `%s`\n", fpReportPath))
	sb.WriteString(fmt.Sprintf("- **PoC 价值评估**: `%s`\n", pocReportPath))
	if pinnedReport != "" {
		sb.WriteString(fmt.Sprintf("- **会话钉选报告副本**: `%s`\n", pinnedReport))
	}
	sb.WriteString("\n以下为 **误报分诊** 与 **PoC 价值评估** 正文。\n")

	if state.Report != nil {
		sb.WriteString("\n---\n\n## 误报分诊\n\n")
		if strings.TrimSpace(state.AIFPReportMarkdown) != "" {
			sb.WriteString(strings.TrimSpace(state.AIFPReportMarkdown))
			sb.WriteString("\n")
		} else {
			sb.WriteString(falsePositiveReportInner(state.Report))
		}
		sb.WriteString("\n---\n\n## PoC 价值评估\n\n")
		sb.WriteString(pocWorthyReportInner(state.Report))
	} else {
		sb.WriteString("\n（内部错误：Report 未生成。）\n")
	}
	sb.WriteString("\n全量结构化数据见 `analysis_summary.json`。\n")
	return sb.String()
}

// scanRiskEmitThoughtStream pushes visible “思考” streams (re-act-loop-thought) like loop_syntaxflow_rule / loop_yaklangcode.
const scanRiskFinalMarkdownMaxBytes = 120 * 1024

// scanRiskDeliverFinalMarkdown 将会话可见摘要推到 re-act-loop-answer-payload，并 EmitResultAfterStream，
// 与 loop_internet_research / loop_http_fuzztest 一致，避免仅 EmitResult(map) 时前端聊天区无正文。
func scanRiskDeliverFinalMarkdown(r aicommon.AIInvokeRuntime, taskKey, markdown string) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return
	}
	if len(markdown) > scanRiskFinalMarkdownMaxBytes {
		markdown = markdown[:scanRiskFinalMarkdownMaxBytes] + "\n\n…(truncated)"
	}
	tk := strings.TrimSpace(taskKey)
	if tk == "" {
		tk = "scan-risk-analysis"
	}
	em := r.GetConfig().GetEmitter()
	if em != nil && !utils.IsNil(em) {
		if _, err := em.EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(markdown), tk, func() {}); err != nil {
			log.Warnf("[ScanRisk] EmitTextMarkdownStreamEvent: %v", err)
		}
	}
	r.EmitResultAfterStream(markdown)
}

func scanRiskEmitThoughtStream(r aicommon.AIInvokeRuntime, taskKey string, format string, args ...any) {
	if r == nil || utils.IsNil(r) {
		return
	}
	em := r.GetConfig().GetEmitter()
	if em == nil || utils.IsNil(em) {
		return
	}
	k := strings.TrimSpace(taskKey)
	if k == "" {
		k = "scan-risk-analysis"
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	_, _ = em.EmitThoughtStream(k, msg)
}
