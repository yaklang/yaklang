package loop_syntaxflow_code_audit

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_CODE_AUDIT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			state := &SFCodeAuditState{}
			cfg := r.GetConfig()
			if c, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
				state.WorkDir = c.GetOrCreateWorkDir()
				log.Infof("[SFCodeAudit] workdir=%s", state.WorkDir)
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(buildOrchestratorInitTask(r, state)),
			}
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_CODE_AUDIT, r, append(opts, preset...)...)
		},
		reactloops.WithVerboseName("IRify · SyntaxFlow Code Audit"),
		reactloops.WithVerboseNameZh("IRify · SyntaxFlow 代码审计"),
		reactloops.WithLoopDescription("End-to-end IRify SyntaxFlow code audit: directory reconnaissance (dir_explore), author or refine SyntaxFlow rules (write_syntaxflow_rule), optionally align with an existing scan via task_id (syntaxflow_scan), and produce a consolidated Markdown security report for the target repo."),
		reactloops.WithLoopDescriptionZh("基于 IRify/SyntaxFlow 的代码安全审计：先目录探索，再编写或迭代 SyntaxFlow 检测规则，可选对照已有扫描任务（task_id）与 SSA 风险，最终输出汇总的 Markdown 审计报告。"),
		reactloops.WithLoopUsagePrompt("Use for a SyntaxFlow-centric repo audit: explore the tree, write or refine .sf rules, optionally tie to an existing scan (task_id), and emit a Markdown report. Yakit may attach irify_syntaxflow/task_id; orchestrators may inject WithVar(syntaxflow_task_id)."),
		reactloops.WithLoopOutputExample(`
* SyntaxFlow 专项代码审计：
  {"@action": "syntaxflow_code_audit", "human_readable_thought": "需要对目标项目做 SyntaxFlow 规则审计并输出报告"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_CODE_AUDIT, err)
	}
}

func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

func newSubTaskWithInput(parent aicommon.AIStatefulTask, name, userInput string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, userInput, true)
}

func auditDir(state *SFCodeAuditState) string {
	if state.WorkDir != "" {
		return filepath.Join(state.WorkDir, "sf_code_audit")
	}
	return filepath.Join(os.TempDir(), "sf_code_audit")
}

func fillStateFromExplore(state *SFCodeAuditState, exploreLoop *reactloops.ReActLoop) {
	projectPath := exploreLoop.Get("result_target_path")
	projectName := exploreLoop.Get("result_project_name")
	if projectPath != "" && projectName == "" {
		projectName = filepath.Base(projectPath)
	}
	tech := exploreLoop.Get("result_tech_stack")
	entry := exploreLoop.Get("result_entry_points")
	recon := exploreLoop.Get("result_report_path")
	state.SetProjectFromExplore(projectPath, projectName, tech, entry, recon)
}

func buildEnrichedRulePrompt(original string, state *SFCodeAuditState, reconPath string) string {
	recon := reconPath
	if recon == "" {
		recon = "（未生成或路径未知）"
	}
	return fmt.Sprintf(`%s

---
[SyntaxFlow 代码审计 — 阶段上下文]
请结合以下项目背景编写或完善可用于 IR 扫描的 SyntaxFlow 规则；生成后务必使用 check-syntaxflow-syntax 完成语法与样例自检。

- 项目路径: %s
- 项目名称: %s
- 技术栈摘要: %s
- 入口点摘要: %s
- 目录探索报告路径: %s
%s`, strings.TrimSpace(original), state.ProjectPath, state.ProjectName, state.TechStack, state.EntryPoints, recon, sfu.SFAuditCodeSearchHint())
}

func buildRefFilesHint(files []string) string {
	if len(files) == 0 {
		return "（无可用参考文件，请根据上方项目信息与用户目标撰写。）"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### 参考文件（共 %d 个，撰写前请用 read_reference_file 全部读取）\n", len(files)))
	for i, f := range files {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}
	return sb.String()
}

func buildSFCodeAuditReportPrompt(userInput string, state *SFCodeAuditState, refFiles []string, hasScanContextFile bool) string {
	fileHint := buildRefFilesHint(refFiles)
	scanNote := ""
	if hasScanContextFile {
		scanNote = "\n若列表中含 syntaxflow_scan_context.md，其中为可选阶段从数据库加载的扫描任务与 SSA 风险列表摘要，须纳入「与已有扫描对照」一节。\n"
	}
	return fmt.Sprintf(`请撰写一份 **SyntaxFlow 代码审计**总结报告（Markdown）。

## 用户目标

%s

## 已知项目上下文

- 项目路径: %s
- 项目名称: %s
- 技术栈: %s
- 入口点: %s

## 写作要求

1. 执行摘要：审计范围、方法与主要产出（含规则文件路径）。
2. 检测假设与规则设计要点（Source/Sink、数据流、告警级别）。
3. 规则自检：说明是否已通过 check-syntaxflow-syntax（及正例匹配情况，若有）。
4. 风险与局限：误报可能、需人工复核点。
5. 后续建议：如何在 IRify/Yakit 中复扫或迭代规则。
%s
## 参考文件

%s

请使用 read_reference_file 读完上述文件后再调用 write_section 写入报告。语言以中文为主。`,
		strings.TrimSpace(userInput),
		state.ProjectPath,
		state.ProjectName,
		state.TechStack,
		state.EntryPoints,
		scanNote,
		fileHint,
	)
}

func generateSFCodeAuditReport(
	r aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	reportPath string,
	writePrompt string,
	refFiles []string,
) error {
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(inner *reactloops.ReActLoop, task aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			inner.Set("report_filename", reportPath)
			inner.Set("full_report_code", "")
			inner.Set("user_requirements", writePrompt)
			inner.Set("available_files", buildRefFilesHint(refFiles))
			inner.Set("available_knowledge_bases", "")
			inner.Set("collected_references", "")
			inner.Set("is_modify_mode", "false")
			innerOp.Continue()
		}),
	)
	if err != nil {
		return err
	}
	sub := aicommon.NewSubTaskBase(parentTask, "sf-code-audit-report", writePrompt, true)
	return reportLoop.ExecuteWithExistedTask(sub)
}

func generateFallbackReport(state *SFCodeAuditState, userInput string) string {
	var sb strings.Builder
	sb.WriteString("# SyntaxFlow 代码审计报告（自动生成）\n\n")
	sb.WriteString("> 报告生成阶段未写出完整内容，以下为结构化占位摘要。\n\n")
	sb.WriteString(fmt.Sprintf("## 用户目标\n\n%s\n\n", strings.TrimSpace(userInput)))
	sb.WriteString("## 项目上下文\n\n")
	sb.WriteString(fmt.Sprintf("- 路径: %s\n", state.ProjectPath))
	sb.WriteString(fmt.Sprintf("- 名称: %s\n", state.ProjectName))
	sb.WriteString(fmt.Sprintf("- 技术栈: %s\n", state.TechStack))
	sb.WriteString(fmt.Sprintf("- 入口: %s\n\n", state.EntryPoints))
	if state.RuleFilePath != "" {
		sb.WriteString(fmt.Sprintf("## 规则文件\n\n`%s`\n\n", state.RuleFilePath))
	}
	if state.ScanReviewSummary != "" {
		sb.WriteString("## 扫描任务上下文（节选）\n\n")
		sb.WriteString(utils.ShrinkTextBlock(state.ScanReviewSummary, 8000))
		sb.WriteString("\n")
	}
	return sb.String()
}

func writeNextSteps(auditDirPath string, state *SFCodeAuditState, reportPath string) {
	p := filepath.Join(auditDirPath, "NEXT_STEPS.md")
	var sb strings.Builder
	sb.WriteString("# 后续建议\n\n")
	sb.WriteString(fmt.Sprintf("- 审计报告: `%s`\n", reportPath))
	if state.RuleFilePath != "" {
		sb.WriteString(fmt.Sprintf("- SyntaxFlow 规则: `%s`（可在 IRify 中挂载后复扫）\n", state.RuleFilePath))
	}
	sb.WriteString("- 若需对照数据库中的扫描任务，请为本任务附加 **irify_syntaxflow / task_id**（或等价结构化输入），以启用扫描解读阶段。\n")
	sb.WriteString("- 单条 SSA 风险深度分析可使用专注模式 `ssa_risk_review`。\n")
	_ = os.WriteFile(p, []byte(sb.String()), 0o644)
}

func buildOrchestratorInitTask(r aicommon.AIInvokeRuntime, state *SFCodeAuditState) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(_ *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := task.GetUserInput()
		log.Infof("[SFCodeAudit] Orchestrator started. workdir=%s", state.WorkDir)
		r.AddToTimeline("[SF_CODE_AUDIT_START]", "SyntaxFlow 代码审计开始: "+utils.ShrinkTextBlock(userInput, 300))

		auditDirPath := auditDir(state)
		if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
			log.Warnf("[SFCodeAudit] mkdir audit dir: %v", err)
			op.Failed(fmt.Sprintf("[SFCodeAudit] 无法创建输出目录: %v", err))
			return
		}
		r.AddToTimeline("[SF_CODE_AUDIT_DIR]", "输出目录: "+auditDirPath)

		// Phase 1: dir_explore
		reconFilePath := filepath.Join(auditDirPath, "recon_notes.md")
		exploreLoop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_DIR_EXPLORE,
			r,
			reactloops.WithVar("output_report_path", reconFilePath),
			reactloops.WithVar("explore_work_dir", auditDirPath),
		)
		if err != nil {
			log.Errorf("[SFCodeAudit] create dir_explore: %v", err)
			op.Failed(err)
			return
		}
		if err := exploreLoop.ExecuteWithExistedTask(newSubTask(task, "phase1")); err != nil {
			log.Warnf("[SFCodeAudit] Phase 1 dir_explore: %v (continuing)", err)
		}
		fillStateFromExplore(state, exploreLoop)
		if state.TechStack == "" {
			r.AddToTimeline("[SF_CODE_AUDIT_PHASE1_WARN]", "Phase 1 未产出完整技术栈信息，后续阶段将依赖用户原文与部分上下文。")
		}

		// Phase 2: write_syntaxflow_rule（子任务 userInput 注入项目上下文）
		enriched := buildEnrichedRulePrompt(userInput, state, state.ReconFilePath)
		ruleLoop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW, r)
		if err != nil {
			log.Errorf("[SFCodeAudit] create write_syntaxflow_rule: %v", err)
			op.Failed(err)
			return
		}
		if err := ruleLoop.ExecuteWithExistedTask(newSubTaskWithInput(task, "phase2", enriched)); err != nil {
			log.Warnf("[SFCodeAudit] Phase 2 write_syntaxflow_rule: %v (continuing)", err)
		}
		if sfPath := strings.TrimSpace(ruleLoop.Get("sf_filename")); sfPath != "" {
			state.SetRulePath(sfPath)
		}

		// Phase 3 (optional): syntaxflow_scan when task_id present (attachments / loop vars on parent task)
		hasScanContextFile := false
		if scanTid, ok := sfu.SyntaxFlowTaskID(task, nil); ok && scanTid != "" {
			scanLoop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, r,
				reactloops.WithVar(sfu.LoopVarSyntaxFlowTaskID, scanTid),
			)
			if err != nil {
				log.Warnf("[SFCodeAudit] create syntaxflow_scan: %v", err)
			} else {
				subScan := newSubTask(task, "phase3")
				subScan.SetAttachedDatas(task.GetAttachedDatas())
				if err := scanLoop.ExecuteWithExistedTask(subScan); err != nil {
					log.Warnf("[SFCodeAudit] Phase 3 syntaxflow_scan: %v (continuing)", err)
				}
				summary := scanLoop.Get("sf_scan_review_preface")
				state.SetScanReviewSummary(summary)
				if strings.TrimSpace(summary) != "" {
					ctxPath := filepath.Join(auditDirPath, "syntaxflow_scan_context.md")
					if err := os.WriteFile(ctxPath, []byte(summary), 0o644); err != nil {
						log.Warnf("[SFCodeAudit] write scan context file: %v", err)
					} else {
						hasScanContextFile = true
					}
				}
			}
		}

		// Phase 4: report_generating
		reportPath := filepath.Join(auditDirPath, "syntaxflow_code_audit_report.md")
		if err := os.WriteFile(reportPath, []byte(""), 0o644); err != nil {
			log.Warnf("[SFCodeAudit] touch report file: %v", err)
		}
		state.FinalReportPath = reportPath

		var refFiles []string
		if p := state.ReconFilePath; p != "" {
			refFiles = append(refFiles, p)
		}
		if p := state.RuleFilePath; p != "" {
			refFiles = append(refFiles, p)
		}
		if hasScanContextFile {
			refFiles = append(refFiles, filepath.Join(auditDirPath, "syntaxflow_scan_context.md"))
		}

		writePrompt := buildSFCodeAuditReportPrompt(userInput, state, refFiles, hasScanContextFile)
		if err := generateSFCodeAuditReport(r, task, reportPath, writePrompt, refFiles); err != nil {
			log.Warnf("[SFCodeAudit] report_generating: %v", err)
		}

		if content, err := os.ReadFile(reportPath); err != nil || len(strings.TrimSpace(string(content))) == 0 {
			log.Warnf("[SFCodeAudit] report empty or unreadable, writing fallback")
			fb := generateFallbackReport(state, userInput)
			if err := os.WriteFile(reportPath, []byte(fb), 0o644); err != nil {
				log.Warnf("[SFCodeAudit] write fallback report: %v", err)
			}
		}

		writeNextSteps(auditDirPath, state, reportPath)

		r.AddToTimeline("[SF_CODE_AUDIT_DONE]", "SyntaxFlow 代码审计完成。报告: "+reportPath)
		log.Infof("[SFCodeAudit] All phases complete. report=%s", reportPath)
		op.Done()
	}
}
