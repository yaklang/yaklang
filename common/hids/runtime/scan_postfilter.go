//go:build hids && linux

package runtime

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/hids/rule"
)

type scanPostFilterConfig struct {
	ScanMatch    string
	EntryMatch   string
	FindingMatch string
	MatchedOnly  bool
}

func parseScanPostFilterConfig(request map[string]any) scanPostFilterConfig {
	metadata := readEvidenceMetadata(request)
	return scanPostFilterConfig{
		ScanMatch: firstNonEmptyEvidenceString(
			readEvidenceString(metadata, "scan_match"),
			readEvidenceString(metadata, "match"),
			readEvidenceString(metadata, "filter"),
		),
		EntryMatch:   strings.TrimSpace(readEvidenceString(metadata, "entry_match")),
		FindingMatch: strings.TrimSpace(readEvidenceString(metadata, "finding_match")),
		MatchedOnly: anyToBool(firstNonNilEvidence(
			metadata["matched_only"],
			metadata["emit_only_matched"],
			metadata["only_matched"],
			request["matched_only"],
		)),
	}
}

func (p *pipeline) applyScanPostFilters(
	request map[string]any,
	summary map[string]any,
) (map[string]any, error) {
	config := parseScanPostFilterConfig(request)
	if len(summary) == 0 || isEmptyScanPostFilterConfig(config) {
		return summary, nil
	}

	if matched, ok, err := p.evaluateScanMatch(config.ScanMatch, request, summary); err != nil {
		return nil, fmt.Errorf("scan_match: %w", err)
	} else if ok {
		summary["scan_match"] = map[string]any{
			"expression": config.ScanMatch,
			"matched":    matched,
		}
	}

	if matches, ok, err := p.evaluateEntryMatches(config.EntryMatch, request, summary); err != nil {
		return nil, fmt.Errorf("entry_match: %w", err)
	} else if ok {
		summary["matched_entries"] = matches
		summary["matched_entry_count"] = len(matches)
		summary["entry_match"] = map[string]any{
			"expression": config.EntryMatch,
			"matched":    len(matches),
		}
		if config.MatchedOnly {
			summary["entries"] = matches
		}
	}

	if matches, ok, err := p.evaluateFindingMatches(config.FindingMatch, request, summary); err != nil {
		return nil, fmt.Errorf("finding_match: %w", err)
	} else if ok {
		summary["matched_findings"] = matches
		summary["matched_finding_count"] = len(matches)
		summary["finding_match"] = map[string]any{
			"expression": config.FindingMatch,
			"matched":    len(matches),
		}
		if config.MatchedOnly {
			summary["findings"] = matches
			summary["finding_count"] = len(matches)
		}
	}

	if config.MatchedOnly {
		summary["matched_only"] = true
	}
	return summary, nil
}

func (p *pipeline) evaluateScanMatch(
	expression string,
	request map[string]any,
	summary map[string]any,
) (bool, bool, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return false, false, nil
	}
	matched, err := rule.EvaluateBooleanExpression(
		p.scanSandbox,
		expression,
		rule.BuildScanEvaluationContext(summary, request, nil, nil),
	)
	return matched, true, err
}

func (p *pipeline) evaluateEntryMatches(
	expression string,
	request map[string]any,
	summary map[string]any,
) ([]map[string]any, bool, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, false, nil
	}

	entries, ok := toEvidenceMapSlice(summary["entries"])
	if !ok {
		return []map[string]any{}, true, nil
	}
	matches := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		matched, err := rule.EvaluateBooleanExpression(
			p.scanSandbox,
			expression,
			rule.BuildScanEvaluationContext(summary, request, entry, nil),
		)
		if err != nil {
			return nil, true, err
		}
		if matched {
			matches = append(matches, cloneEvidenceMap(entry))
		}
	}
	return matches, true, nil
}

func (p *pipeline) evaluateFindingMatches(
	expression string,
	request map[string]any,
	summary map[string]any,
) ([]map[string]any, bool, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, false, nil
	}

	findings, ok := toEvidenceMapSlice(summary["findings"])
	if !ok {
		return []map[string]any{}, true, nil
	}
	matches := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		matched, err := rule.EvaluateBooleanExpression(
			p.scanSandbox,
			expression,
			rule.BuildScanEvaluationContext(summary, request, nil, finding),
		)
		if err != nil {
			return nil, true, err
		}
		if matched {
			matches = append(matches, cloneEvidenceMap(finding))
		}
	}
	return matches, true, nil
}

func isEmptyScanPostFilterConfig(config scanPostFilterConfig) bool {
	return config.ScanMatch == "" &&
		config.EntryMatch == "" &&
		config.FindingMatch == "" &&
		!config.MatchedOnly
}

func toEvidenceMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, cloneEvidenceMap(item))
		}
		return result, true
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped := readEvidenceMap(item)
			if len(mapped) == 0 {
				continue
			}
			result = append(result, cloneEvidenceMap(mapped))
		}
		return result, true
	default:
		return nil, false
	}
}

func firstNonEmptyEvidenceString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
