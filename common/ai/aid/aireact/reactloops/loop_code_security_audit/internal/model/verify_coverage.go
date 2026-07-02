package model

import (
	"fmt"
	"strings"
)

const autoVerifyGapReason = "Phase3 验证阶段未对该 finding 调用 conclude_finding（循环提前结束或跳过），系统自动标记为 uncertain，需人工复核。"

// UpsertVerifiedFinding stores or replaces the verification result for a finding ID.
func (s *AuditState) UpsertVerifiedFinding(vf *VerifiedFinding) {
	if s == nil || vf == nil || vf.Finding == nil || vf.Finding.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, v := range s.VerifiedVulns {
		if v != nil && v.Finding != nil && v.Finding.ID == vf.Finding.ID {
			s.VerifiedVulns[i] = vf
			return
		}
	}
	s.VerifiedVulns = append(s.VerifiedVulns, vf)
}

// GetVerifiedFindingByID returns the latest verified record for a finding ID.
func (s *AuditState) GetVerifiedFindingByID(id string) *VerifiedFinding {
	if s == nil || id == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.VerifiedVulns {
		if v != nil && v.Finding != nil && v.Finding.ID == id {
			return v
		}
	}
	return nil
}

// DedupeVerifiedVulns keeps the last record per finding ID (in-order scan).
func (s *AuditState) DedupeVerifiedVulns() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]int)
	var out []*VerifiedFinding
	removed := 0
	for _, v := range s.VerifiedVulns {
		if v == nil || v.Finding == nil || v.Finding.ID == "" {
			removed++
			continue
		}
		if idx, ok := seen[v.Finding.ID]; ok {
			out[idx] = v
			removed++
			continue
		}
		seen[v.Finding.ID] = len(out)
		out = append(out, v)
	}
	s.VerifiedVulns = out
	return removed
}

// MissingVerifiedFindingIDs returns finding IDs without any verified record.
func (s *AuditState) MissingVerifiedFindingIDs() []string {
	if s == nil {
		return nil
	}
	findings := s.GetFindings()
	var missing []string
	for _, f := range findings {
		if f == nil || f.ID == "" {
			continue
		}
		if s.GetVerifiedFindingByID(f.ID) == nil {
			missing = append(missing, f.ID)
		}
	}
	return missing
}

// EnsureVerifyCoverage fills gaps with uncertain verified records. Returns auto-filled IDs.
func (s *AuditState) EnsureVerifyCoverage() []string {
	if s == nil {
		return nil
	}
	s.DedupeVerifiedVulns()
	var filled []string
	for _, f := range s.GetFindings() {
		if f == nil || f.ID == "" {
			continue
		}
		if s.GetVerifiedFindingByID(f.ID) != nil {
			continue
		}
		s.UpsertVerifiedFinding(&VerifiedFinding{
			Finding:    f,
			Status:     VerifyUncertain,
			Confidence: clampConfidence(f.Confidence, 5),
			Reason:     autoVerifyGapReason,
			DataFlow:   f.DataFlow,
			Exploit:    f.ExploitScenario,
			Fix:        f.Recommendation,
		})
		filled = append(filled, f.ID)
	}
	return filled
}

func clampConfidence(v, fallback int) int {
	if v < 1 {
		return fallback
	}
	if v > 10 {
		return 10
	}
	return v
}

// ReportableVerifiedVulns returns verified records that should appear in the audit report.
func (s *AuditState) ReportableVerifiedVulns() []*VerifiedFinding {
	var out []*VerifiedFinding
	for _, v := range s.GetVerifiedVulns() {
		if v == nil || v.Finding == nil {
			continue
		}
		if v.Status == VerifyConfirmed || v.Status == VerifyUncertain {
			out = append(out, v)
		}
	}
	return out
}

// FindingsMissingFromReport returns reportable findings not referenced in report markdown.
func FindingsMissingFromReport(report string, vulns []*VerifiedFinding) []*VerifiedFinding {
	reportLower := strings.ToLower(report)
	var missing []*VerifiedFinding
	for _, v := range vulns {
		if v == nil || v.Finding == nil {
			continue
		}
		if findingMentionedInReport(reportLower, v.Finding) {
			continue
		}
		missing = append(missing, v)
	}
	return missing
}

func findingMentionedInReport(reportLower string, f *Finding) bool {
	if f == nil {
		return false
	}
	if f.ID != "" && strings.Contains(reportLower, strings.ToLower(f.ID)) {
		return true
	}
	title := strings.TrimSpace(f.Title)
	if title != "" && strings.Contains(reportLower, strings.ToLower(title)) {
		return true
	}
	file := strings.TrimSpace(f.File)
	if file != "" && strings.Contains(reportLower, strings.ToLower(file)) {
		// file path alone is weak; require line hint when possible
		if f.Line > 0 {
			lineHint := fmt.Sprintf("%s:%d", file, f.Line)
			if strings.Contains(reportLower, strings.ToLower(lineHint)) {
				return true
			}
		}
	}
	return false
}

// RepairAuditReportCoverage appends structured sections for findings missing from the report.
func RepairAuditReportCoverage(report string, vulns []*VerifiedFinding) (string, []string) {
	missing := FindingsMissingFromReport(report, vulns)
	if len(missing) == 0 {
		return report, nil
	}
	var ids []string
	var b strings.Builder
	b.WriteString(strings.TrimRight(report, "\n"))
	b.WriteString("\n\n---\n\n## 附录：报告补录（系统自动补全）\n\n")
	b.WriteString("> 以下漏洞在 verified_vulns 中有验证结论，但 AI 生成的报告正文未覆盖，已由系统在 Go 层补录。\n\n")
	for _, v := range missing {
		ids = append(ids, v.Finding.ID)
		b.WriteString(renderVerifiedFindingReportSection(v))
		b.WriteByte('\n')
	}
	return b.String(), ids
}

func renderVerifiedFindingReportSection(v *VerifiedFinding) string {
	if v == nil || v.Finding == nil {
		return ""
	}
	f := v.Finding
	var b strings.Builder
	b.WriteString(fmt.Sprintf("### [%s] %s（%s）\n\n", f.ID, f.Title, strings.ToUpper(string(v.Status))))
	b.WriteString(fmt.Sprintf("- **严重程度**: %s\n", f.Severity))
	b.WriteString(fmt.Sprintf("- **位置**: `%s:%d`\n", f.File, f.Line))
	b.WriteString(fmt.Sprintf("- **验证结论**: %s（置信度 %d/10）\n", v.Status, v.Confidence))
	if v.Reason != "" {
		b.WriteString(fmt.Sprintf("- **验证理由**: %s\n", v.Reason))
	}
	if v.DataFlow != "" {
		b.WriteString(fmt.Sprintf("- **数据流**: %s\n", v.DataFlow))
	}
	if v.Exploit != "" {
		b.WriteString(fmt.Sprintf("- **利用方式**: %s\n", v.Exploit))
	} else if f.ExploitScenario != "" {
		b.WriteString(fmt.Sprintf("- **利用方式**: %s\n", f.ExploitScenario))
	}
	if v.Fix != "" {
		b.WriteString(fmt.Sprintf("- **修复建议**: %s\n", v.Fix))
	} else if f.Recommendation != "" {
		b.WriteString(fmt.Sprintf("- **修复建议**: %s\n", f.Recommendation))
	}
	return b.String()
}

func BuildMandatoryFindingIDChecklist(state *AuditState) string {
	vulns := state.ReportableVerifiedVulns()
	if len(vulns) == 0 {
		return "（无需要写入报告的 verified finding）\n"
	}
	var b strings.Builder
	for i, v := range vulns {
		if v == nil || v.Finding == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. **%s** [%s] %s — 必须在报告中出现（建议作为独立小节）\n",
			i+1, v.Finding.ID, v.Status, v.Finding.Title))
	}
	return b.String()
}
