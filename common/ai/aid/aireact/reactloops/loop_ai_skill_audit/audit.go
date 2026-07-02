package loop_ai_skill_audit

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	skillAuditPhase1NodeID = "skill-audit-phase1"
	skillAuditPhase2NodeID = "skill-audit-phase2"
	skillAuditReportNodeID = "skill-audit-report"
)

//go:embed prompts/persistent_instruction.txt
var auditInstruction string

//go:embed prompts/output_example.txt
var auditOutputExample string

// reactive data template for the Phase 2 static analysis loop
const auditReactiveDataTpl = `## 当前审计状态
<|SKILL_AUDIT_STATUS_{{ .Nonce }}|>
[路径规范] 所有工具调用必须使用绝对路径

**Skill 路径**: {{ .SkillPath }}
{{ if .HasSelectionFocus }}**用户选中片段**（Phase2 第一优先级，必须先分析）:
{{ .Selection }}
{{ else if .HasOpenFileFocus }}**前端打开文件**: {{ .FocusFilePath }}（优先审计此文件及其关联脚本）
{{ end }}
**审计进度**: 已执行 {{ .IterationCount }} 次操作
{{ if .NoteFiles }}**已写出审计笔记文件（{{ .NoteFileCount }} 个）**:
{{ .NoteFiles }}{{ else }}尚未写出任何审计笔记（建议先写 skill_audit_notes.md）{{ end }}
{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|SKILL_AUDIT_STATUS_END_{{ .Nonce }}|>

[终止规则] complete_skill_audit 是本 loop 退出的唯一合法方式。调用前必须：
1. 已用 read_file 读取 SKILL.md
{{ if .HasSelectionFocus }}2. 已重点分析用户选中的代码片段（文件: {{ .FocusFilePath }}）
3. 已完成全部 6 个检测类别的 grep 扫描
4. 已用 write_file 写出至少一个审计笔记文件{{ else }}2. 已完成全部 6 个检测类别的 grep 扫描
3. 已用 write_file 写出至少一个审计笔记文件{{ end }}`

// BuildSkillAuditLoop constructs the orchestrator loop for AI Skill security auditing.
// It runs three phases sequentially inside InitTask:
//
//	Phase 1: directory exploration (delegates to dir_explore sub-loop)
//	Phase 2: static security analysis (specialized loop with FS tools + audit actions)
//	Phase 3: report generation (delegates to report_generating sub-loop)
func BuildSkillAuditLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	state := NewSkillAuditState()

	cfg := r.GetConfig()
	if c, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
		state.WorkDir = c.GetOrCreateWorkDir()
		log.Infof("[SkillAudit] workdir=%s", state.WorkDir)
	}

	preset := []reactloops.ReActLoopOption{
		reactloops.WithInitTask(buildOrchestratorInitTask(r, state)),
	}

	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_AI_SKILL_AUDIT, r, append(opts, preset...)...)
}

// buildOrchestratorInitTask drives the three-phase pipeline.
func buildOrchestratorInitTask(r aicommon.AIInvokeRuntime, state *SkillAuditState) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := task.GetUserInput()

		ws := reactloops.InitWorkspaceAttachedContext(r, loop, task, AttachedResourceKeySkillAuditTargetPath)
		reactloops.RecordWorkspaceAttachedTimeline(r, ws, "SKILL_AUDIT")
		if ws != nil {
			var sel *aicommon.AttachedCodeSelection
			if ws.Selection != nil {
				copied := *ws.Selection
				sel = &copied
			}
			if ws.FilePath != "" || sel != nil {
				state.SetFrontendFocus(ws.FilePath, sel)
			}
		}

		reactloops.EmitStatus(loop, "AI Skill 安全审计启动 / Starting AI Skill security audit")
		r.AddToTimeline("[SKILL_AUDIT_START]", "AI Skill 安全审计开始，用户输入: "+utils.ShrinkTextBlock(userInput, 300))

		// ── Phase 1: 目录探索（委托给 dir_explore loop）──
		log.Infof("[SkillAudit] Starting Phase 1 (Recon via dir_explore)")
		reactloops.EmitActionLog(loop, skillAuditPhase1NodeID, "Phase 1：Skill 目录探索 / Phase 1: Skill directory exploration")
		reactloops.EmitStatus(loop, "Phase 1：目录探索中 / Phase 1: Directory exploration...")
		r.AddToTimeline("[PHASE1_START]", "Phase 1：Skill 目录探索")

		auditDirPath := skillAuditDir(state)
		if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
			log.Warnf("[SkillAudit] Failed to create audit dir %s: %v", auditDirPath, err)
			op.Failed(fmt.Errorf("[SkillAudit] failed to create audit dir: %w", err))
			return
		}

		reconFilePath := filepath.Join(auditDirPath, "recon_notes.md")
		exploreOpts := []reactloops.ReActLoopOption{
			reactloops.WithVar("output_report_path", reconFilePath),
			reactloops.WithVar("explore_work_dir", auditDirPath),
		}
		if ws != nil {
			if scanPath := ws.ResolveAttachedScanDirectory(); scanPath != "" {
				if err := reactloops.ValidateAttachedDirectoryTarget(scanPath); err != nil {
					log.Warnf("[SkillAudit] attached target path not accessible: %q: %v", scanPath, err)
					op.Failed(fmt.Errorf(
						"[SkillAudit] %s",
						reactloops.FormatAttachedDirectoryValidationError(scanPath, AttachedResourceKeySkillAuditTargetPath, err)))
					return
				}
				exploreOpts = reactloops.WithExploreTargetPath(exploreOpts, scanPath)
				log.Infof("[SkillAudit] Phase1 using attached scan target: %s", scanPath)
				reactloops.RecordExploreTargetTimeline(r, ws, scanPath, "SKILL_AUDIT")
			}
		}
		exploreLoop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_DIR_EXPLORE,
			r,
			exploreOpts...,
		)
		if err != nil {
			log.Errorf("[SkillAudit] Failed to create dir_explore loop: %v", err)
			op.Failed(err)
			return
		}
		if err := exploreLoop.ExecuteWithExistedTask(newSubTask(task, "phase1")); err != nil {
			log.Warnf("[SkillAudit] Phase 1 (dir_explore) returned error: %v (continuing)", err)
		}

		// Backfill state from dir_explore results
		if skillPath := exploreLoop.Get("result_target_path"); skillPath != "" {
			skillName := exploreLoop.Get("result_project_name")
			if skillName == "" {
				skillName = filepath.Base(skillPath)
			}
			state.SetProjectInfo(skillPath, skillName)
		}
		if techStack := exploreLoop.Get("result_tech_stack"); techStack != "" {
			state.SetReconResult(techStack, exploreLoop.Get("result_entry_points"))
		}
		if reportPath := exploreLoop.Get("result_report_path"); reportPath != "" {
			state.SetReconFilePath(reportPath)
		}
		if noteFilesStr := exploreLoop.Get("result_note_files"); noteFilesStr != "" {
			for _, f := range strings.Split(noteFilesStr, "\n") {
				if f = strings.TrimSpace(f); f != "" {
					state.AddReconNoteFile(f)
				}
			}
		}

		// Validate that the target is a proper AI Skill (has SKILL.md)
		skillMDPath := filepath.Join(state.SkillPath, "SKILL.md")
		if _, err := os.Stat(skillMDPath); err != nil {
			log.Warnf("[SkillAudit] SKILL.md not found at %s: %v. Proceeding anyway.", skillMDPath, err)
			r.AddToTimeline("[SKILL_MD_MISSING]",
				fmt.Sprintf("警告：在 %s 未找到 SKILL.md。此目录可能不是标准 AI Skill，审计仍将继续。", state.SkillPath))
		}

		log.Infof("[SkillAudit] Phase 1 complete. skill=%s path=%s", state.SkillName, state.SkillPath)
		reactloops.EmitStatus(loop, "Phase 1 完成 / Phase 1 complete")
		reactloops.EmitActionLog(loop, skillAuditPhase1NodeID,
			fmt.Sprintf("完成: skill=%s, path=%s, recon_files=%d",
				state.SkillName, state.SkillPath, len(state.GetReconNoteFiles())))
		r.AddToTimeline("[PHASE1_COMPLETE]", fmt.Sprintf(
			"Phase 1 完成: skill=%s, path=%s, recon_report=%s, note_files=%d",
			state.SkillName, state.SkillPath, state.ReconFilePath, len(state.GetReconNoteFiles())))

		// ── Phase 2: 静态安全分析 ──
		log.Infof("[SkillAudit] Starting Phase 2 (Static security analysis)")
		reactloops.EmitActionLog(loop, skillAuditPhase2NodeID, "Phase 2：静态安全分析 / Phase 2: Static security analysis")
		reactloops.EmitStatus(loop, "Phase 2：静态分析中 / Phase 2: Static analysis...")
		r.AddToTimeline("[PHASE2_START]", "Phase 2：AI Skill 静态安全分析")

		analysisLoop, err := buildPhase2StaticAnalysisLoop(r, state, auditDirPath)
		if err != nil {
			log.Errorf("[SkillAudit] Failed to build Phase 2 loop: %v", err)
			op.Failed(err)
			return
		}
		if err := analysisLoop.ExecuteWithExistedTask(newSubTask(task, "phase2")); err != nil {
			log.Warnf("[SkillAudit] Phase 2 returned error: %v (continuing)", err)
		}

		log.Infof("[SkillAudit] Phase 2 complete. risk=%s", state.RiskLevel)
		reactloops.EmitStatus(loop, "Phase 2 完成 / Phase 2 complete")
		reactloops.EmitActionLog(loop, skillAuditPhase2NodeID,
			fmt.Sprintf("完成: risk=%s, audit_notes=%d", state.RiskLevel, len(state.GetAuditNoteFiles())))

		// ── Phase 3: 报告生成 ──
		log.Infof("[SkillAudit] Starting Phase 3 (Report generation)")
		reactloops.EmitActionLog(loop, skillAuditReportNodeID, "Phase 3：安全报告生成 / Phase 3: Security report generation")
		reactloops.EmitStatus(loop, "Phase 3：报告生成中 / Phase 3: Generating report...")
		r.AddToTimeline("[PHASE3_START]", "Phase 3：安全报告生成")

		reportPath := filepath.Join(auditDirPath, "skill_security_report.md")
		canonicalReport := composeSkillSecurityReport(state)
		reportLoop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
			r,
			reactloops.WithMaxIterations(math.MaxInt32),
			reactloops.WithAllowUserInteract(false),
			reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, _ aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
				innerLoop.Set("report_filename", reportPath)
				innerLoop.Set("full_report_code", canonicalReport)
				innerLoop.Set("user_requirements", buildReportPromptWithCanonical(state, reportPath, canonicalReport))
				innerLoop.Set("available_files", buildAvailableFilesHint(state))
				innerLoop.Set("available_knowledge_bases", "")
				innerLoop.Set("collected_references", "")
				innerLoop.Set("is_modify_mode", "true")
				innerOp.Continue()
			}),
		)
		if err != nil {
			log.Errorf("[SkillAudit] Failed to create report_generating loop: %v", err)
			op.Failed(err)
			return
		}
		if err := reportLoop.ExecuteWithExistedTask(newSubTask(task, "phase3")); err != nil {
			log.Warnf("[SkillAudit] Phase 3 returned error: %v (continuing)", err)
		}

		finalReport := finalizeSkillAuditReport(state, reportPath)
		if err := reactloops.SaveAndPinFile(loop, reportPath, []byte(finalReport)); err != nil {
			log.Warnf("[SkillAudit] Failed to pin final report: %v", err)
		}

		reportPath = state.FinalReportPath
		if reportPath == "" {
			reportPath = filepath.Join(auditDirPath, "skill_security_report.md")
		}
		reactloops.EmitStatus(loop, "审计完成 / Audit complete")
		reactloops.EmitActionLog(loop, skillAuditReportNodeID,
			fmt.Sprintf("完成: report=%s, risk=%s", reportPath, state.RiskLevel))

		r.AddToTimeline("[SKILL_AUDIT_DONE]",
			fmt.Sprintf("AI Skill 安全审计完成。Skill: %s | 风险等级: %s | 报告: %s",
				state.SkillName, state.RiskLevel, reportPath))
		log.Infof("[SkillAudit] All phases complete. skill=%s risk=%s", state.SkillName, state.RiskLevel)

		op.Done()
	}
}

// buildPhase2StaticAnalysisLoop constructs the Phase 2 static analysis loop.
// The loop has access to FS tools (tree, read_file, grep, find_file, write_file) and
// a custom complete_skill_audit action to finalize findings.
func buildPhase2StaticAnalysisLoop(r aicommon.AIInvokeRuntime, state *SkillAuditState, auditWorkDir string) (*reactloops.ReActLoop, error) {
	// Track audit note files written by the AI
	var noteFiles []string
	addNoteFile := func(path string) {
		for _, f := range noteFiles {
			if f == path {
				return
			}
		}
		noteFiles = append(noteFiles, path)
		state.AddAuditNoteFile(path)
	}

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(true),
		reactloops.WithSameActionTypeSpinThreshold(5),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithMaxConsecutiveSpinWarnings(2),
		reactloops.WithAITagFieldWithAINodeId("FINDINGS", "findings_summary", "skill-audit-findings", aicommon.TypeTextMarkdown),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		// Persistent instruction injected each round
		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			vars := map[string]any{
				"Nonce":         nonce,
				"SkillPath":     state.SkillPath,
				"AuditWorkDir":  auditWorkDir,
				"TechStack":     state.TechStack,
				"ReconFilePath": state.ReconFilePath,
			}
			for k, v := range reactloops.FocusPromptVars(state.GetFocusFilePath(), state.GetSelection()) {
				vars[k] = v
			}
			return utils.RenderTemplate(auditInstruction, vars)
		}),
		reactloops.WithReflectionOutputExample(auditOutputExample),

		// Reactive data: current progress snapshot
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			iterCount := loop.GetCurrentIterationIndex()
			noteFilesList := ""
			for _, f := range noteFiles {
				noteFilesList += "  - " + f + "\n"
			}
			vars := map[string]any{
				"Nonce":            nonce,
				"SkillPath":        state.SkillPath,
				"IterationCount":   iterCount,
				"NoteFiles":        noteFilesList,
				"NoteFileCount":    len(noteFiles),
				"FeedbackMessages": feedbacker.String(),
			}
			for k, v := range reactloops.FocusPromptVars(state.GetFocusFilePath(), state.GetSelection()) {
				vars[k] = v
			}
			return utils.RenderTemplate(auditReactiveDataTpl, vars)
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			if state.HasFrontendFocus() {
				focusPath := reactloops.ResolveFocusFilePath(state.GetFocusFilePath(), state.GetSelection())
				if state.GetSelection() != nil && strings.TrimSpace(state.GetSelection().Content) != "" {
					reactloops.EmitStatus(loop, "选中片段优先分析 / Selection-first static analysis")
					r.AddToTimeline("[SKILL_AUDIT_FOCUS]",
						fmt.Sprintf("Phase2 选中片段优先: %s", utils.ShrinkTextBlock(state.GetSelection().Content, 200)))
				} else if focusPath != "" {
					reactloops.EmitStatus(loop, "打开文件优先审计 / Open-file focused analysis")
					r.AddToTimeline("[SKILL_AUDIT_FOCUS]", "Phase2 优先审计打开文件: "+focusPath)
				}
			} else {
				reactloops.EmitStatus(loop, "静态安全分析就绪 / Static security analysis ready")
			}
			log.Infof("[SkillAudit/Phase2] Static analysis started. skill_path=%s focus=%v", state.SkillPath, state.HasFrontendFocus())
			op.Continue()
		}),
	}

	// ─── Register FS tools ───
	preset = append(preset, buildFSToolAction(r, "tree", nil))
	preset = append(preset, buildFSToolAction(r, "read_file", nil))
	preset = append(preset, buildFSToolAction(r, "grep", nil))
	preset = append(preset, buildFSToolAction(r, "find_file", nil))
	preset = append(preset, buildFSToolAction(r, "write_file", func(action *aicommon.Action) {
		if filePath := action.GetString("file"); filePath != "" {
			addNoteFile(filePath)
			log.Infof("[SkillAudit/Phase2] Audit note written: %s", filePath)
		}
	}))

	// ─── complete_skill_audit: the only legal exit from Phase 2 ───
	// findings_summary content is supplied via <FINDINGS>...</FINDINGS> AITag (loop-level
	// registration above), which streams Markdown to the frontend without JSON escaping.
	preset = append(preset, reactloops.WithRegisterLoopAction(
		"complete_skill_audit",
		"完成 AI Skill 静态安全审计，提交意图一致性审计表和风险等级（JSON 字段），漏洞详情通过 FINDINGS AITag 输出（Markdown 格式）。调用前必须已完成全部 6 个检测类别的扫描并写出审计笔记文件。",
		[]aitool.ToolOption{
			aitool.WithStringParam("skill_name",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Skill 名称（从 SKILL.md name 字段提取）")),
			aitool.WithStringParam("risk_level",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("整体风险等级：Clean / Medium / High / Critical")),
			aitool.WithStringParam("alignment_table",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("意图一致性审计表（Markdown 表格格式），包含恶意行为、隐形行为、功能意图三个检查项")),
			aitool.WithStringParam("findings_summary",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("审计结论与漏洞详情摘要")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if len(noteFiles) == 0 {
				return utils.Error(
					"[complete_skill_audit 被拒绝] 尚未写出任何审计笔记文件。" +
						"请先用 write_file 将审计过程记录到工作目录（如 skill_audit_notes.md），再调用 complete_skill_audit。",
				)
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			skillName := action.GetString("skill_name")
			riskLevel := action.GetString("risk_level")
			alignmentTable := action.GetString("alignment_table")
			findingsSummary := resolveFindingsSummaryFromCompleteAction(loop, action)

			if skillName == "" {
				skillName = state.SkillName
			}
			if skillName == "" && state.SkillPath != "" {
				skillName = filepath.Base(state.SkillPath)
			}

			state.SetProjectInfo(state.SkillPath, skillName)
			state.SetAuditResult(riskLevel, alignmentTable, findingsSummary)

			if detailPath := persistFindingsDetail(auditWorkDir, findingsSummary); detailPath != "" {
				addNoteFile(detailPath)
				log.Infof("[SkillAudit/Phase2] Findings detail saved: %s (%d bytes)", detailPath, len(findingsSummary))
			}

			auditResultPath := filepath.Join(auditWorkDir, "audit_result.md")
			auditContent := fmt.Sprintf("# AI Skill 安全审计结果\n\n"+
				"**Skill 名称**: %s\n**Skill 路径**: %s\n**风险等级**: %s\n\n"+
				"## 意图一致性审计\n\n%s\n\n"+
				"## 审计结论与漏洞详情\n\n%s\n",
				skillName, state.SkillPath, riskLevel,
				alignmentTable, findingsSummary)
			if err := os.WriteFile(auditResultPath, []byte(auditContent), 0o644); err != nil {
				log.Warnf("[SkillAudit/Phase2] Failed to persist audit result: %v", err)
			} else {
				addNoteFile(auditResultPath)
				r.AddToTimeline("[AUDIT_RESULT_SAVED]", "审计结果已写入: "+auditResultPath)
				log.Infof("[SkillAudit/Phase2] Audit result saved: %s", auditResultPath)
			}

			findingsPreview := utils.ShrinkTextBlock(findingsSummary, 500)
			r.AddToTimeline("[PHASE2_COMPLETE]",
				fmt.Sprintf("Phase 2 静态分析完成。Skill: %s | 风险等级: %s | 结果文件: %s\n摘要: %s",
					skillName, riskLevel, auditResultPath, findingsPreview))
			log.Infof("[SkillAudit/Phase2] Complete. skill=%s risk=%s", skillName, riskLevel)

			op.Feedback(fmt.Sprintf("审计完成。风险等级: %s\n\n%s", riskLevel, findingsPreview))
			op.Exit()
		},
	))

	return reactloops.NewReActLoop("skill_audit_static_analysis", r, preset...)
}

// buildFSToolAction registers a file-system tool as a loop action.
// It mirrors the implementation in loop_dir_explore to avoid cross-package coupling.
func buildFSToolAction(r aicommon.AIInvokeRuntime, toolName string, onAction func(action *aicommon.Action)) reactloops.ReActLoopOption {
	toolMgr := r.GetConfig().GetAiToolManager()
	if toolMgr == nil {
		log.Warnf("[SkillAudit] tool manager not available, skip %q action", toolName)
		return func(r *reactloops.ReActLoop) {}
	}
	tool, err := toolMgr.GetToolByName(toolName)
	if err != nil || tool == nil {
		log.Warnf("[SkillAudit] tool %q not found: %v", toolName, err)
		return func(r *reactloops.ReActLoop) {}
	}

	return reactloops.WithRegisterLoopAction(
		toolName,
		tool.GetDescription(),
		tool.BuildParamsOptions(),
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			params := action.GetParams()
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
			if err != nil {
				log.Warnf("[SkillAudit] tool %q failed: %v", toolName, err)
				op.Feedback(fmt.Sprintf("[工具执行失败] %s: %v，请尝试其他方法。", toolName, err))
				op.Continue()
				return
			}

			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}
			summary, _ := reactloops.SpillLongContent(loop, toolName, content)
			invoker.AddToTimeline(fmt.Sprintf("[%s]", toolName),
				fmt.Sprintf("%s 完成: %d bytes\n%s", toolName, len(content), summary))

			op.Feedback(fmt.Sprintf("[%s 完成] %d bytes\n%s", toolName, len(content), summary))
			op.Continue()

			if onAction != nil {
				onAction(action)
			}
		},
	)
}

// skillAuditDir returns the output directory for audit artifacts.
func skillAuditDir(state *SkillAuditState) string {
	if state.WorkDir != "" {
		return filepath.Join(state.WorkDir, "skill_audit")
	}

	return filepath.Join(os.TempDir(), "skill_audit_"+utils.RandAlphaNumStringBytes(5))
}

// newSubTask creates an independent sub-task for a child loop.
func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// buildAvailableFilesHint constructs the reference file list hint for the report generator.
func buildAvailableFilesHint(state *SkillAuditState) string {
	var allFiles []string
	allFiles = append(allFiles, state.GetReconNoteFiles()...)
	allFiles = append(allFiles, state.GetAuditNoteFiles()...)
	if len(allFiles) == 0 {
		return "（未写出任何参考文件）"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### 参考文件（共 %d 个，必须全部读取后再开始写报告）\n", len(allFiles)))
	for i, f := range allFiles {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}
	sb.WriteString("\n> [强制] 在调用 write_section 之前，必须对以上每一个文件都调用 read_reference_file。\n")
	return sb.String()
}

// buildReportPrompt constructs the writing task description for the report_generating loop.
func buildReportPrompt(state *SkillAuditState, outputPath string) string {
	focusSection := ""
	if state.HasFrontendFocus() {
		focusPath := reactloops.ResolveFocusFilePath(state.GetFocusFilePath(), state.GetSelection())
		focusSection = fmt.Sprintf(`
## 前端聚焦上下文
- **优先审计文件**: %s
%s
报告 Findings 章节必须包含对上述聚焦内容/analysis 的结论（如有发现）。
`, focusPath, reactloops.FormatAttachedCodeSelection(state.GetSelection()))
	}

	return fmt.Sprintf(`请根据以下 AI Skill 安全审计结果生成结构化的 Markdown 安全报告。

## Skill 信息
- **Skill 名称**: %s
- **Skill 路径**: %s
- **技术栈**: %s
- **整体风险等级**: %s
%s
## 审计结论与漏洞详情
%s

## 报告结构要求

报告必须包含以下章节：
1. **Executive Summary**（执行摘要）：2-3 句话描述 Skill 声明功能、发现情况和整体风险结论
2. **Intent Alignment Audit**（意图一致性审计）：从参考文件中提取意图一致性审计表
3. **Project Overview**（项目概览）：Skill 名称、声明功能、文件列表、脚本清单
4. **Findings**（发现）：将漏洞详情整合为 Markdown 小节；无发现时写 "No findings above Medium threshold detected."
5. **Audit Coverage**（审计覆盖范围）：按六个类别（网络/反弹Shell/后门/敏感文件/混淆/挖矿）列出检查结果

## 重要说明
- 必须先用 read_reference_file 读取所有参考文件再开始写作
- 输出文件: %s
- 使用 Markdown 格式，用 write_section 写入`,
		state.SkillName,
		state.SkillPath,
		state.TechStack,
		state.RiskLevel,
		focusSection,
		state.GetFindingsSummary(),
		outputPath,
	)
}

// generateFallbackReport produces a basic Markdown report from state when Phase 3 fails.
func generateFallbackReport(state *SkillAuditState) string {
	var sb strings.Builder
	now := time.Now().Format("2006-01-02 15:04:05")

	skillName := state.SkillName
	if skillName == "" {
		skillName = filepath.Base(state.SkillPath)
	}
	if skillName == "" {
		skillName = "Unknown Skill"
	}

	sb.WriteString(fmt.Sprintf("# AI Skill 安全审计报告：%s\n\n", skillName))
	sb.WriteString(fmt.Sprintf("> **审计时间**: %s  \n", now))
	sb.WriteString(fmt.Sprintf("> **Skill 路径**: %s  \n", state.SkillPath))
	sb.WriteString(fmt.Sprintf("> **技术栈**: %s  \n\n", state.TechStack))

	riskLevel := state.RiskLevel
	if riskLevel == "" {
		riskLevel = "Unknown"
	}
	sb.WriteString(fmt.Sprintf("## 执行摘要\n\n**整体风险等级**: **%s**\n\n", riskLevel))

	if state.GetFindingsSummary() != "" {
		sb.WriteString("## 审计结论与漏洞详情\n\n")
		sb.WriteString(state.GetFindingsSummary() + "\n")
	} else {
		sb.WriteString("## 漏洞发现\n\nNo findings above Medium threshold detected.\n")
	}

	return sb.String()
}
