//go:build hids && linux

package runtime

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
)

type scanPromotionPolicy struct {
	Category string
}

func (p *pipeline) promoteAlertFromEvidence(alert model.Alert) model.Alert {
	results, ok := toEvidenceMapSlice(alert.Detail["evidence_results"])
	if !ok || len(results) == 0 {
		return alert
	}

	promotedFindings, promotedRuleIDs, promotedTags, categories, highestSeverity :=
		collectPromotedScanFindings(results)
	if len(promotedFindings) == 0 {
		return alert
	}

	summary := map[string]any{
		"promoted":          true,
		"finding_count":     len(promotedFindings),
		"findings":          promotedFindings,
		"promoted_rule_ids": promotedRuleIDs,
		"promoted_tags":     promotedTags,
		"categories":        categories,
		"highest_severity":  highestSeverity,
	}

	previousSeverity := normalizePromotionSeverity(alert.Severity)
	if severityShouldReplace(previousSeverity, highestSeverity) {
		summary["previous_alert_severity"] = alert.Severity
		summary["severity_elevated"] = true
		alert.Severity = highestSeverity
	}
	if len(promotedTags) > 0 {
		alert.Tags = mergeRuntimeTags(alert.Tags, promotedTags)
	}
	alert.Detail["scan_promotion"] = summary
	return alert
}

func collectPromotedScanFindings(results []map[string]any) ([]map[string]any, []string, []string, []string, string) {
	promotedFindings := make([]map[string]any, 0)
	promotedRuleSet := map[string]struct{}{}
	promotedTagSet := map[string]struct{}{}
	categorySet := map[string]struct{}{}
	highestSeverity := ""

	for _, result := range results {
		scan := readEvidenceMap(result["scan"])
		if len(scan) == 0 {
			continue
		}
		findings, ok := toEvidenceMapSlice(scan["findings"])
		if !ok || len(findings) == 0 {
			continue
		}

		sourceKind := strings.TrimSpace(readEvidenceString(result, "kind"))
		for _, finding := range findings {
			ruleID := strings.TrimSpace(readEvidenceString(finding, "rule_id"))
			policy, promote := lookupScanPromotionPolicy(ruleID)
			if !promote {
				continue
			}

			promoted := cloneEvidenceMap(finding)
			if sourceKind != "" {
				promoted["source_kind"] = sourceKind
			}
			if category := strings.TrimSpace(policy.Category); category != "" {
				promoted["category"] = category
				categorySet[category] = struct{}{}
			}
			promotedFindings = append(promotedFindings, promoted)
			promotedRuleSet[ruleID] = struct{}{}
			for _, tag := range toStringSlice(finding["tags"]) {
				promotedTagSet[tag] = struct{}{}
			}
			highestSeverity = maxPromotionSeverity(highestSeverity, readEvidenceString(finding, "severity"))
		}
	}

	return promotedFindings,
		sortedStringSet(promotedRuleSet),
		sortedStringSet(promotedTagSet),
		sortedStringSet(categorySet),
		highestSeverity
}

func lookupScanPromotionPolicy(ruleID string) (scanPromotionPolicy, bool) {
	switch strings.TrimSpace(ruleID) {
	case "linux.scan.authorized_keys_artifact":
		return scanPromotionPolicy{Category: "credential_access"}, true
	case "linux.scan.sensitive_path_artifact":
		return scanPromotionPolicy{Category: "integrity"}, true
	case "linux.scan.writable_tmp_elf_artifact":
		return scanPromotionPolicy{Category: "dropper"}, true
	default:
		return scanPromotionPolicy{}, false
	}
}

func severityShouldReplace(current string, next string) bool {
	return promotionSeverityRank(next) > promotionSeverityRank(current)
}

func maxPromotionSeverity(left string, right string) string {
	left = normalizePromotionSeverity(left)
	right = normalizePromotionSeverity(right)
	if promotionSeverityRank(right) > promotionSeverityRank(left) {
		return right
	}
	return left
}

func normalizePromotionSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	default:
		return "unknown"
	}
}

func promotionSeverityRank(value string) int {
	switch normalizePromotionSeverity(value) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func mergeRuntimeTags(left []string, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(left)+len(right))
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			text := strings.TrimSpace(readEvidenceString(map[string]any{"value": item}, "value"))
			if text == "" {
				continue
			}
			items = append(items, text)
		}
		return items
	default:
		return nil
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
