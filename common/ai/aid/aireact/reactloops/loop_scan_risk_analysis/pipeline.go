package loop_scan_risk_analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

func parseScanID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)scan[_-]?id\s*[:=]\s*([A-Za-z0-9._\-:]+)`),
		regexp.MustCompile(`(?i)task[_-]?id\s*[:=]\s*([A-Za-z0-9._\-:]+)`),
	}
	for _, p := range patterns {
		m := p.FindStringSubmatch(raw)
		if len(m) >= 2 {
			return strings.TrimSpace(m[1])
		}
	}
	fields := strings.Fields(raw)
	if len(fields) == 1 && len(fields[0]) >= 8 {
		tok := fields[0]
		if regexp.MustCompile(`^[A-Za-z0-9._\-:]+$`).MatchString(tok) {
			return tok
		}
	}
	return ""
}

func (s *AnalysisState) loadAndMerge() error {
	db := consts.GetGormSSAProjectDataBase()
	task, err := schema.GetSyntaxFlowScanTaskById(db, s.ScanID)
	if err != nil {
		return fmt.Errorf("scan_id %s not found: %w", s.ScanID, err)
	}
	s.Task = task

	var raw []*schema.SSARisk
	if err := db.Model(&schema.SSARisk{}).Where("runtime_id = ?", s.ScanID).Order("id asc").Find(&raw).Error; err != nil {
		return err
	}
	s.RawRisks = make([]UnifiedRisk, 0, len(raw))
	for _, r := range raw {
		s.RawRisks = append(s.RawRisks, newUnifiedRisk(r))
	}

	groupMap := make(map[string]*MergedRiskGroup)
	keys := make([]string, 0)
	for _, r := range s.RawRisks {
		key := mergeKey(r)
		g, ok := groupMap[key]
		if !ok {
			g = &MergedRiskGroup{
				GroupID:         fmt.Sprintf("G-%04d", len(groupMap)+1),
				Key:             key,
				RiskFeatureHash: r.RiskFeatureHash,
				SeverityMax:     r.Severity,
			}
			groupMap[key] = g
			keys = append(keys, key)
		}
		g.RiskIDs = append(g.RiskIDs, r.ID)
		g.Risks = append(g.Risks, r)
		g.Paths = append(g.Paths, r.CodeSourceURL)
		g.Functions = append(g.Functions, r.FunctionName)
		g.Rules = append(g.Rules, r.FromRule)
		g.SeverityMax = maxSeverity(g.SeverityMax, r.Severity)
		loc := r.CodeSourceURL
		if r.Line > 0 {
			loc = fmt.Sprintf("%s:%d", r.CodeSourceURL, r.Line)
		}
		g.Locations = append(g.Locations, loc)
	}

	sort.Strings(keys)
	s.Groups = make([]MergedRiskGroup, 0, len(keys))
	for _, key := range keys {
		g := groupMap[key]
		g.Paths = uniqueSorted(g.Paths)
		g.Functions = uniqueSorted(g.Functions)
		g.Rules = uniqueSorted(g.Rules)
		g.Locations = uniqueSorted(g.Locations)
		g.Count = len(g.RiskIDs)
		g.MergeStats = GroupMergeStats{
			RawRiskCountInGroup: len(g.Risks),
			DistinctPaths:       len(g.Paths),
			DistinctLocations:   len(g.Locations),
			DistinctFunctions:   len(g.Functions),
			DistinctRules:       len(g.Rules),
		}
		s.Groups = append(s.Groups, *g)
	}
	s.Phase = PhaseFP
	return nil
}

// generateSourceSSAReport 先生成与 gRPC GenerateSSAReport 同源的数据报告，并导出 Markdown 供 AI 误报分析使用。
func (s *AnalysisState) generateSourceSSAReport(ctx context.Context) error {
	if s.Task == nil {
		return fmt.Errorf("scan task is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ssaReport, err := sfreport.GenerateSSAProjectReportFromTask(ctx, s.Task)
	if err != nil {
		return err
	}
	reportIns := &schema.Report{}
	reportIns.From("ssa-scan")
	reportIns.Title(fmt.Sprintf("SSA项目扫描报告_%s", s.ScanID))
	if err := sfreport.GenerateYakitReportContent(reportIns, ssaReport); err != nil {
		return err
	}
	s.SourceSSAReportID = reportIns.SaveForIRify()
	s.SourceSSAReportMarkdown = extractMarkdownFromReportItems(reportIns)
	if strings.TrimSpace(s.SourceSSAReportMarkdown) == "" {
		s.SourceSSAReportMarkdown = fallbackSSAReportMarkdownFromMergedGroups(s.ScanID, s.Groups)
	}

	baseDir := filepath.Join(s.WorkDir, "scan_risk_analysis", s.ScanID)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	s.SourceSSAReportPath = filepath.Join(baseDir, "source_ssa_report.md")
	body := strings.TrimSpace(s.SourceSSAReportMarkdown)
	if body == "" {
		body = "# SSA scan report\n\n（报告生成器未产出 Markdown 片段；请检查扫描任务与报告模板。）\n"
		s.SourceSSAReportMarkdown = body
	}
	if err := os.WriteFile(s.SourceSSAReportPath, []byte(body), 0o644); err != nil {
		return err
	}
	return nil
}

// fallbackSSAReportMarkdownFromMergedGroups builds a minimal Markdown digest when Yakit report items lack markdown blocks.
func fallbackSSAReportMarkdownFromMergedGroups(scanID string, groups []MergedRiskGroup) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# SSA scan digest (fallback)\n\nscan_id: `%s`\n\n", scanID))
	for _, g := range groups {
		b.WriteString(fmt.Sprintf("## %s\n\n", g.GroupID))
		b.WriteString(fmt.Sprintf("- merge_key: `%s`\n", g.Key))
		b.WriteString(fmt.Sprintf("- severity_max: %s\n", g.SeverityMax))
		b.WriteString(fmt.Sprintf("- raw_risks: %d\n\n", len(g.Risks)))
		sampled := sampleGroupRisksForFP(g, 3)
		for _, ur := range sampled {
			loc := ur.CodeSourceURL
			if ur.Line > 0 {
				loc = fmt.Sprintf("%s:%d", ur.CodeSourceURL, ur.Line)
			}
			b.WriteString(fmt.Sprintf("### risk_id %d @ %s\n\n", ur.ID, loc))
			if strings.TrimSpace(ur.FromRule) != "" {
				b.WriteString(fmt.Sprintf("- rule: `%s`\n", ur.FromRule))
			}
			if strings.TrimSpace(ur.Title) != "" {
				b.WriteString(fmt.Sprintf("- title: %s\n", ur.Title))
			}
			if strings.TrimSpace(ur.RiskType) != "" {
				b.WriteString(fmt.Sprintf("- type: %s\n", ur.RiskType))
			}
			if strings.TrimSpace(ur.Details) != "" {
				b.WriteString(fmt.Sprintf("- details: %s\n", strings.TrimSpace(ur.Details)))
			}
			if strings.TrimSpace(ur.CodeFragment) != "" {
				b.WriteString("- code_fragment:\n\n```\n")
				b.WriteString(strings.TrimSpace(ur.CodeFragment))
				b.WriteString("\n```\n")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *AnalysisState) analyzeFalsePositive(ctx context.Context, inv aicommon.AIInvokeRuntime) error {
	db := consts.GetGormSSAProjectDataBase()
	if s.Task == nil {
		return fmt.Errorf("scan task is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]FPDecision, 0, len(s.Groups))
	for _, g := range s.Groups {
		decision := FPDecision{
			GroupID: g.GroupID,
			Status:  FPSuspicious,
		}
		decision.Reasons = append(decision.Reasons,
			fmt.Sprintf("本合并组含 %d 条原始 SSA 告警（合并键 `%s`），对整组统一误报评估；逐条告警结论见报告「逐条 SSA 告警」表。", len(g.Risks), g.Key))

		history := make([]*schema.SSARiskDisposals, 0)
		if strings.TrimSpace(g.RiskFeatureHash) != "" {
			err := db.Model(&schema.SSARiskDisposals{}).
				Joins("JOIN "+schema.TableSyntaxFlowScanTask+" ON "+schema.TableSSARiskDisposals+".task_id="+schema.TableSyntaxFlowScanTask+".task_id").
				Where(schema.TableSSARiskDisposals+".risk_feature_hash = ? AND "+schema.TableSyntaxFlowScanTask+".scan_batch <= ?", g.RiskFeatureHash, s.Task.ScanBatch).
				Order(schema.TableSyntaxFlowScanTask + ".scan_batch desc, " + schema.TableSSARiskDisposals + ".updated_at desc").
				Find(&history).Error
			if err != nil {
				return err
			}
		}
		decision.HistoryDisposals = history

		issueScore := severityRank(g.SeverityMax)
		falseScore := 0
		if len(g.Paths) > 0 {
			issueScore += 1
			decision.Evidence = append(decision.Evidence, "has_code_path")
		} else {
			falseScore += 1
			decision.Reasons = append(decision.Reasons, "缺少代码路径信息")
		}
		if len(g.Locations) > 0 {
			issueScore += 1
			decision.Evidence = append(decision.Evidence, "has_code_location")
		}
		if len(g.Rules) > 0 {
			issueScore += 1
			decision.Evidence = append(decision.Evidence, "has_rule_binding")
		}

		sampled := sampleGroupRisksForFP(g, 4)
		contentSignals := scoreSampledRiskContent(sampled)
		issueScore += contentSignals.IssueDelta
		falseScore += contentSignals.FalseDelta
		decision.Evidence = append(decision.Evidence, contentSignals.Evidence...)
		decision.Reasons = append(decision.Reasons, contentSignals.Reasons...)

		notIssueCount := 0
		isIssueCount := 0
		for _, h := range history {
			switch schema.ValidSSARiskDisposalStatus(h.Status) {
			case schema.SSARiskDisposalStatus_NotIssue:
				notIssueCount++
			case schema.SSARiskDisposalStatus_IsIssue:
				isIssueCount++
			}
		}
		if notIssueCount > 0 {
			falseScore += 2 + notIssueCount
			decision.Evidence = append(decision.Evidence, fmt.Sprintf("history_not_issue=%d", notIssueCount))
		}
		if isIssueCount > 0 {
			issueScore += 2 + isIssueCount
			decision.Evidence = append(decision.Evidence, fmt.Sprintf("history_is_issue=%d", isIssueCount))
		}

		if groupAllRisksHaveTrivialRules(g) {
			decision.Evidence = append(decision.Evidence, "all_rules_placeholder_like")
			decision.Reasons = append(decision.Reasons, "组内规则名偏测试/占位，但仅作为弱特征；最终结论以风险内容证据为主")
			if contentSignals.FalseDelta >= contentSignals.IssueDelta {
				falseScore += 1
				decision.Evidence = append(decision.Evidence, "rule_name_placeholder_hint_weak")
			}
		}

		decision.IssueScore = issueScore
		decision.FalsePositiveScore = falseScore

		switch {
		case issueScore-falseScore >= 3:
			decision.Status = FPIsIssue
			decision.Confidence = min(10, 6+(issueScore-falseScore))
			decision.Reasons = append(decision.Reasons, "风险证据与历史状态更偏向真实问题")
		case falseScore-issueScore >= 2:
			decision.Status = FPNotIssue
			decision.Confidence = min(10, 5+(falseScore-issueScore))
			decision.Reasons = append(decision.Reasons, "历史处置与证据完整性更偏向误报")
		default:
			decision.Status = FPSuspicious
			decision.Confidence = 6
			decision.Reasons = append(decision.Reasons, "证据不足以确认，需要人工复核")
		}
		results = append(results, decision)
	}
	s.Decisions = results
	aiReport, err := generateAIFPReportMarkdownFromSourceReport(ctx, inv, s.ScanID, s.Groups, s.SourceSSAReportMarkdown)
	if err != nil {
		for i := range results {
			results[i].Evidence = append(results[i].Evidence, "ai_fp_report_failed")
			results[i].Reasons = append(results[i].Reasons, "AI 原始误报报告生成失败，保留结构化评分结论: "+err.Error())
		}
	} else {
		s.AIFPReportMarkdown = aiReport
	}
	// PoC 不在本流水线中自动生成；下一阶段直接进入报告。
	s.Phase = PhaseReport
	return nil
}

const maxSourceReportPromptBytes = 120000

func extractMarkdownFromReportItems(r *schema.Report) string {
	if r == nil || len(r.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, item := range r.Items {
		if item.Type != schema.REPORT_ITEM_TYPE_MARKDOWN || strings.TrimSpace(item.Content) == "" {
			continue
		}
		b.WriteString(strings.TrimSpace(item.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func generateAIFPReportMarkdownFromSourceReport(ctx context.Context, inv aicommon.AIInvokeRuntime, scanID string, groups []MergedRiskGroup, reportMarkdown string) (string, error) {
	if inv == nil {
		return "", fmt.Errorf("nil ai invoker")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reportBody := strings.TrimSpace(reportMarkdown)
	if reportBody == "" {
		return "", fmt.Errorf("empty source ssa report markdown")
	}
	if len(reportBody) > maxSourceReportPromptBytes {
		reportBody = reportBody[:maxSourceReportPromptBytes] + "\n\n...(truncated for AI prompt)"
	}

	var idx strings.Builder
	idx.WriteString("## 合并组索引（仅可引用以下 group_id，不得自创）\n\n")
	for _, g := range groups {
		locations := "n/a"
		if len(g.Locations) > 0 {
			picked := g.Locations
			if len(picked) > 8 {
				picked = picked[:8]
			}
			locations = strings.Join(picked, "; ")
		}
		idx.WriteString(fmt.Sprintf("- %s | merge_key=%s | raw_risks=%d | severity_max=%s | locations=%s\n",
			g.GroupID, g.Key, len(g.Risks), g.SeverityMax, locations))
	}

	prompt := fmt.Sprintf(`你是安全扫描误报分诊专家。请直接生成最终误报报告 Markdown，作为 false_positive_report.md 的正文。
要求：
1) 只能依据下方 SSA 报告内容判断，不要凭规则名直接下结论。
2) 必须覆盖每个 group_id，且每组明确标注 status（not_issue/suspicious/is_issue）和理由。
3) 报告必须包含以下标题结构（原样输出）：
# 误报分诊
`+"`scan_id`: `%s`"+`

## 合并组分诊结论
| group_id | status | confidence | reason | evidence_location |
| --- | --- | --- | --- | --- |

## 误报倾向（not_issue）
## 疑似误报（suspicious）

4) 输出必须是 Markdown 正文，不要 JSON，不要代码围栏，不要额外解释。

%s

--- SSA 报告 Markdown ---
%s`, scanID, idx.String(), reportBody)

	action, err := inv.InvokeQualityPriorityLiteForge(ctx, "scan_risk_fp_report_from_ssa_report", prompt, []aitool.ToolOption{
		aitool.WithStringParam("false_positive_report_markdown",
			aitool.WithParam_Description("final markdown body for false_positive_report.md"),
			aitool.WithParam_Required(true)),
	})
	if err != nil {
		return "", err
	}
	md := strings.TrimSpace(action.GetString("false_positive_report_markdown"))
	if md == "" {
		return "", fmt.Errorf("empty false_positive_report_markdown from model")
	}
	return md, nil
}

func applyAIFPTriageFromSourceReport(ctx context.Context, inv aicommon.AIInvokeRuntime, scanID string, groups []MergedRiskGroup, decisions *[]FPDecision, reportMarkdown string) error {
	if inv == nil || decisions == nil || len(*decisions) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reportBody := strings.TrimSpace(reportMarkdown)
	if reportBody == "" {
		return fmt.Errorf("empty source ssa report markdown")
	}
	if len(reportBody) > maxSourceReportPromptBytes {
		reportBody = reportBody[:maxSourceReportPromptBytes] + "\n\n...(truncated for AI prompt)"
	}

	var idx strings.Builder
	idx.WriteString("## 合并组索引（必须逐条输出 verdict，group_id 不得自创）\n\n")
	for _, g := range groups {
		locations := "n/a"
		if len(g.Locations) > 0 {
			picked := g.Locations
			if len(picked) > 5 {
				picked = picked[:5]
			}
			locations = strings.Join(picked, "; ")
		}
		idx.WriteString(fmt.Sprintf("- %s | merge_key=%s | raw_risks=%d | severity_max=%s | locations=%s\n",
			g.GroupID, g.Key, len(g.Risks), g.SeverityMax, locations))
	}

	prompt := fmt.Sprintf(`你是安全扫描误报分诊专家。下面是一份「SSA 项目扫描报告」的 Markdown（与 Yak gRPC GenerateSSAReport 同源），以及本任务的合并组列表。
请**只依据报告中的漏洞类型、标题、描述、代码片段与位置**判断每个合并组更可能是真实漏洞、误报还是需人工复核。
不要仅凭规则名（例如 test）下结论；规则名只能作为弱参考。

scan_id: %s

%s

--- SSA 报告 Markdown ---

%s

--- 输出要求 ---
仅输出一个 JSON 对象，不要 Markdown 围栏，不要额外文字。格式如下：
{"verdicts":[{"group_id":"G-0001","status":"is_issue|suspicious|not_issue","confidence":1-10,"reason":"中文简短理由","evidence":["引用报告中的关键短语或位置，至少一个必须是该组 locations 中的具体位置"]}]}
必须覆盖索引中的每一个 group_id。`, scanID, idx.String(), reportBody)

	action, err := inv.InvokeQualityPriorityLiteForge(ctx, "scan_risk_fp_triage_from_ssa_report", prompt, []aitool.ToolOption{
		aitool.WithStringParam("verdict_json",
			aitool.WithParam_Description("strict JSON object with verdicts array as specified in the prompt"),
			aitool.WithParam_Required(true)),
	})
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(action.GetString("verdict_json"))
	if raw == "" {
		return fmt.Errorf("empty verdict_json from model")
	}
	verdicts, err := parseFPVerdictJSONArray(raw)
	if err != nil {
		return err
	}
	decByID := make(map[string]int, len(*decisions))
	for i := range *decisions {
		decByID[(*decisions)[i].GroupID] = i
	}
	groupLocationSet := buildGroupLocationSet(groups)
	for _, v := range verdicts {
		i, ok := decByID[v.GroupID]
		if !ok {
			continue
		}
		validEvidence := make([]string, 0, len(v.Evidence))
		allowedLocs := groupLocationSet[v.GroupID]
		for _, ev := range v.Evidence {
			evTrim := strings.TrimSpace(ev)
			if evTrim == "" {
				continue
			}
			norm := normalizeCodeLocation(evTrim)
			if norm != "" {
				if _, ok := allowedLocs[norm]; !ok {
					continue
				}
			}
			validEvidence = append(validEvidence, evTrim)
		}
		if len(validEvidence) == 0 {
			(*decisions)[i].Evidence = append((*decisions)[i].Evidence, "ai_fp_verdict_unbound_to_group")
			(*decisions)[i].Reasons = append((*decisions)[i].Reasons, "AI verdict 未提供可绑定到本组位置的证据，保留结构化评分结论")
			continue
		}
		st := FPStatus(strings.ToLower(strings.TrimSpace(v.Status)))
		switch st {
		case FPIsIssue, FPSuspicious, FPNotIssue:
		default:
			continue
		}
		(*decisions)[i].Status = st
		if v.Confidence > 0 {
			(*decisions)[i].Confidence = min(10, v.Confidence)
		}
		(*decisions)[i].Evidence = append((*decisions)[i].Evidence, "ai_fp_triage_applied")
		for _, ev := range validEvidence {
			(*decisions)[i].Evidence = append((*decisions)[i].Evidence, "ai_fp_evidence="+ev)
		}
		reason := strings.TrimSpace(v.Reason)
		if reason != "" {
			(*decisions)[i].Reasons = append((*decisions)[i].Reasons, "AI 误报分诊（基于 SSA 报告）: "+reason)
		}
	}
	return nil
}

type aiFPVerdict struct {
	GroupID    string   `json:"group_id"`
	Status     string   `json:"status"`
	Confidence int      `json:"confidence"`
	Reason     string   `json:"reason"`
	Evidence   []string `json:"evidence"`
}

func parseFPVerdictJSONArray(raw string) ([]aiFPVerdict, error) {
	var envelope struct {
		Verdicts []aiFPVerdict `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Verdicts) == 0 {
		return nil, fmt.Errorf("verdicts array empty")
	}
	return envelope.Verdicts, nil
}

func buildGroupLocationSet(groups []MergedRiskGroup) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(groups))
	for _, g := range groups {
		set := make(map[string]struct{}, len(g.Locations))
		for _, loc := range g.Locations {
			norm := normalizeCodeLocation(loc)
			if norm == "" {
				continue
			}
			set[norm] = struct{}{}
		}
		out[g.GroupID] = set
	}
	return out
}

func normalizeCodeLocation(loc string) string {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return ""
	}
	loc = strings.ReplaceAll(loc, "\\", "/")
	return strings.ToLower(loc)
}

type fpContentSignals struct {
	IssueDelta int
	FalseDelta int
	Evidence   []string
	Reasons    []string
	Conflict   bool
}

func sampleGroupRisksForFP(g MergedRiskGroup, sampleLimit int) []UnifiedRisk {
	if len(g.Risks) == 0 {
		return nil
	}
	if sampleLimit <= 0 {
		sampleLimit = 4
	}
	repIndex := 0
	best := representativeRiskScore(g.Risks[0])
	for i := 1; i < len(g.Risks); i++ {
		score := representativeRiskScore(g.Risks[i])
		if score > best {
			repIndex = i
			best = score
		}
	}
	out := make([]UnifiedRisk, 0, sampleLimit)
	out = append(out, g.Risks[repIndex])
	seen := map[string]struct{}{
		fmt.Sprintf("%s:%d:%s", g.Risks[repIndex].CodeSourceURL, g.Risks[repIndex].Line, g.Risks[repIndex].FunctionName): {},
	}
	for idx, ur := range g.Risks {
		if idx == repIndex {
			continue
		}
		if len(out) >= sampleLimit {
			break
		}
		key := fmt.Sprintf("%s:%d:%s", ur.CodeSourceURL, ur.Line, ur.FunctionName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ur)
	}
	return out
}

func representativeRiskScore(ur UnifiedRisk) int {
	score := 0
	if strings.TrimSpace(ur.CodeSourceURL) != "" {
		score += 2
	}
	if ur.Line > 0 {
		score += 1
	}
	if strings.TrimSpace(ur.Details) != "" {
		score += 3
	}
	if strings.TrimSpace(ur.CodeFragment) != "" {
		score += 3
	}
	if strings.TrimSpace(ur.RiskType) != "" {
		score += 1
	}
	if strings.TrimSpace(ur.Title) != "" {
		score += 1
	}
	return score
}

func scoreSampledRiskContent(sampled []UnifiedRisk) fpContentSignals {
	signals := fpContentSignals{
		Evidence: make([]string, 0, 8),
		Reasons:  make([]string, 0, 8),
	}
	if len(sampled) == 0 {
		signals.FalseDelta += 2
		signals.Evidence = append(signals.Evidence, "content_missing_all_samples", "content_fp_signal_empty_sample")
		signals.Reasons = append(signals.Reasons, "缺少可分析的风险内容样本，误报侧权重上调")
		return signals
	}

	detailsCount := 0
	fragmentCount := 0
	issueKeywordHits := 0
	fpKeywordHits := 0
	weakContentCount := 0
	riskTypeAligned := 0
	for _, ur := range sampled {
		detail := strings.TrimSpace(ur.Details)
		fragment := strings.TrimSpace(ur.CodeFragment)
		title := strings.TrimSpace(ur.Title)
		mixed := strings.ToLower(strings.Join([]string{detail, fragment, title, strings.TrimSpace(ur.RiskType)}, "\n"))
		if detail != "" {
			detailsCount++
		}
		if fragment != "" {
			fragmentCount++
		}
		issueKeywordHits += countMatchedKeywords(mixed, issuePositiveKeywords)
		fpKeywordHits += countMatchedKeywords(mixed, falsePositiveKeywords)
		if looksWeakRiskContent(detail, fragment, title) {
			weakContentCount++
		}
		if alignsRiskTypeWithText(ur.RiskType, mixed) {
			riskTypeAligned++
		}
	}

	if detailsCount == 0 && fragmentCount == 0 {
		signals.FalseDelta += 3
		signals.Evidence = append(signals.Evidence, "content_missing_details_and_fragment", "content_fp_signal_missing_payload")
		signals.Reasons = append(signals.Reasons, "样本缺少 details/code_fragment，无法形成稳定触发链")
	} else if detailsCount > 0 && fragmentCount > 0 {
		signals.IssueDelta += 2
		signals.Evidence = append(signals.Evidence, "content_has_details_and_fragment")
	}

	if issueKeywordHits > 0 {
		signals.IssueDelta += min(3, issueKeywordHits)
		signals.Evidence = append(signals.Evidence, fmt.Sprintf("content_issue_semantic_hits=%d", issueKeywordHits))
	}
	if fpKeywordHits > 0 {
		signals.FalseDelta += min(3, fpKeywordHits)
		signals.Evidence = append(signals.Evidence, fmt.Sprintf("content_fp_signal_keywords=%d", fpKeywordHits))
		signals.Reasons = append(signals.Reasons, "风险内容出现 mock/demo/placeholder 等弱语义信号")
	}
	if riskTypeAligned > 0 {
		signals.IssueDelta += 1
		signals.Evidence = append(signals.Evidence, "content_risk_type_aligned")
	}
	if weakContentCount == len(sampled) {
		signals.FalseDelta += 2
		signals.Evidence = append(signals.Evidence, "content_fp_signal_all_weak")
		signals.Reasons = append(signals.Reasons, "样本标题与内容整体偏弱，倾向进入可疑复核")
	}
	if issueKeywordHits > 0 && fpKeywordHits > 0 {
		signals.FalseDelta += 1
		signals.Conflict = true
		signals.Evidence = append(signals.Evidence, "content_signal_conflict")
		signals.Reasons = append(signals.Reasons, "风险内容同时存在攻击与误报倾向信号，建议边界复核")
	}
	return signals
}

func countMatchedKeywords(text string, keywords []string) int {
	hits := 0
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(text, kw) {
			hits++
		}
	}
	return hits
}

func looksWeakRiskContent(details, fragment, title string) bool {
	joined := strings.ToLower(strings.TrimSpace(details + "\n" + fragment + "\n" + title))
	if joined == "" {
		return true
	}
	if countMatchedKeywords(joined, falsePositiveKeywords) > 0 && countMatchedKeywords(joined, issuePositiveKeywords) == 0 {
		return true
	}
	return false
}

func alignsRiskTypeWithText(riskType, text string) bool {
	rt := strings.ToLower(strings.TrimSpace(riskType))
	if rt == "" || text == "" {
		return false
	}
	if strings.Contains(text, rt) {
		return true
	}
	mapping := map[string][]string{
		"sql注入":  {"sql injection", "sqli", "select", "where", "union", "or 1=1"},
		"sql 注入": {"sql injection", "sqli", "select", "where", "union", "or 1=1"},
		"命令注入":   {"command injection", "cmd", "os/exec", "runtime.exec", "sh -c", "bash -c"},
		"xss":    {"xss", "<script", "javascript:"},
		"ssrf":   {"ssrf", "http client", "url fetch", "request.get"},
	}
	for k, v := range mapping {
		if strings.Contains(rt, k) {
			return countMatchedKeywords(text, v) > 0
		}
	}
	return false
}

var issuePositiveKeywords = []string{
	"sql injection", "sqli", "or 1=1", "union select", "runtime.exec", "os/exec", "command injection",
	"xss", "<script", "ssrf", "path traversal", "../", "xxe", "deserialize", "template injection",
	"sql注入", "命令注入", "路径遍历", "模板注入", "反序列化", "外部实体", "跨站脚本",
}

var falsePositiveKeywords = []string{
	"mock", "demo", "placeholder", "sample", "unit test", "test only", "fake",
	"poc template", "not reachable", "sanitized", "safe wrapper", "todo",
	"示例", "演示", "占位", "样例", "测试代码", "模拟数据", "不可达", "已过滤", "误报",
}

func (s *AnalysisState) generatePOCScripts(ctx context.Context, inv aicommon.AIInvokeRuntime) error {
	return runScanRiskPocPhase(ctx, inv, s)
}

func (s *AnalysisState) generateReports() error {
	baseDir := filepath.Join(s.WorkDir, "scan_risk_analysis", s.ScanID)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	totals := ReportTotals{
		ScanID:            s.ScanID,
		OriginalRiskCount: len(s.RawRisks),
		MergedGroupCount:  len(s.Groups),
		PocScriptCount:    len(s.PocArtifacts),
	}
	for _, d := range s.Decisions {
		switch d.Status {
		case FPNotIssue:
			totals.NotIssueCount++
		case FPIsIssue:
			totals.IsIssueCount++
		default:
			totals.SuspiciousCount++
		}
	}
	totals.NonFalsePositiveCount = totals.IsIssueCount + totals.SuspiciousCount

	programs := []string{}
	taskStatus := ""
	if s.Task != nil {
		taskStatus = s.Task.Status
		if strings.TrimSpace(s.Task.Programs) != "" {
			programs = strings.Split(s.Task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
		}
	}
	riskRows := buildRiskRowSummaries(s.Groups, s.Decisions)
	report := &FinalAnalysisReport{
		ScanID:              s.ScanID,
		TaskStatus:          taskStatus,
		Programs:            programs,
		Totals:              totals,
		Groups:              s.Groups,
		FPDecisions:         s.Decisions,
		PocArtifacts:        s.PocArtifacts,
		RiskRows:            riskRows,
		SourceSSAReportID:   s.SourceSSAReportID,
		SourceSSAReportPath: s.SourceSSAReportPath,
		AIFPReportMarkdown:  s.AIFPReportMarkdown,
	}
	s.Report = report

	summaryPath := filepath.Join(baseDir, "analysis_summary.json")
	if err := writeJSON(summaryPath, report); err != nil {
		return err
	}
	manifestPath := filepath.Join(baseDir, "poc_manifest.json")
	if err := writeJSON(manifestPath, s.PocArtifacts); err != nil {
		return err
	}
	markdownPath := filepath.Join(baseDir, "analysis_report.md")
	if err := os.WriteFile(markdownPath, []byte(buildMarkdownReport(report)), 0o644); err != nil {
		return err
	}
	fpPath := filepath.Join(baseDir, "false_positive_report.md")
	fpBody := strings.TrimSpace(s.AIFPReportMarkdown)
	if fpBody == "" {
		fpBody = buildFalsePositiveStandaloneMarkdown(report)
	}
	if err := os.WriteFile(fpPath, []byte(fpBody), 0o644); err != nil {
		return err
	}
	pocPath := filepath.Join(baseDir, "poc_generation_report.md")
	if err := os.WriteFile(pocPath, []byte(buildPocGenerationStandaloneMarkdown(report)), 0o644); err != nil {
		return err
	}
	s.Phase = PhaseDone
	return nil
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func buildPOCScript(g MergedRiskGroup, category, scriptType string) string {
	var ur UnifiedRisk
	if len(g.Risks) > 0 {
		ur = g.Risks[0]
	}
	ctx := formatRiskContextComment(ur, g)
	switch scriptType {
	case "python":
		return fmt.Sprintf(`# PoC 模板 — 分组 %s / 类型 %s
# 说明：以下为工程占位脚本，请按目标环境改写 URL 与参数。
# 代表告警（合并组内首条）上下文：
%s
import requests

TARGET = "http://127.0.0.1:8080"
PAYLOAD = "' OR 1=1 -- "

def run():
    url = TARGET + "/replace-me"
    resp = requests.get(url, params={"q": PAYLOAD}, timeout=10)
    print("status:", resp.status_code)
    print(resp.text[:300])

if __name__ == "__main__":
    run()
`, g.GroupID, category, ctx)
	case "shell":
		return fmt.Sprintf(`# PoC 模板 — 分组 %s / 类型 %s
# 代表告警上下文：
%s
set -euo pipefail
TARGET="http://127.0.0.1:8080/replace-me"
curl -i "$TARGET?cmd=id"
`, g.GroupID, category, ctx)
	default:
		return fmt.Sprintf(`# PoC 模板 — 分组 %s / 类型 %s
# 代表告警上下文：
%s
GET http://127.0.0.1:8080/replace-me?q=test
`, g.GroupID, category, ctx)
	}
}

func formatRiskContextComment(ur UnifiedRisk, g MergedRiskGroup) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("#   group=%s merge_key=%s\n", g.GroupID, g.Key))
	if ur.ID != 0 {
		b.WriteString(fmt.Sprintf("#   risk_id=%d severity=%s rule=%s\n", ur.ID, ur.Severity, ur.FromRule))
	}
	if strings.TrimSpace(ur.CodeSourceURL) != "" {
		b.WriteString(fmt.Sprintf("#   location=%s:%d func=%s\n", ur.CodeSourceURL, ur.Line, ur.FunctionName))
	}
	if strings.TrimSpace(ur.Title) != "" {
		b.WriteString("#   title: " + strings.ReplaceAll(strings.TrimSpace(ur.Title), "\n", " ") + "\n")
	}
	if frag := strings.TrimSpace(ur.CodeFragment); frag != "" {
		if len(frag) > 600 {
			frag = frag[:600] + "...(truncated)"
		}
		b.WriteString("#   code_fragment:\n")
		for _, ln := range strings.Split(frag, "\n") {
			b.WriteString("#   | " + ln + "\n")
		}
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
