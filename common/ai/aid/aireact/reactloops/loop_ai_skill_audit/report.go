package loop_ai_skill_audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const findingsDetailFilename = "findings_detail.md"

// preferRicherFindingsText keeps the longer findings body (JSON param is often a short summary;
// FINDINGS AITag streams the full markdown into loop vars).
func preferRicherFindingsText(parts ...string) string {
	best := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) > len(best) {
			best = part
		}
	}
	return best
}

// resolveFindingsSummaryFromCompleteAction merges action params and loop AITag capture.
func resolveFindingsSummaryFromCompleteAction(loop *reactloops.ReActLoop, action *aicommon.Action) string {
	fromAction := ""
	if action != nil {
		fromAction = action.GetString("findings_summary")
	}
	fromLoop := ""
	if loop != nil {
		fromLoop = loop.Get("findings_summary")
	}
	return preferRicherFindingsText(fromAction, fromLoop)
}

func persistFindingsDetail(auditWorkDir, content string) string {
	content = strings.TrimSpace(content)
	if content == "" || auditWorkDir == "" {
		return ""
	}
	path := filepath.Join(auditWorkDir, findingsDetailFilename)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		log.Warnf("[SkillAudit] Failed to write %s: %v", path, err)
		return ""
	}
	return path
}

// composeSkillSecurityReport builds the canonical full report from Phase2 outputs.
// This matches what the frontend sees in the FINDINGS / 报告内容 stream.
func composeSkillSecurityReport(state *SkillAuditState) string {
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
	if ts := strings.TrimSpace(state.TechStack); ts != "" {
		sb.WriteString(fmt.Sprintf("> **技术栈**: %s  \n", ts))
	}
	sb.WriteString("\n")

	riskLevel := strings.TrimSpace(state.RiskLevel)
	if riskLevel == "" {
		riskLevel = "Unknown"
	}
	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("**整体风险等级**: **%s**\n\n", riskLevel))

	if table := strings.TrimSpace(state.GetAlignmentTable()); table != "" {
		sb.WriteString("## Intent Alignment Audit\n\n")
		sb.WriteString(table)
		sb.WriteString("\n\n")
	}

	findings := strings.TrimSpace(state.GetFindingsSummary())
	if findings != "" {
		sb.WriteString("## Findings\n\n")
		sb.WriteString(findings)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## Findings\n\nNo findings above Medium threshold detected.\n\n")
	}

	sb.WriteString("## Audit Coverage\n\n")
	sb.WriteString("详见 Phase2 审计笔记与 findings_detail.md。\n")

	return sb.String()
}

// finalizeSkillAuditReport loads Phase3 output from disk and ensures it is at least as complete
// as the Phase2 FINDINGS canonical draft (frontend 报告内容应与终稿一致).
func finalizeSkillAuditReport(state *SkillAuditState, reportPath string) string {
	reportPath = strings.TrimSpace(reportPath)
	canonical := composeSkillSecurityReport(state)
	if reportPath == "" {
		state.SetFinalReport(canonical)
		return canonical
	}

	diskContent := ""
	if raw, err := os.ReadFile(reportPath); err == nil {
		diskContent = strings.TrimSpace(string(raw))
	}

	finalContent := diskContent
	switch {
	case finalContent == "":
		finalContent = canonical
		log.Warnf("[SkillAudit] Phase3 report file empty, using canonical draft (%d bytes)", len(canonical))
	case len(canonical) > 500 && len(finalContent)*10 < len(canonical)*8:
		// Phase3 明显短于 Phase2 FINDINGS 正文 — 用 canonical 保证与前端流式内容一致
		log.Warnf("[SkillAudit] Phase3 report (%d bytes) much shorter than canonical (%d bytes), replacing with canonical",
			len(finalContent), len(canonical))
		finalContent = canonical
	}

	if err := os.WriteFile(reportPath, []byte(finalContent), 0o644); err != nil {
		log.Warnf("[SkillAudit] Failed to write finalized report %s: %v", reportPath, err)
	}

	state.SetFinalReport(finalContent)
	state.SetFinalReportPath(reportPath)
	return finalContent
}

func buildReportPromptWithCanonical(state *SkillAuditState, outputPath, canonicalDraft string) string {
	base := buildReportPrompt(state, outputPath)
	if strings.TrimSpace(canonicalDraft) == "" {
		return base
	}
	return base + fmt.Sprintf(`

## 重要：与 Phase2 报告内容保持一致

以下是由 Phase2 FINDINGS（前端已展示的「报告内容」）生成的**完整初稿**，已写入 report loop 的 full_report_code。
你的任务是润色结构与章节标题，**禁止删减 Findings 中的漏洞细节**；只能补充 Executive Summary / Audit Coverage 等章节，不能缩短 Findings 段落。

初稿长度约 %d 字符。`, len(canonicalDraft))
}
