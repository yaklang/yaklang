//go:build hids && linux

package runtime

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

type scanFinding struct {
	RuleID   string
	Severity string
	Title    string
	Tags     []string
	Detail   map[string]any
}

func buildSingleFileScanSummary(path string, artifact *model.Artifact) map[string]any {
	summary := map[string]any{
		"mode":     "single_file",
		"artifact": artifactDetailMap(artifact),
	}
	findings, matchedRules := matchScanArtifactFindings(path, artifact)
	if len(findings) > 0 {
		summary["findings"] = findings
		summary["finding_count"] = len(findings)
	}
	if len(matchedRules) > 0 {
		summary["matched_rules"] = matchedRules
	}
	return summary
}

func buildDirectoryScanSummary(result boundedDirectoryScanResult) map[string]any {
	entries := make([]map[string]any, 0, len(result.Entries))
	aggregateFindings := make([]map[string]any, 0)
	aggregateRules := map[string]struct{}{}

	for _, entry := range result.Entries {
		item := map[string]any{
			"path":          entry.Path,
			"relative_path": entry.RelativePath,
			"depth":         entry.Depth,
			"is_dir":        entry.IsDir,
		}
		if entry.Artifact != nil {
			item["artifact"] = artifactDetailMap(entry.Artifact)
		}

		findings, matchedRules := matchScanArtifactFindings(entry.Path, entry.Artifact)
		if len(findings) > 0 {
			item["findings"] = findings
			item["finding_count"] = len(findings)
			for _, finding := range findings {
				cloned := cloneEvidenceMap(finding)
				cloned["path"] = entry.Path
				cloned["relative_path"] = entry.RelativePath
				aggregateFindings = append(aggregateFindings, cloned)
			}
		}
		if len(matchedRules) > 0 {
			item["matched_rules"] = matchedRules
			for _, ruleID := range matchedRules {
				aggregateRules[ruleID] = struct{}{}
			}
		}
		entries = append(entries, item)
	}

	summary := map[string]any{
		"mode":            "directory",
		"recursive":       result.Recursive,
		"max_entries":     result.MaxEntries,
		"max_depth":       result.MaxDepth,
		"scanned_count":   result.ScannedCount,
		"file_count":      result.FileCount,
		"directory_count": result.DirectoryCount,
		"truncated":       result.Truncated,
		"entries":         entries,
	}
	if result.RootArtifact != nil {
		summary["target"] = artifactDetailMap(result.RootArtifact)
	}

	if len(aggregateFindings) > 0 {
		sort.Slice(aggregateFindings, func(left, right int) bool {
			leftPath, _ := aggregateFindings[left]["relative_path"].(string)
			rightPath, _ := aggregateFindings[right]["relative_path"].(string)
			if leftPath == rightPath {
				leftRule, _ := aggregateFindings[left]["rule_id"].(string)
				rightRule, _ := aggregateFindings[right]["rule_id"].(string)
				return leftRule < rightRule
			}
			return leftPath < rightPath
		})
		summary["findings"] = aggregateFindings
		summary["finding_count"] = len(aggregateFindings)
	}

	if len(aggregateRules) > 0 {
		matchedRules := make([]string, 0, len(aggregateRules))
		for ruleID := range aggregateRules {
			matchedRules = append(matchedRules, ruleID)
		}
		sort.Strings(matchedRules)
		summary["matched_rules"] = matchedRules
	}

	return summary
}

func matchScanArtifactFindings(path string, artifact *model.Artifact) ([]map[string]any, []string) {
	findings := buildScanFindings(path, artifact)
	if len(findings) == 0 {
		return nil, nil
	}

	maps := make([]map[string]any, 0, len(findings))
	rules := make([]string, 0, len(findings))
	for _, finding := range findings {
		maps = append(maps, scanFindingMap(finding))
		rules = append(rules, finding.RuleID)
	}
	sort.Strings(rules)
	return maps, dedupeSortedStrings(rules)
}

func buildScanFindings(path string, artifact *model.Artifact) []scanFinding {
	if artifact == nil || !artifact.Exists {
		return nil
	}

	normalizedPath := policy.NormalizePath(path)
	if normalizedPath == "" {
		return nil
	}

	findings := make([]scanFinding, 0, 4)
	if policy.IsAuthorizedKeysPath(normalizedPath) {
		findings = append(findings, scanFinding{
			RuleID:   "linux.scan.authorized_keys_artifact",
			Severity: "high",
			Title:    "ssh authorized_keys artifact found during bounded scan",
			Tags:     []string{"builtin", "scan", "file", "ssh"},
			Detail:   scanFindingArtifactDetail(normalizedPath, artifact),
		})
	}
	if policy.IsSensitiveSystemPath(normalizedPath) {
		findings = append(findings, scanFinding{
			RuleID:   "linux.scan.sensitive_path_artifact",
			Severity: "high",
			Title:    "sensitive system path artifact found during bounded scan",
			Tags:     []string{"builtin", "scan", "file", "integrity"},
			Detail:   scanFindingArtifactDetail(normalizedPath, artifact),
		})
	}
	if policy.IsWritableTmpPath(normalizedPath) && hasELFArtifactSnapshot(artifact) {
		findings = append(findings, scanFinding{
			RuleID:   "linux.scan.writable_tmp_elf_artifact",
			Severity: "high",
			Title:    "ELF artifact found under writable tmp path during bounded scan",
			Tags:     []string{"builtin", "scan", "file", "tmp", "artifact", "elf"},
			Detail:   scanFindingArtifactDetail(normalizedPath, artifact),
		})
	}
	if policy.IsSystemELFArtifactPath(normalizedPath) && hasELFArtifactSnapshot(artifact) {
		findings = append(findings, scanFinding{
			RuleID:   "linux.scan.system_elf_artifact",
			Severity: "medium",
			Title:    "system ELF artifact found during bounded scan",
			Tags:     []string{"builtin", "scan", "file", "system", "artifact", "elf"},
			Detail:   scanFindingArtifactDetail(normalizedPath, artifact),
		})
	}
	return findings
}

func hasELFArtifactSnapshot(artifact *model.Artifact) bool {
	if artifact == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(artifact.FileType), "elf") {
		return true
	}
	if artifact.ELF != nil {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(artifact.Magic)), "7f454c46")
}

func scanFindingArtifactDetail(path string, artifact *model.Artifact) map[string]any {
	detail := map[string]any{
		"path":      path,
		"file_type": artifact.FileType,
	}
	if artifact.Hashes != nil {
		if artifact.Hashes.SHA256 != "" {
			detail["sha256"] = artifact.Hashes.SHA256
		}
		if artifact.Hashes.MD5 != "" {
			detail["md5"] = artifact.Hashes.MD5
		}
	}
	return detail
}

func scanFindingMap(finding scanFinding) map[string]any {
	item := map[string]any{
		"rule_id":  finding.RuleID,
		"severity": finding.Severity,
		"title":    finding.Title,
	}
	if len(finding.Tags) > 0 {
		item["tags"] = cloneStringSlice(finding.Tags)
	}
	if len(finding.Detail) > 0 {
		item["detail"] = cloneEvidenceMap(finding.Detail)
	}
	return item
}

func dedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	last := ""
	for index, value := range values {
		if index == 0 || value != last {
			result = append(result, value)
		}
		last = value
	}
	return result
}
