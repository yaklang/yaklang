package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"path/filepath"
	"strings"
)

// auditSummaryClaimsVulnerability detects whether mark_file_done audit_summary
// describes a confirmed vulnerability (vs. safe / no issue).
func auditSummaryClaimsVulnerability(summary string) bool {
	s := strings.ToLower(strings.TrimSpace(summary))
	if s == "" {
		return false
	}
	neg := []string{
		"无漏洞", "未发现", "不存在漏洞", "不存在命令注入", "不存在 sql", "无明显漏洞",
		"安全", "已防护", "已采用", "参数化", "白名单",
		"no vulnerability", "not vulnerable", "no issue", "safe",
	}
	for _, n := range neg {
		if strings.Contains(s, n) {
			return false
		}
	}
	pos := []string{
		"漏洞", "注入", "xss", "rce", "绕过", "vulnerable", "injection",
		"exploit", "命令执行", "代码执行", "ssrf", "xxe", "idor", "越权",
		"疑似", "风险", "可疑", "suspicious", "risk",
	}
	for _, p := range pos {
		if strings.Contains(s, p) {
			return true
		}
	}
	return strings.Contains(summary, "发现") && !strings.Contains(summary, "未发现")
}

func hasFindingForAbsPath(state *model.AuditState, categoryID, absPath, projectRoot string) bool {
	if state == nil || absPath == "" {
		return false
	}
	rel := absPath
	if projectRoot != "" {
		if r, err := filepath.Rel(projectRoot, absPath); err == nil && r != "" && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		}
	}
	rel = strings.TrimPrefix(rel, "./")
	absNorm := filepath.ToSlash(absPath)
	for _, f := range state.GetFindings() {
		if f == nil || f.Category != categoryID {
			continue
		}
		fFile := filepath.ToSlash(strings.TrimPrefix(f.File, "./"))
		if fFile == rel || strings.HasSuffix(absNorm, "/"+fFile) || fFile == absNorm {
			return true
		}
	}
	return false
}

func formatMarkFileDoneMissingFindingFeedback(filePath string) string {
	return fmt.Sprintf(
		"[错误] audit_summary 描述漏洞/风险，但本文件尚未调用 add_finding。\n"+
			"Phase2 广度优先：请先 add_finding(..., confidence=4–10)，再 mark_file_done。\n"+
			"请立即对 %q 调用 add_finding（data_flow 可基于当前阅读写「待 Phase3 复核」），然后再 mark_file_done。\n"+
			"若确认无问题，请将 audit_summary 改为明确「无漏洞/已防护」。",
		filePath,
	)
}
