package loop_code_security_audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			state := NewAuditState()

			cfg := r.GetConfig()
			if c, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
				state.WorkDir = c.GetOrCreateWorkDir()
				log.Infof("[CodeAudit] workdir=%s", state.WorkDir)
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(buildOrchestratorInitTask(r, state)),
			}

			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, r, append(opts, preset...)...)
		},
		reactloops.WithLoopDescription("Code security audit mode: a three-phase pipeline (recon+compile -> SAST scan + LLM logic analysis -> batch check + report)."),
		reactloops.WithLoopDescriptionZh("代码安全审计模式：三阶段流水线（侦察+编译→SAST扫描+LLM逻辑分析→批量Check+报告）。"),
		reactloops.WithVerboseName("Code Security Audit"),
		reactloops.WithVerboseNameZh("代码安全审计"),
		reactloops.WithLoopUsagePrompt(`当用户需要使用 AI 独立对整个代码项目进行安全审计时使用此流程。流程分三阶段：Phase 1 侦察+编译 → Phase 2 SAST扫描+LLM逻辑分析 → Phase 3 批量Check+报告。`),
		reactloops.WithLoopOutputExample(`
* 当需要进行代码安全审计时：
  {"@action": "code_security_audit", "human_readable_thought": "需要对项目进行全面的安全审计"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, err)
	}
}

// newSubTask 为子 Loop 创建独立的子任务。
func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// buildOrchestratorInitTask 编排三个子 Loop 的 initTask
func buildOrchestratorInitTask(r aicommon.AIInvokeRuntime, state *AuditState) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		log.Infof("[CodeAudit] Orchestrator started. workdir=%s", state.WorkDir)

		defer func() {
			os.Unsetenv(aitool.EnvAuditTargetPath)
			os.Unsetenv(aitool.EnvAuditWorkDir)
			log.Infof("[CodeAudit] Cleared audit scope env vars")
		}()
		if state.WorkDir != "" {
			os.Setenv(aitool.EnvAuditWorkDir, state.WorkDir)
		}

		userInput := task.GetUserInput()
		r.AddToTimeline("[AUDIT_START]", "代码安全审计开始，用户输入: "+utils.ShrinkTextBlock(userInput, 300))

		auditDirPath := auditDir(state)
		if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
			op.Failed(fmt.Sprintf("[CodeAudit] Fatal err failed to create audit dir %v", err))
			return
		}
		log.Infof("[CodeAudit] Audit dir ready: %s", auditDirPath)
		r.AddToTimeline("[AUDIT_DIR]", "审计输出目录: "+auditDirPath)

		// ═══════════════════════════════════════════════════════════════
		// Phase 1: 侦察 + 编译配置
		// 目标：确定语言、依赖、入口点、编译方式
		// ═══════════════════════════════════════════════════════════════
		log.Infof("[CodeAudit] Starting Phase 1 (Recon + Compile Config)")
		r.AddToTimeline("[PHASE1_START]", "开始 Phase 1：侦察 + 编译配置")

		scanPath := scanTargetPathFromTask(task)
		if scanPath == "" {
			scanPath = extractPathFromInput(userInput)
		}
		if scanPath == "" {
			op.Failed("[CodeAudit] 无法确定扫描目标路径。请在附件中提供项目目录。")
			return
		}

		// 1a. 确定性预分析（不依赖 LLM）
		preResult := PreAnalyzeProject(scanPath)
		state.SetPreAnalysis(preResult)
		state.SetProjectInfo(scanPath, filepath.Base(scanPath))
		log.Infof("[CodeAudit] Phase 1 pre-analysis: lang=%s, %d deps, %d entries, %d files",
			preResult.Language, len(preResult.RawDeps), len(preResult.EntryPoints), preResult.ProjectScale.TotalFiles)

		// 1b. LLM 目录探索（除非跳过）
		skipPhase1 := strings.Contains(userInput, "[skip-phase1]")
		if !skipPhase1 {
			runDirExplore(r, task, state, scanPath, auditDirPath)
		} else {
			log.Infof("[CodeAudit] Skipping dir_explore (skip-phase1 directive)")
			state.SetReconResult(
				preResult.Language,
				formatEntryPointsSummary(preResult.EntryPoints),
				"",
			)
		}

		os.Setenv(aitool.EnvAuditTargetPath, scanPath)
		r.AddToTimeline("[PHASE1_DONE]", fmt.Sprintf(
			"Phase 1 完成: lang=%s, %d deps, %d entries, %d files, tech=%s",
			preResult.Language, len(preResult.RawDeps), len(preResult.EntryPoints),
			preResult.ProjectScale.TotalFiles, state.TechStack))

		// ═══════════════════════════════════════════════════════════════
		// Phase 2: SAST 扫描 + LLM 逻辑分析
		// SAST：SyntaxFlow lib 规则扫描所有 source/sink
		// LLM：基于 Phase 1 结果分析逻辑漏洞
		// ═══════════════════════════════════════════════════════════════
		log.Infof("[CodeAudit] Starting Phase 2 (SAST + LLM Logic)")
		r.AddToTimeline("[PHASE2_START]", "开始 Phase 2：SAST 扫描 + LLM 逻辑分析")

		// 2a. SAST：SyntaxFlow lib 规则扫描
		skipSFScan := strings.Contains(userInput, "[skip-sf-scan]")
		if !skipSFScan && preResult.Language != "" && preResult.Language != "unknown" {
			log.Infof("[CodeAudit] Phase 2a: SyntaxFlow lib scan for %s (lang=%s)", scanPath, preResult.Language)
			r.AddToTimeline("[SAST_START]", fmt.Sprintf("开始 SAST 扫描（语言: %s）", preResult.Language))
			summary, sfErr := CompileAndScanProject(scanPath, preResult.Language)
			if sfErr != nil {
				log.Warnf("[CodeAudit] SyntaxFlow scan failed: %v (continuing with LLM-only)", sfErr)
				r.AddToTimeline("[SAST_ERROR]", fmt.Sprintf("SAST 扫描失败: %v", sfErr))
			} else {
				state.SetSFScanSummary(summary)
				log.Infof("[CodeAudit] Phase 2a SAST complete: %d/%d rules hit, %d sources, %d sinks",
					summary.HitRules, summary.TotalRules, len(summary.Sources), len(summary.Sinks))
				r.AddToTimeline("[SAST_DONE]", fmt.Sprintf(
					"SAST 扫描完成: %d/%d 规则命中, %d sources, %d sinks",
					summary.HitRules, summary.TotalRules, len(summary.Sources), len(summary.Sinks)))
			}
		} else {
			log.Infof("[CodeAudit] Phase 2a: Skipping SAST scan")
		}

		// 2b. LLM 逻辑漏洞分析
		log.Infof("[CodeAudit] Phase 2b: LLM logic vulnerability analysis")
		r.AddToTimeline("[LLM_LOGIC_START]", "开始 LLM 逻辑漏洞分析")
		runLLMLogicAnalysis(r, task, state, auditDirPath)

		// Phase 2 结束：持久化 findings
		findings := state.GetFindings()
		if len(findings) > 0 {
			findingsFile := filepath.Join(auditDirPath, "scan_findings.json")
			state.PersistFindings(findingsFile)
			r.AddToTimeline("[PHASE2_PERSISTED]",
				fmt.Sprintf("Phase 2 完成，共 %d 个 finding 已写入", len(findings)))
		}
		log.Infof("[CodeAudit] Phase 2 complete: %d findings", len(findings))

		// ═══════════════════════════════════════════════════════════════
		// Phase 3: 批量 Check + 报告
		// 对 Phase 2 发现的每条 source→sink 连接验证可利用性
		// ═══════════════════════════════════════════════════════════════
		log.Infof("[CodeAudit] Starting Phase 3 (Batch Check + Report)")
		r.AddToTimeline("[PHASE3_START]", "开始 Phase 3：批量 Check + 报告")
		runBatchCheckAndReport(r, task, state, auditDirPath)

		// 持久化最终结果
		verifiedVulns := state.GetVerifiedVulns()
		if len(verifiedVulns) > 0 {
			verifiedFile := filepath.Join(auditDirPath, "verified_vulns.json")
			state.PersistVerifiedVulns(verifiedFile)
		}

		// 兜底报告
		if state.GetFinalReport() == "" {
			fallbackReport := generateFallbackReport(state)
			state.SetFinalReport(fallbackReport)
			savePath := filepath.Join(auditDirPath, "security_audit_report.md")
			os.WriteFile(savePath, []byte(fallbackReport), 0o644)
			state.SetFinalReportPath(savePath)
		}

		r.AddToTimeline("[AUDIT_DONE]", "代码安全审计全部完成。报告预览:\n"+
			utils.ShrinkTextBlock(state.GetFinalReport(), 200))
		log.Infof("[CodeAudit] All phases complete. Report length: %d bytes", len(state.GetFinalReport()))

		op.Done()
	}
}

// runDirExplore 运行 Phase 1 的目录探索子 loop
func runDirExplore(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *AuditState, scanPath, auditDirPath string) {
	log.Infof("[CodeAudit] Starting dir_explore for %s", scanPath)
	r.AddToTimeline("[DIR_EXPLORE_START]", "开始目录探索")

	reconFilePath := filepath.Join(auditDirPath, "recon_notes.md")
	exploreOpts := []reactloops.ReActLoopOption{
		reactloops.WithVar("output_report_path", reconFilePath),
		reactloops.WithVar("explore_work_dir", auditDirPath),
		reactloops.WithVar("target_path", scanPath),
	}
	if state.PreAnalysis != nil {
		exploreOpts = append(exploreOpts,
			reactloops.WithVar("pre_analysis_hint", state.PreAnalysisPrompt))
	}

	exploreLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DIR_EXPLORE, r, exploreOpts...)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to create dir_explore loop: %v", err)
		return
	}
	if err := exploreLoop.ExecuteWithExistedTask(newSubTask(task, "phase1-explore")); err != nil {
		log.Warnf("[CodeAudit] dir_explore returned error: %v (continuing)", err)
	}

	// 回填结果
	if projectPath := exploreLoop.Get("result_target_path"); projectPath != "" {
		state.SetProjectInfo(projectPath, exploreLoop.Get("result_project_name"))
	}
	if techStack := exploreLoop.Get("result_tech_stack"); techStack != "" {
		state.SetReconResult(techStack, exploreLoop.Get("result_entry_points"), "")
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
	log.Infof("[CodeAudit] dir_explore complete. tech=%s", state.TechStack)
}

// runLLMLogicAnalysis 运行 Phase 2b 的 LLM 逻辑漏洞分析
func runLLMLogicAnalysis(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *AuditState, auditDirPath string) {
	logicLoop, err := buildLLMLogicAnalysisLoop(r, state)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to build LLM logic loop: %v", err)
		return
	}
	if err := logicLoop.ExecuteWithExistedTask(newSubTask(task, "phase2-llm")); err != nil {
		log.Warnf("[CodeAudit] LLM logic analysis returned error: %v (continuing)", err)
	}

	// 持久化 LLM 发现的逻辑漏洞
	logicVulns := state.GetFindings()
	if len(logicVulns) > 0 {
		logicFile := filepath.Join(auditDirPath, "logic_findings.json")
		state.PersistFindings(logicFile)
		log.Infof("[CodeAudit] LLM logic analysis: %d findings persisted", len(logicVulns))
	}
}

// runBatchCheckAndReport 运行 Phase 3：批量 Check + 报告生成
func runBatchCheckAndReport(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *AuditState, auditDirPath string) {
	checkLoop, err := buildBatchCheckLoop(r, state)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to build batch check loop: %v", err)
		return
	}
	if err := checkLoop.ExecuteWithExistedTask(newSubTask(task, "phase3")); err != nil {
		log.Warnf("[CodeAudit] Batch check returned error: %v (continuing)", err)
	}
}

// auditDir 返回本次审计的输出目录路径
func auditDir(state *AuditState) string {
	if state.WorkDir != "" {
		return filepath.Join(state.WorkDir, "audit")
	}
	return filepath.Join(os.TempDir(), "code_audit_"+state.ProjectName)
}

// extractPathFromInput 从用户输入中提取可能的目录路径
func extractPathFromInput(input string) string {
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, field := range strings.Fields(line) {
			field = strings.Trim(field, "'\",;)")
			if strings.HasPrefix(field, "/") {
				if st, err := os.Stat(field); err == nil && st.IsDir() {
					return field
				}
			}
		}
	}
	return ""
}

// formatEntryPointsSummary 将入口点列表格式化为一行摘要
func formatEntryPointsSummary(entries []EntryPointInfo) string {
	if len(entries) == 0 {
		return ""
	}
	var parts []string
	for _, ep := range entries {
		parts = append(parts, fmt.Sprintf("%s:%d (%s)", filepath.Base(ep.File), ep.Line, ep.Type))
	}
	return strings.Join(parts, "; ")
}

// generateFallbackReport 生成兜底报告
func generateFallbackReport(state *AuditState) string {
	var sb strings.Builder
	now := time.Now().Format("2006-01-02 15:04:05")

	sb.WriteString(fmt.Sprintf("# %s 安全审计报告\n\n", state.ProjectName))
	sb.WriteString(fmt.Sprintf("> 审计时间: %s  \n", now))
	sb.WriteString(fmt.Sprintf("> 项目路径: %s  \n", state.ProjectPath))
	sb.WriteString(fmt.Sprintf("> 技术栈: %s  \n\n", state.TechStack))

	stats := state.GetStats()
	sb.WriteString("## 摘要\n\n")
	sb.WriteString("| 指标 | 数量 |\n|------|------|\n")
	sb.WriteString(fmt.Sprintf("| 扫描 Finding 数 | %d |\n", stats.TotalFindings))
	sb.WriteString(fmt.Sprintf("| 高危 (HIGH) | %d |\n", stats.HighCount))
	sb.WriteString(fmt.Sprintf("| 中危 (MEDIUM) | %d |\n", stats.MediumCount))
	sb.WriteString(fmt.Sprintf("| 低危 (LOW) | %d |\n", stats.LowCount))
	sb.WriteString(fmt.Sprintf("| 需人工确认 | %d |\n", stats.UncertainCount))
	sb.WriteString(fmt.Sprintf("| 已排除（安全）| %d |\n\n", stats.SafeCount))

	vulns := state.GetVerifiedVulns()
	if len(vulns) == 0 {
		sb.WriteString("## 漏洞详情\n\n本次审计未发现高置信度漏洞。\n")
		return sb.String()
	}

	sb.WriteString("## 漏洞详情\n\n")
	for i, vf := range vulns {
		if vf.Status == VerifySafe {
			continue
		}
		f := vf.Finding
		if f == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %d. %s (%s)\n\n", i+1, f.Title, f.Severity))
		sb.WriteString(fmt.Sprintf("- **漏洞类型**: %s\n", f.Category))
		sb.WriteString(fmt.Sprintf("- **文件**: `%s`（行 %d）\n", f.File, f.Line))
		sb.WriteString(fmt.Sprintf("- **置信度**: %d/10\n", vf.Confidence))
		sb.WriteString(fmt.Sprintf("- **验证状态**: %s\n", string(vf.Status)))
		if f.Description != "" {
			sb.WriteString(fmt.Sprintf("\n**描述**: %s\n", f.Description))
		}
		if vf.DataFlow != "" {
			sb.WriteString(fmt.Sprintf("\n**数据流**: `%s`\n", vf.DataFlow))
		}
		if vf.Reason != "" {
			sb.WriteString(fmt.Sprintf("\n**验证分析**: %s\n", vf.Reason))
		}
		if vf.Exploit != "" {
			sb.WriteString(fmt.Sprintf("\n**利用方式**: %s\n", vf.Exploit))
		}
		if vf.Fix != "" {
			sb.WriteString(fmt.Sprintf("\n**修复建议**: %s\n", vf.Fix))
		}
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}
