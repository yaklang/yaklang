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
)

// buildPhase4ReportLoop 构建 Phase 4 报告生成 Loop。
// 直接委托给 report_generating 子 loop，传入审计数据作为参考文件，
// 由专用报告写作 loop 完成 Markdown 报告的生成和磁盘写入。
func buildPhase4ReportLoop(r aicommon.AIInvokeRuntime, state *AuditState, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(2),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(false),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			stats := state.GetStats()
			log.Infof("[CodeAudit/Phase4] Report phase started. confirmed=%d uncertain=%d safe=%d",
				stats.ConfirmedCount, stats.UncertainCount, stats.SafeCount)

			// 兜底：Phase3 未执行任何 conclude_finding 时，自动将 findings 以 uncertain 状态写入
			if len(state.GetVerifiedVulns()) == 0 && len(state.GetFindings()) > 0 {
				for _, f := range state.GetFindings() {
					state.AddVerifiedFinding(&VerifiedFinding{
						Finding:    f,
						Status:     VerifyUncertain,
						Confidence: f.Confidence,
						Reason:     "Phase3 验证阶段未能完成验证（迭代超限或文件路径问题），自动标记为 uncertain，需人工复核。",
						DataFlow:   f.DataFlow,
						Fix:        f.Recommendation,
					})
				}
				r.AddToTimeline("[PHASE4_FALLBACK]",
					fmt.Sprintf("Phase3 未完成验证，已自动将 %d 个 findings 标记为 uncertain。", len(state.GetFindings())))
				log.Warnf("[CodeAudit/Phase4] Auto-promoted %d findings as uncertain", len(state.GetFindings()))
			}

			// 将 verified_vulns 持久化（确保报告 loop 能读到最新文件）
			auditDirPath := auditDir(state)
			if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
				log.Warnf("[CodeAudit/Phase4] Failed to create audit dir: %v", err)
			} else {
				if p := state.GetVerifiedVulnsFilePath(); p == "" {
					vulnsFile := filepath.Join(auditDirPath, "verified_vulns.json")
					if err := state.PersistVerifiedVulns(vulnsFile); err != nil {
						log.Warnf("[CodeAudit/Phase4] Failed to persist verified_vulns: %v", err)
					}
				}
			}

			// 收集可作为参考的审计文件
			var referenceFiles []string
			if p := state.GetVerifiedVulnsFilePath(); p != "" {
				referenceFiles = append(referenceFiles, p)
			}
			if p := state.GetReconFilePath(); p != "" {
				referenceFiles = append(referenceFiles, p)
			}
			if p := state.GetScanObservationsFilePath(); p != "" {
				referenceFiles = append(referenceFiles, p)
			}
			if p := state.GetFindingsFilePath(); p != "" {
				referenceFiles = append(referenceFiles, p)
			}

			// 确定报告输出路径
			reportPath := filepath.Join(auditDirPath, "security_audit_report.md")
			_ = os.WriteFile(reportPath, []byte(""), 0o644)
			state.SetFinalReportPath(reportPath)

			// 构建报告写作任务提示词
			writePrompt := buildAuditReportWritePrompt(state, referenceFiles)

			// 启动 report_generating 子 loop
			reportLoop, err := reactloops.CreateLoopByName(
				schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
				r,
				reactloops.WithMaxIterations(30),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, innerTask aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
					innerLoop.Set("report_filename", reportPath)
					innerLoop.Set("full_report_code", "")
					innerLoop.Set("user_requirements", writePrompt)
					innerLoop.Set("collected_references", "")
					innerLoop.Set("is_modify_mode", "false")

					// 注入参考文件列表
					var filesHint strings.Builder
					filesHint.WriteString("### 审计数据文件（必须读取后再写报告）\n")
					for _, f := range referenceFiles {
						filesHint.WriteString(fmt.Sprintf("- %s\n", f))
					}
					innerLoop.Set("available_files", filesHint.String())
					innerLoop.Set("available_knowledge_bases", "")
					innerOp.Continue()
				}),
			)
			if err != nil {
				log.Warnf("[CodeAudit/Phase4] Failed to create report_generating loop: %v", err)
				r.AddToTimeline("[PHASE4_ERROR]", fmt.Sprintf("报告 loop 创建失败: %v", err))
				op.Done()
				return
			}

			subTask := aicommon.NewSubTaskBase(task, "phase4-audit-report", writePrompt, true)
			if err := reportLoop.ExecuteWithExistedTask(subTask); err != nil {
				log.Warnf("[CodeAudit/Phase4] Report loop returned error: %v", err)
			}

			// 读取写入结果，同步到 state
			if content, err := os.ReadFile(reportPath); err == nil && len(content) > 0 {
				state.SetFinalReport(string(content))
				log.Infof("[CodeAudit/Phase4] Report saved: %s (%d bytes)", reportPath, len(content))
			}

			stats2 := state.GetStats()
			r.AddToTimeline("[PHASE4_COMPLETE]", fmt.Sprintf(
				"Phase 4 报告生成完成。路径: %s\n高危: %d，中危: %d，低危: %d，需人工确认: %d",
				reportPath, stats2.HighCount, stats2.MediumCount, stats2.LowCount, stats2.UncertainCount))

			op.Done()
		}),
	}

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase4_report", r, preset...)
}

// buildAuditReportWritePrompt 构造传给 report_generating loop 的安全审计报告写作任务描述
func buildAuditReportWritePrompt(state *AuditState, referenceFiles []string) string {
	stats := state.GetStats()

	var fileListSB strings.Builder
	for _, f := range referenceFiles {
		fileListSB.WriteString(fmt.Sprintf("- %s\n", f))
	}

	return fmt.Sprintf(`请根据以下审计数据，撰写一份完整的代码安全审计报告（Markdown 格式）。

## 项目信息

- **项目名称**: %s
- **项目路径**: %s
- **技术栈**: %s
- **审计时间**: %s

## 审计统计

| 指标 | 数量 |
|------|------|
| 确认漏洞 (confirmed) | %d |
| 需人工确认 (uncertain) | %d |
| 已排除 (safe) | %d |
| 高危 | %d |
| 中危 | %d |
| 低危 | %d |

## 可用数据文件（**必须在写报告前依次读取**）

%s
- verified_vulns.json：包含每个漏洞的详细验证结果（reason / exploit / fix / data_flow）
- recon_notes.md：项目背景信息（技术栈、路由、认证机制）
- scan_observations.md：各类别扫描覆盖总结
- scan_findings.json：Phase2 原始发现

## 报告结构要求

请生成包含以下章节的完整报告：

1. **执行摘要** — 项目概况、审计范围、核心发现数量
2. **项目背景** — 技术栈、架构、主要入口点（从 recon_notes.md 提取）
3. **漏洞详情**（每个 confirmed 漏洞独立小节）
   - 漏洞标题 + 严重程度
   - 位置（文件:行号）
   - 漏洞描述
   - 数据流路径
   - 攻击场景 / Payload 示例
   - 修复建议（含代码示例）
4. **潜在风险**（uncertain 条目，供人工复核）
5. **安全建议** — 针对本项目的通用安全加固建议
6. **审计局限性** — 静态分析的覆盖范围和局限

## 写作要求

- 漏洞详情必须基于 verified_vulns.json 的实际数据，不要编造
- 每个漏洞的修复建议应包含具体代码示例
- 报告语言：中文
- 格式：标准 Markdown，使用表格、代码块等元素增强可读性
`,
		state.ProjectName,
		state.ProjectPath,
		state.TechStack,
		time.Now().Format("2006-01-02 15:04:05"),
		stats.ConfirmedCount,
		stats.UncertainCount,
		stats.SafeCount,
		stats.HighCount,
		stats.MediumCount,
		stats.LowCount,
		fileListSB.String(),
	)
}
