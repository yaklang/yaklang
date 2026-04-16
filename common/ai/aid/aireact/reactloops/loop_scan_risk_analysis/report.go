package loop_scan_risk_analysis

import (
	"fmt"
	"strings"
)

func buildRiskRowSummaries(groups []MergedRiskGroup, decisions []FPDecision) []RiskRowSummary {
	decByGroup := make(map[string]FPDecision, len(decisions))
	for _, d := range decisions {
		decByGroup[d.GroupID] = d
	}
	out := make([]RiskRowSummary, 0, 64)
	for _, g := range groups {
		d, ok := decByGroup[g.GroupID]
		if !ok {
			continue
		}
		reasonJoined := strings.Join(d.Reasons, "；")
		for _, ur := range g.Risks {
			out = append(out, RiskRowSummary{
				RiskID:          ur.ID,
				GroupID:         g.GroupID,
				MergeKey:        g.Key,
				Title:           ur.Title,
				Severity:        ur.Severity,
				FromRule:        ur.FromRule,
				RiskType:        ur.RiskType,
				CodeSourceURL:   ur.CodeSourceURL,
				Line:            ur.Line,
				FunctionName:    ur.FunctionName,
				FPStatus:        d.Status,
				FPConfidence:    d.Confidence,
				FPReasonsJoined: reasonJoined,
			})
		}
	}
	return out
}

func decisionHasContentFPSignal(d FPDecision) bool {
	for _, ev := range d.Evidence {
		v := strings.ToLower(strings.TrimSpace(ev))
		if strings.Contains(v, "content_fp_signal") || strings.Contains(v, "content_signal_conflict") {
			return true
		}
		if strings.Contains(v, "ai_fp_evidence=") || strings.Contains(v, "ai_fp_triage_applied") {
			return true
		}
		if strings.Contains(v, "llm_boundary_status=suspicious") {
			return true
		}
	}
	return false
}

// falsePositiveReportInner 输出 not_issue + 内容证据驱动的 suspicious，避免“误报分诊为空”。
func falsePositiveReportInner(r *FinalAnalysisReport) string {
	if r == nil {
		return "（内部错误：FinalAnalysisReport 为空）\n"
	}
	var b strings.Builder
	b.WriteString("以下为 **`not_issue`（误报倾向）** 结论与对应告警行。\n\n")

	var fpOnly []FPDecision
	for _, d := range r.FPDecisions {
		if d.Status == FPNotIssue {
			fpOnly = append(fpOnly, d)
		}
	}
	if len(fpOnly) == 0 {
		b.WriteString("本次分诊 **无** `not_issue` 分组。\n\n")
	} else {
		b.WriteString("### 误报倾向 · 合并组（摘要）\n\n")
		for _, d := range fpOnly {
			b.WriteString(fmt.Sprintf("#### %s\n\n", d.GroupID))
			b.WriteString(fmt.Sprintf("- **结论**: `%s`（置信度 %d）\n", d.Status, d.Confidence))
			if len(d.Reasons) > 0 {
				b.WriteString("- **说明**:\n")
				for _, reason := range d.Reasons {
					b.WriteString(fmt.Sprintf("  - %s\n", reason))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("### 误报倾向 · 逐条 SSA 告警（not_issue）\n\n")
	var rows []RiskRowSummary
	for _, row := range r.RiskRows {
		if row.FPStatus == FPNotIssue {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		b.WriteString("无 `not_issue` 逐条告警行。\n\n")
	} else {
		b.WriteString("| risk_id | 分组 | 标题(截断) | 严重级别 | 规则 | 位置 | 置信度 |\n")
		b.WriteString("|---------|------|-------------|----------|------|------|--------|\n")
		for _, row := range rows {
			loc := row.CodeSourceURL
			if row.Line > 0 {
				loc = fmt.Sprintf("%s:%d", row.CodeSourceURL, row.Line)
			}
			title := strings.ReplaceAll(row.Title, "|", "/")
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | `%s` | `%s` | %d |\n",
				row.RiskID, row.GroupID, title, row.Severity, row.FromRule, loc, row.FPConfidence))
		}
		b.WriteString("\n")
	}

	var suspiciousContent []FPDecision
	for _, d := range r.FPDecisions {
		if d.Status == FPSuspicious && decisionHasContentFPSignal(d) {
			suspiciousContent = append(suspiciousContent, d)
		}
	}
	b.WriteString("### 疑似误报 · 合并组（suspicious，内容证据驱动）\n\n")
	if len(suspiciousContent) == 0 {
		b.WriteString("无命中内容误报信号的 `suspicious` 分组。\n\n")
	} else {
		for _, d := range suspiciousContent {
			b.WriteString(fmt.Sprintf("#### %s\n\n", d.GroupID))
			b.WriteString(fmt.Sprintf("- **结论**: `%s`（置信度 %d）\n", d.Status, d.Confidence))
			if len(d.Reasons) > 0 {
				b.WriteString("- **说明**:\n")
				for _, reason := range d.Reasons {
					b.WriteString(fmt.Sprintf("  - %s\n", reason))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("### 疑似误报 · 逐条 SSA 告警（suspicious）\n\n")
	suspiciousGroupSet := make(map[string]struct{}, len(suspiciousContent))
	for _, d := range suspiciousContent {
		suspiciousGroupSet[d.GroupID] = struct{}{}
	}
	var suspiciousRows []RiskRowSummary
	for _, row := range r.RiskRows {
		if row.FPStatus != FPSuspicious {
			continue
		}
		if _, ok := suspiciousGroupSet[row.GroupID]; ok {
			suspiciousRows = append(suspiciousRows, row)
		}
	}
	if len(suspiciousRows) == 0 {
		b.WriteString("无 `suspicious` 疑似误报逐条告警行。\n\n")
	} else {
		b.WriteString("| risk_id | 分组 | 标题(截断) | 严重级别 | 规则 | 位置 | 置信度 |\n")
		b.WriteString("|---------|------|-------------|----------|------|------|--------|\n")
		confByGroup := make(map[string]int, len(suspiciousContent))
		for _, d := range suspiciousContent {
			confByGroup[d.GroupID] = d.Confidence
		}
		for _, row := range suspiciousRows {
			loc := row.CodeSourceURL
			if row.Line > 0 {
				loc = fmt.Sprintf("%s:%d", row.CodeSourceURL, row.Line)
			}
			title := strings.ReplaceAll(row.Title, "|", "/")
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | `%s` | `%s` | %d |\n",
				row.RiskID, row.GroupID, title, row.Severity, row.FromRule, loc, confByGroup[row.GroupID]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

const maxPocWorthyMarkdownRows = 64

// pickRepresentativeRiskForPoc prefers a row with non-placeholder FromRule when multiple risks exist in the group.
func pickRepresentativeRiskForPoc(g MergedRiskGroup) UnifiedRisk {
	var fallback UnifiedRisk
	for _, ur := range g.Risks {
		if ur.ID <= 0 || strings.TrimSpace(ur.CodeSourceURL) == "" {
			continue
		}
		if !trivialFromRule(ur.FromRule) {
			return ur
		}
		if fallback.ID == 0 {
			fallback = ur
		}
	}
	return fallback
}

// pocWorthyReportInner 列出「非 not_issue」分组中 PoC 价值（启发式：规则名/标题/risk_id/路径；**未**调用 LLM 读 Details/CodeFragment）。
func pocWorthyReportInner(r *FinalAnalysisReport) string {
	if r == nil {
		return "（内部错误：FinalAnalysisReport 为空）\n"
	}
	decBy := make(map[string]FPDecision, len(r.FPDecisions))
	for _, d := range r.FPDecisions {
		decBy[d.GroupID] = d
	}
	var b strings.Builder
	b.WriteString("以下为 **`is_issue` / `suspicious`** 分组中的 **PoC 价值**（本模式不自动跑 forge）。\n\n")
	b.WriteString("说明：下表依据 **分诊结论、FromRule、Title、risk_id、代码路径** 做工程启发式分级，**不会**调用大模型逐条阅读 `details` / 代码片段；占位规则名（如 `test`）会标为 **低** 或推动分诊偏向可疑/误报侧。\n\n")

	type row struct {
		groupID, fp string
		conf        int
		riskID      int64
		tier, note  string
		rule, loc   string
	}
	var rows []row
	for _, g := range r.Groups {
		d, ok := decBy[g.GroupID]
		if !ok || d.Status == FPNotIssue {
			continue
		}
		rep := pickRepresentativeRiskForPoc(g)
		fp := string(d.Status)
		if rep.ID > 0 {
			loc := rep.CodeSourceURL
			if rep.Line > 0 {
				loc = fmt.Sprintf("%s:%d", rep.CodeSourceURL, rep.Line)
			}
			tier, note := pocSignalForRepresentative(rep)
			rows = append(rows, row{
				groupID: g.GroupID, fp: fp, conf: d.Confidence, riskID: rep.ID,
				tier: tier, note: note,
				rule: rep.FromRule, loc: loc,
			})
		} else {
			rows = append(rows, row{
				groupID: g.GroupID, fp: fp, conf: d.Confidence, riskID: 0,
				tier: "低", note: "本组无同时满足 risk_id>0 且含代码路径的代表告警",
				rule: "", loc: "",
			})
		}
		if len(rows) >= maxPocWorthyMarkdownRows {
			break
		}
	}
	if len(rows) == 0 {
		b.WriteString("无非误报（`is_issue`/`suspicious`）分组。\n")
		return b.String()
	}
	b.WriteString("| 分组 | 分诊 | 置信度 | 代表 risk_id | PoC 价值 | 规则 | 位置 | 说明 |\n")
	b.WriteString("|------|------|--------|--------------|----------|------|------|------|\n")
	for _, x := range rows {
		rid := "-"
		if x.riskID > 0 {
			rid = fmt.Sprintf("%d", x.riskID)
		}
		rule := x.rule
		if rule == "" {
			rule = "-"
		}
		loc := x.loc
		if loc == "" {
			loc = "-"
		}
		n := strings.ReplaceAll(x.note, "|", "/")
		if len(n) > 80 {
			n = n[:77] + "..."
		}
		b.WriteString(fmt.Sprintf("| %s | `%s` | %d | %s | **%s** | `%s` | `%s` | %s |\n",
			x.groupID, x.fp, x.conf, rid, x.tier, rule, loc, n))
	}
	b.WriteString("\n")
	if len(rows) >= maxPocWorthyMarkdownRows {
		b.WriteString(fmt.Sprintf("（表内最多展示前 **%d** 组；其余组见 `analysis_summary.json`。）\n", maxPocWorthyMarkdownRows))
	}
	return b.String()
}

// pocGenerationReportInner 与 pocWorthyReportInner 相同语义（独立 PoC 价值件用）。
func pocGenerationReportInner(r *FinalAnalysisReport) string {
	return pocWorthyReportInner(r)
}

// buildFalsePositiveStandaloneMarkdown 独立误报分诊 Markdown（磁盘 false_positive_report.md）。
func buildFalsePositiveStandaloneMarkdown(r *FinalAnalysisReport) string {
	if r == nil {
		return "# 误报分诊\n\n（错误：报告对象为空）\n"
	}
	var b strings.Builder
	b.WriteString("# 误报分诊\n\n")
	b.WriteString(fmt.Sprintf("`scan_id`: `%s`\n\n", r.ScanID))
	b.WriteString(falsePositiveReportInner(r))
	return b.String()
}

// buildPocGenerationStandaloneMarkdown 独立 PoC 价值评估 Markdown（磁盘 poc_generation_report.md）。
func buildPocGenerationStandaloneMarkdown(r *FinalAnalysisReport) string {
	if r == nil {
		return "# PoC 价值评估\n\n（错误：报告对象为空）\n"
	}
	var b strings.Builder
	b.WriteString("# PoC 价值评估\n\n")
	b.WriteString(fmt.Sprintf("`scan_id`: `%s`\n\n", r.ScanID))
	b.WriteString(pocWorthyReportInner(r))
	return b.String()
}

func buildMarkdownReport(r *FinalAnalysisReport) string {
	var b strings.Builder
	b.WriteString("# 扫描风险分析\n\n")
	b.WriteString("仅包含 **误报分诊** 与 **PoC 生成价值** 两类信息（不展示合并统计、任务元数据等）。\n\n")
	b.WriteString(fmt.Sprintf("`scan_id`: `%s`\n\n", r.ScanID))
	b.WriteString("## 1. 误报分诊（not_issue + suspicious）\n\n")
	b.WriteString(falsePositiveReportInner(r))
	b.WriteString("## 2. PoC 生成价值（is_issue / suspicious）\n\n")
	b.WriteString(pocWorthyReportInner(r))
	b.WriteString("\n机器可读全量见 `analysis_summary.json`。\n")
	return b.String()
}
