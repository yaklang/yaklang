package scannode

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

type legionSyntaxFlowRuleCandidateSummary struct {
	SchemaVersion      string                        `json:"schema_version"`
	RuleName           string                        `json:"rule_name"`
	Language           string                        `json:"language"`
	RuleContent        string                        `json:"rule_content"`
	RuleSHA256         string                        `json:"rule_sha256"`
	Explanation        string                        `json:"explanation"`
	Limitations        []string                      `json:"limitations"`
	VerificationStatus string                        `json:"verification_status"`
	DebugResults       []legionSyntaxFlowDebugResult `json:"debug_results"`
	WorkspaceRevision  string                        `json:"workspace_revision"`
	SourceSHA256       string                        `json:"source_sha256"`
}

func (r *legionServerFocusRuntime) submitSyntaxFlowRuleCandidate(params map[string]any) (map[string]any, error) {
	r.ruleMu.Lock()
	defer r.ruleMu.Unlock()
	ctx, generation, err := r.beginRuleOperation(serverFocusCapabilityRuleCandidate, false)
	if err != nil {
		return nil, err
	}
	// Evidence fields, selectors, and opaque summaries are not accepted.
	if err := rejectLegionRuleExtraParams(params, "title", "rule_name", "language", "rule", "explanation", "limitations", "markdown"); err != nil {
		return nil, err
	}
	title, err := legionRuleString(params, "title", 256, true)
	if err != nil {
		return nil, err
	}
	ruleName, err := legionRuleString(params, "rule_name", 128, true)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(ruleName, "/\\\r\n") || strings.TrimSpace(ruleName) != ruleName {
		return nil, fmt.Errorf("rule_name must be a simple name without a path")
	}
	languageText, err := legionRuleString(params, "language", 32, true)
	if err != nil {
		return nil, err
	}
	language, err := normalizeLegionSyntaxFlowLanguage(languageText)
	if err != nil {
		return nil, err
	}
	rule, err := legionRuleString(params, "rule", legionSyntaxFlowMaxRuleBytes, true)
	if err != nil {
		return nil, err
	}
	explanation, err := legionRuleString(params, "explanation", 8*1024, true)
	if err != nil {
		return nil, err
	}
	// Optional model Markdown is never the authoritative report. Validate its
	// bounds for compatibility, then render the candidate and observed facts.
	if _, err := legionRuleString(params, "markdown", 32*1024, false); err != nil {
		return nil, err
	}
	limitations := []string{}
	if raw, present := params["limitations"]; present {
		limitations, err = legionRuleStringList(raw, 16)
		if err != nil {
			return nil, fmt.Errorf("limitations: %w", err)
		}
		for _, limitation := range limitations {
			if len(limitation) > 1024 || !utf8.ValidString(limitation) || strings.ContainsRune(limitation, '\x00') {
				return nil, fmt.Errorf("each limitation must be bounded UTF-8 text")
			}
		}
	}
	frame, err := compileLegionSyntaxFlowRule(rule)
	if err != nil {
		return nil, fmt.Errorf("rule candidate is invalid: %s", trimLegionRuleText(err.Error(), legionSyntaxFlowMaxDiagnosticBytes))
	}
	hasAlert := false
	for _, code := range frame.Codes {
		if code != nil && code.OpCode == sfvm.OpAlert && code.UnaryStr != "" {
			hasAlert = true
			break
		}
	}
	if !hasAlert {
		return nil, fmt.Errorf("rule candidate must contain a compiled alert definition; intermediate queries belong in check/debug")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ruleGenerationActive(generation) || ctx.Err() != nil {
		return nil, fmt.Errorf("rule candidate Turn is no longer active")
	}
	resultContract, ok := r.activeExecutionContract.resultForCapability(serverFocusCapabilityRuleCandidate)
	if !ok || resultContract.Kind != legionSyntaxFlowRuleCandidateKind {
		return nil, fmt.Errorf("rule candidate has no dedicated immutable result contract")
	}
	sink, ok := r.sink.(aiFocusCodeResultSink)
	if !ok {
		return nil, fmt.Errorf("result sink does not accept rule candidate reports")
	}
	summary := legionSyntaxFlowRuleCandidateSummary{
		SchemaVersion: legionSyntaxFlowCandidateSchema, RuleName: ruleName,
		Language: language.String(), RuleContent: rule, RuleSHA256: legionRuleHash(rule),
		Explanation: explanation, Limitations: limitations,
		DebugResults:      cloneLegionRuleDebugHistory(r.ruleDebugHistory),
		WorkspaceRevision: r.workspace.lockedRevision, SourceSHA256: r.workspace.sha256,
	}
	summary.VerificationStatus = legionRuleVerificationStatus(summary.RuleSHA256, summary.Language, summary.DebugResults)
	raw, err := marshalBoundedLegionRuleCandidate(&summary)
	if err != nil {
		return nil, err
	}
	report := aiFocusCodeAuditReport{
		WorkspaceID: r.workspace.spec.WorkspaceID, Title: title,
		Markdown:                     renderLegionRuleCandidateMarkdown(title, summary),
		StructuredSummary:            raw,
		validatedRuleCandidateSHA256: legionRuleHash(string(raw)),
	}
	// Hold the generation fence through submission. A follow-up cannot switch
	// the active Focus or reuse this history while its report is being sent.
	receipt, err := sink.SubmitCodeAuditReport(ctx, resultContract.Kind, report)
	if err != nil {
		return nil, err
	}
	result := focusResultReceiptMap(receipt)
	result["rule_sha256"] = summary.RuleSHA256
	result["verification_status"] = summary.VerificationStatus
	return result, nil
}

func cloneLegionRuleDebugHistory(history []legionSyntaxFlowDebugResult) []legionSyntaxFlowDebugResult {
	result := make([]legionSyntaxFlowDebugResult, 0, len(history))
	for _, item := range history {
		item.Matches = append([]legionSyntaxFlowMatch{}, item.Matches...)
		item.Diagnostics = append([]string{}, item.Diagnostics...)
		result = append(result, item)
	}
	return result
}

func legionRuleVerificationStatus(hash, language string, history []legionSyntaxFlowDebugResult) string {
	status := "syntax_only"
	for _, item := range history {
		if item.RuleSHA256 != hash || item.Language != language {
			continue
		}
		if item.Status == "completed" {
			// "tested" means an actual debug execution, never regression
			// acceptance or a claim that match_count==0 proves correctness.
			return "tested"
		}
		status = "debug_failed"
	}
	return status
}

func marshalBoundedLegionRuleCandidate(summary *legionSyntaxFlowRuleCandidateSummary) ([]byte, error) {
	for {
		raw, err := json.Marshal(summary)
		if err != nil {
			return nil, err
		}
		if len(raw) <= maxInlineCodeAuditSummaryBytes {
			return raw, nil
		}
		// Preserve the exact candidate and every distinct baseline/candidate
		// run's status, hashes, expected result, and observed match count.
		largest := -1
		for i := range summary.DebugResults {
			if len(summary.DebugResults[i].Matches) > 0 &&
				(largest < 0 || len(summary.DebugResults[i].Matches) > len(summary.DebugResults[largest].Matches)) {
				largest = i
			}
		}
		if largest >= 0 {
			item := &summary.DebugResults[largest]
			item.Matches = item.Matches[:len(item.Matches)-1]
			item.Truncated = true
			continue
		}
		trimmed := false
		for i := range summary.DebugResults {
			item := &summary.DebugResults[i]
			if len(item.Diagnostics) > 0 {
				item.Diagnostics = []string{}
				item.Truncated = true
				trimmed = true
			}
		}
		if trimmed {
			continue
		}
		// A pathological rule/narrative can itself exceed the JSON transport
		// limit after escaping. Never silently alter the candidate rule.
		return nil, fmt.Errorf("rule candidate exceeds %d encoded bytes even without snippets; shorten the rule or explanation", maxInlineCodeAuditSummaryBytes)
	}
}

func renderLegionRuleCandidateMarkdown(title string, summary legionSyntaxFlowRuleCandidateSummary) string {
	var text strings.Builder
	fmt.Fprintf(&text, "# %s\n\n", strings.ReplaceAll(strings.ReplaceAll(title, "\r", " "), "\n", " "))
	fmt.Fprintf(&text, "规则名称：%s\n\n语言：%s\n\n规则 SHA-256：%s\n\n", summary.RuleName, summary.Language, summary.RuleSHA256)
	fence := "```"
	for strings.Contains(summary.RuleContent, fence) {
		fence += "`"
	}
	fmt.Fprintf(&text, "## 候选规则\n\n%ssyntaxflow\n%s\n%s\n\n", fence, summary.RuleContent, fence)
	fmt.Fprintf(&text, "## 规则说明（模型生成）\n\n%s\n\n", summary.Explanation)
	fmt.Fprintf(&text, "## 运行时验证事实\n\n验证状态：%s。tested 仅表示当前规则在本次任务中完成过真实调试，不表示回归通过。\n\n", summary.VerificationStatus)
	fmt.Fprintf(&text, "工作区版本：%s\n\n工作区摘要：%s\n\n", summary.WorkspaceRevision, summary.SourceSHA256)
	if len(summary.DebugResults) == 0 {
		text.WriteString("本次任务没有真实调试记录，仅完成语法检查。\n\n")
	}
	for _, result := range summary.DebugResults {
		origin := "授权工作区文件"
		if result.SourceKind == "inline_sample" {
			origin = "生成或改写的内联样例（不代表原项目）"
			if result.SampleOrigin == "user_supplied" {
				origin = "用户提供的内联样例（不代表原项目）"
			}
		}
		conclusion := "仅观察"
		if result.Status != "completed" {
			conclusion = "未完成，不能作验证结论"
		} else if result.Expected == "match" {
			conclusion = "未符合预期"
			if result.MatchCount > 0 {
				conclusion = "符合本条命中预期"
			}
		} else if result.Expected == "no_match" {
			conclusion = "未符合预期"
			if result.MatchCount == 0 {
				conclusion = "符合本条不命中预期"
			}
		}
		fmt.Fprintf(&text, "- 调试 %s；规则 %s；来源：%s；状态：%s；告警命中：%d；预期：%s；%s。\n", result.DebugID, result.RuleSHA256, origin, result.Status, result.MatchCount, result.Expected, conclusion)
	}
	text.WriteString("\n调试只覆盖显式选择的有界源码；零命中不能证明规则或项目没有问题。候选规则尚未发布，也不会自动修改风险处置。\n")
	if len(summary.Limitations) > 0 {
		text.WriteString("\n## 局限（模型说明）\n\n")
		for _, limitation := range summary.Limitations {
			fmt.Fprintf(&text, "- %s\n", limitation)
		}
	}
	return text.String()
}

func (s *legionAIFocusResultSink) validateRuleCandidateReport(report aiFocusCodeAuditReport) error {
	if report.validatedRuleCandidateSHA256 == "" || report.validatedRuleCandidateSHA256 != legionRuleHash(string(report.StructuredSummary)) {
		return fmt.Errorf("rule candidate report requires runtime-computed evidence")
	}
	s.mu.Lock()
	allowed := s.ruleCandidateAllowed
	s.mu.Unlock()
	if !allowed {
		return fmt.Errorf("rule candidate report is not allowed by the bound immutable contract")
	}
	var summary legionSyntaxFlowRuleCandidateSummary
	if err := json.Unmarshal(report.StructuredSummary, &summary); err != nil {
		return fmt.Errorf("invalid rule candidate summary")
	}
	revision, sourceHash, err := s.codeWorkspaceEvidence()
	if err != nil {
		return err
	}
	if summary.SchemaVersion != legionSyntaxFlowCandidateSchema || summary.RuleSHA256 != legionRuleHash(summary.RuleContent) ||
		summary.WorkspaceRevision != revision || summary.SourceSHA256 != sourceHash ||
		summary.VerificationStatus != legionRuleVerificationStatus(summary.RuleSHA256, summary.Language, summary.DebugResults) {
		return fmt.Errorf("rule candidate summary does not match its runtime evidence")
	}
	return nil
}
