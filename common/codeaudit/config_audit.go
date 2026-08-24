package codeaudit

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/catalog"
)

// AuditConfig performs configuration security auditing for a given framework.
func AuditConfig(target string, framework string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	rules := catalog.GetConfigRules(o.Language, framework)
	if o.ConfigScope == "all" {
		// Get all rules regardless of framework
		rules = catalog.JavaConfigRules
	}

	report := NewReport("codeaudit/config_audit", root, framework)
	configFiles := []string{}

	for _, rule := range rules {
		files := collectConfigFiles(idx, rule.FilePatterns, o)
		for _, fp := range files {
			content, ok := ReadFileLimited(fp, 1000000)
			if !ok {
				continue
			}
			kv := parseConfigKV(content, fp)
			finding := rule.Check(content, kv, fp)
			if finding != nil {
				ev := convertEvidence(finding.Evidence, content)
				report.AddFinding(finding.ID, finding.Severity, finding.Title, finding.Recommendation, ev)
			}
		}
		configFiles = append(configFiles, files...)
	}

	configFiles = dedupStrings(configFiles)
	report.Artifacts = map[string]any{
		"config_files":   configFiles,
		"audit_options":  o,
		"scan_root":      root,
		"framework":      framework,
		"rules_checked":  len(rules),
	}
	return report.Finish(start, len(configFiles), o)
}

// AuditCmsProduct performs CMS product-specific auditing.
func AuditCmsProduct(target string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	// First detect CMS products
	cmsProducts := detectCmsProducts(idx, o)

	report := NewReport("codeaudit/cms_audit", root, o.Language)

	for _, cms := range cmsProducts {
		rules := catalog.GetCmsConfigRules(cms.ID)
		for _, rule := range rules {
			files := collectConfigFiles(idx, rule.FilePatterns, o)
			for _, fp := range files {
				content, ok := ReadFileLimited(fp, 1000000)
				if !ok {
					continue
				}
				kv := parseConfigKV(content, fp)
				finding := rule.Check(content, kv, fp)
				if finding != nil {
					ev := convertEvidence(finding.Evidence, content)
					report.AddFinding(finding.ID, finding.Severity, finding.Title, finding.Recommendation, ev)
				}
			}
		}
	}

	report.Artifacts = map[string]any{
		"detected_cms_products": cmsProducts,
		"audit_options":          o,
		"scan_root":              root,
	}
	return report.Finish(start, idx.FilesScanned, o)
}

// collectConfigFiles finds files matching the given glob patterns.
func collectConfigFiles(idx *FSIndex, patterns []string, opts *ProbeOptions) []string {
	var out []string
	seen := map[string]bool{}

	for _, pattern := range patterns {
		matches := matchGlobInIndex(idx, pattern)
		for _, m := range matches {
			if !seen[m] && !IsExcludedPath(m, opts) {
				seen[m] = true
				out = append(out, m)
			}
		}
	}

	return out
}

// matchGlobInIndex matches a glob pattern against the file index.
func matchGlobInIndex(idx *FSIndex, pattern string) []string {
	var out []string

	// Handle glob patterns with wildcards
	if strings.Contains(pattern, "*") {
		for _, fp := range idx.AllFiles {
			base := filepath.Base(fp)
			matched, err := filepath.Match(pattern, base)
			if err == nil && matched {
				out = append(out, fp)
			}
		}
	} else {
		// Exact match
		out = idx.FindByExactName(pattern)
	}

	return out
}

// convertEvidence converts FindingProxy evidence to Finding evidence.
func convertEvidence(proxyEvidence []catalog.EvidenceProxy, content string) []Evidence {
	out := []Evidence{}
	for _, e := range proxyEvidence {
		line := e.Line
		if line == 0 && content != "" && e.Snippet != "" {
			line = findLineNumber(content, e.Snippet)
		}
		out = append(out, Evidence{
			File:    e.File,
			Line:    line,
			Snippet: e.Snippet,
		})
	}
	return out
}

// findLineNumber finds the line number of a snippet in content.
func findLineNumber(content, snippet string) int {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, snippet) {
			return i + 1
		}
	}
	return 0
}

// parseConfigKV parses configuration content into key-value pairs based on file extension.
func parseConfigKV(content string, filePath string) map[string]string {
	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".properties"):
		return parseProperties(content)
	case strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml"):
		return parseSimpleYaml(content)
	case strings.HasSuffix(lower, ".ini"):
		return parseIni(content)
	case strings.HasSuffix(lower, ".xml"):
		return parseSimpleXml(content)
	case strings.HasSuffix(lower, ".conf"):
		return parseProperties(content) // HOCON/conf often uses properties-style
	default:
		return map[string]string{}
	}
}

// parseProperties parses a .properties file.
func parseProperties(content string) map[string]string {
	kv := map[string]string{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		// Find = or : separator
		sepIdx := -1
		for i, c := range line {
			if c == '=' || c == ':' {
				sepIdx = i
				break
			}
		}
		if sepIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:sepIdx])
		value := strings.TrimSpace(line[sepIdx+1:])
		// Remove surrounding quotes
		value = strings.Trim(value, `"'`)
		kv[key] = value
	}
	return kv
}

// parseSimpleYaml parses a simple YAML file into key-value pairs.
// This handles common flat and two-level nested YAML used in Spring Boot configs.
func parseSimpleYaml(content string) map[string]string {
	kv := map[string]string{}
	lines := strings.Split(content, "\n")

	// Track indentation levels for nested keys
	pathStack := []string{}
	indentStack := []int{}

	for _, line := range lines {
		// Skip comments and empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Calculate indentation
		indent := len(line) - len(strings.TrimLeft(line, " "))

		// Pop stack until we find the parent
		for len(indentStack) > 0 && indent <= indentStack[len(indentStack)-1] {
			indentStack = indentStack[:len(indentStack)-1]
			pathStack = pathStack[:len(pathStack)-1]
		}

		// Parse key: value
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:colonIdx])
		value := strings.TrimSpace(trimmed[colonIdx+1:])

		// Remove quotes from value
		value = strings.Trim(value, `"'`)

		// Build full key path
		currentPath := append([]string{}, pathStack...)
		currentPath = append(currentPath, key)
		fullKey := strings.Join(currentPath, ".")

		if value != "" {
			kv[fullKey] = value
		}

		// Push to stack for potential children
		pathStack = currentPath
		indentStack = append(indentStack, indent)
	}

	return kv
}

// parseIni parses an INI file.
func parseIni(content string) map[string]string {
	kv := map[string]string{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			continue // section header
		}
		sepIdx := strings.Index(line, "=")
		if sepIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:sepIdx])
		value := strings.TrimSpace(line[sepIdx+1:])
		value = strings.Trim(value, `"'`)
		kv[key] = value
	}
	return kv
}

// parseSimpleXml parses a simple XML file extracting key="value" pairs.
func parseSimpleXml(content string) map[string]string {
	kv := map[string]string{}
	// Extract attribute key="value" patterns
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Match name="value" patterns
		attrPattern := `(\w[\w.-]*)\s*=\s*"([^"]*)"`
		matches := findAllStrings(line, attrPattern)
		for i := 0; i+1 < len(matches); i += 2 {
			kv[matches[i]] = matches[i+1]
		}
	}
	return kv
}

// findAllStrings finds all groups matching a regex pattern.
func findAllStrings(s, pattern string) []string {
	re := regexpCompile(pattern)
	if re == nil {
		return nil
	}
	matches := re.FindAllStringSubmatch(s, -1)
	var out []string
	for _, m := range matches {
		for i := 1; i < len(m); i++ {
			out = append(out, m[i])
		}
	}
	return out
}
