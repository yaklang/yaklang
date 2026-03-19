package loop_code_security_audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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

			// 获取 AI workdir 并存入 state，所有审计输出文件统一写入此目录
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
		reactloops.WithLoopDescription("代码安全审计模式：四阶段流水线（项目探索→结构化扫描→逐 finding 验证→报告生成）。"),
		reactloops.WithVerboseName("Code Security Audit"),
		reactloops.WithVerboseNameZh("代码安全审计"),
		reactloops.WithLoopUsagePrompt(`当用户需要使用 AI 独立对整个代码项目进行安全审计时使用此流程。流程分四阶段：Phase 1 项目探索 → Phase 2 结构化 Finding 扫描 → Phase 3 逐 Finding 验证 → Phase 4 Markdown 报告生成。`),
		reactloops.WithLoopOutputExample(`
* 当需要进行代码安全审计时：
  {"@action": "code_security_audit", "human_readable_thought": "需要对项目进行全面的安全审计"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, err)
	}
}

// generateFallbackReport 在 Phase4 AI 未生成报告时的兜底实现。
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

// newSubTask 为子 Loop 创建独立的子任务。
func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// buildOrchestratorInitTask 编排四个子 Loop 的 initTask
func buildOrchestratorInitTask(r aicommon.AIInvokeRuntime, state *AuditState) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		log.Infof("[CodeAudit] Orchestrator started. workdir=%s", state.WorkDir)
		userInput := task.GetUserInput()
		r.AddToTimeline("[AUDIT_START]", "代码安全审计开始，用户输入: "+utils.ShrinkTextBlock(userInput, 300))

		// 提前创建 audit 输出目录
		auditDirPath := auditDir(state)
		if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
			log.Warnf("[CodeAudit] Failed to create audit dir %s: %v", auditDirPath, err)
			op.Failed(fmt.Sprintf("[CodeAudit] Fatal err failed to create audit dir %v", err))
			return
		} else {
			log.Infof("[CodeAudit] Audit dir ready: %s", auditDirPath)
			r.AddToTimeline("[AUDIT_DIR]", "审计输出目录: "+auditDirPath)
		}

		// ── Phase 1: 项目探索 ──
		log.Infof("[CodeAudit] Starting Phase 1 (Recon)")
		r.AddToTimeline("[PHASE1_START]", "开始 Phase 1：项目探索")
		reconLoop, err := buildPhase1ReconLoop(r, state)
		if err != nil {
			log.Errorf("[CodeAudit] Failed to build Phase 1 loop: %v", err)
			op.Failed(err)
			return
		}
		if err := reconLoop.ExecuteWithExistedTask(newSubTask(task, "phase1")); err != nil {
			log.Warnf("[CodeAudit] Phase 1 returned error: %v (continuing)", err)
		}

		if state.TechStack == "" {
			log.Warnf("[CodeAudit] Phase 1 ended without calling complete_recon (no tech_stack). " +
				"AI may have exited the loop prematurely. Proceeding with empty recon state.")
			r.AddToTimeline("[PHASE1_INCOMPLETE]",
				"警告：Phase 1 未调用 complete_recon 就结束了。侦察笔记未生成，后续扫描将在缺少背景信息的情况下进行。")
		} else {
			log.Infof("[CodeAudit] Phase 1 complete. tech=%s recon_file=%s", state.TechStack, state.GetReconFilePath())
		}

		// ── Phase 2: 按漏洞类别串行扫描 ──
		log.Infof("[CodeAudit] Starting Phase 2 (Serial category scan, %d categories)", len(DefaultVulnCategories))
		scanLoop, err := buildPhase2AllCategoriesLoop(r, state, nil)
		if err != nil {
			log.Errorf("[CodeAudit] Failed to build Phase 2 loop: %v", err)
			op.Failed(err)
			return
		}
		if err := scanLoop.ExecuteWithExistedTask(newSubTask(task, "phase2")); err != nil {
			log.Warnf("[CodeAudit] Phase 2 returned error: %v (continuing)", err)
		}

		// Phase 2 结束：持久化 findings
		if len(state.GetFindings()) > 0 {
			findingsFile := filepath.Join(auditDirPath, "scan_findings.json")
			if err := state.PersistFindings(findingsFile); err != nil {
				log.Warnf("[CodeAudit] Failed to persist findings: %v", err)
			} else {
				r.AddToTimeline("[PHASE2_PERSISTED]",
					fmt.Sprintf("Phase 2 扫描完成，共 %d 个 finding 已写入: %s",
						len(state.GetFindings()), findingsFile))
				log.Infof("[CodeAudit] Findings persisted to: %s", findingsFile)
			}
		}

		// Phase 2 结束：持久化 scan_observations
		obsFile := filepath.Join(auditDirPath, "scan_observations.md")
		if err := state.PersistScanObservations(obsFile); err != nil {
			log.Warnf("[CodeAudit] Failed to persist scan_observations: %v", err)
		} else if state.GetScanObservationsFilePath() != "" {
			totalUncertain := 0
			for _, obs := range state.GetScanObservations() {
				totalUncertain += obs.UncertainCount
			}
			r.AddToTimeline("[PHASE2_OBSERVATIONS]",
				fmt.Sprintf("Phase 2 扫描观察记录已写入: %s（%d 类别，%d 条 uncertain 线索）",
					obsFile, len(state.GetScanObservations()), totalUncertain))
			log.Infof("[CodeAudit] Scan observations persisted to: %s", obsFile)
		}

		// ── Phase 3: 逐 Finding 验证 ──
		findings := state.GetFindings()
		if len(findings) == 0 {
			r.AddToTimeline("[NO_FINDINGS]", "扫描未发现疑似漏洞，跳过验证阶段。")
			log.Infof("[CodeAudit] No findings, skipping Phase 3")
		} else {
			log.Infof("[CodeAudit] Starting Phase 3 (Verify), %d findings", len(findings))
			r.AddToTimeline("[PHASE3_START]", "开始 Phase 3：逐 Finding 验证")
			verifyLoop, err := buildPhase3VerifyLoop(r, state)
			if err != nil {
				log.Errorf("[CodeAudit] Failed to build Phase 3 loop: %v", err)
				op.Failed(err)
				return
			}
			if err := verifyLoop.ExecuteWithExistedTask(newSubTask(task, "phase3")); err != nil {
				log.Warnf("[CodeAudit] Phase 3 returned error: %v (continuing)", err)
			}

			// Phase 3 结束：持久化 verified_vulns
			if len(state.GetVerifiedVulns()) > 0 {
				verifiedFile := filepath.Join(auditDirPath, "verified_vulns.json")
				if err := state.PersistVerifiedVulns(verifiedFile); err != nil {
					log.Warnf("[CodeAudit] Failed to persist verified_vulns: %v", err)
				} else {
					r.AddToTimeline("[PHASE3_PERSISTED]",
						fmt.Sprintf("Phase 3 验证完成，共 %d 个验证结果已写入: %s",
							len(state.GetVerifiedVulns()), verifiedFile))
					log.Infof("[CodeAudit] VerifiedVulns persisted to: %s", verifiedFile)
				}
			}
		}

		// ── Phase 4: 报告生成 ──
		log.Infof("[CodeAudit] Starting Phase 4 (Report)")
		r.AddToTimeline("[PHASE4_START]", "开始 Phase 4：报告生成")
		reportLoop, err := buildPhase4ReportLoop(r, state)
		if err != nil {
			log.Errorf("[CodeAudit] Failed to build Phase 4 loop: %v", err)
			op.Failed(err)
			return
		}
		if err := reportLoop.ExecuteWithExistedTask(newSubTask(task, "phase4")); err != nil {
			log.Warnf("[CodeAudit] Phase 4 returned error: %v (continuing)", err)
		}

		// 兜底：Phase4 未生成报告时用代码生成基础报告
		if state.GetFinalReport() == "" {
			log.Warnf("[CodeAudit] Phase 4 did not produce report, generating fallback")
			fallbackReport := generateFallbackReport(state)
			state.SetFinalReport(fallbackReport)
			savePath := filepath.Join(auditDirPath, "security_audit_report.md")
			if err := os.WriteFile(savePath, []byte(fallbackReport), 0o644); err != nil {
				log.Warnf("[CodeAudit] Failed to write fallback report: %v", err)
			} else {
				state.SetFinalReportPath(savePath)
				log.Infof("[CodeAudit] Fallback report saved to: %s", savePath)
			}
			r.AddToTimeline("[REPORT_FALLBACK]", "已自动生成基础审计报告: "+savePath)
		}

		finalReport := state.GetFinalReport()
		r.AddToTimeline("[AUDIT_DONE]", "代码安全审计全部完成。报告预览:\n"+utils.ShrinkTextBlock(finalReport, 200))
		log.Infof("[CodeAudit] All phases complete. Report length: %d bytes", len(finalReport))

		op.Done()
	}
}

// auditDir 返回本次审计的输出目录路径
func auditDir(state *AuditState) string {
	if state.WorkDir != "" {
		return filepath.Join(state.WorkDir, "audit")
	}
	// 最后兜底：系统临时目录
	return filepath.Join(os.TempDir(), "code_audit_"+state.ProjectName)
}
