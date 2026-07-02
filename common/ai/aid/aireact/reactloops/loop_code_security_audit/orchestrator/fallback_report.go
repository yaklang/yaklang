package orchestrator

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

// GenerateFallbackReport builds a minimal markdown report when Phase 4 AI output is empty.
func GenerateFallbackReport(state *model.AuditState) string {
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
		if vf.Status == model.VerifySafe {
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
