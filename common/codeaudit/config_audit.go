package codeaudit

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/catalog"
	"github.com/yaklang/yaklang/common/codeaudit/sfaudit"
)

// AuditConfig performs configuration security auditing for a given framework.
// Rule matching is delegated to the SyntaxFlow source rules embedded in
// sfaudit; catalog.ConfigCheckRule entries carry the metadata.
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
	configFiles := runConfigRules(context.Background(), report, rules, idx, o)

	report.Artifacts = map[string]any{
		"config_files":  configFiles,
		"audit_options": o,
		"scan_root":     root,
		"framework":     framework,
		"rules_checked": len(rules),
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
		runConfigRules(context.Background(), report, rules, idx, o)
	}

	report.Artifacts = map[string]any{
		"detected_cms_products": cmsProducts,
		"audit_options":         o,
		"scan_root":             root,
	}
	return report.Finish(start, idx.FilesScanned, o)
}

// runConfigRules executes config rules over their collected files and appends
// findings to the report. It returns the deduplicated list of config files
// that were examined. File contents are read at most once via cache.
func runConfigRules(ctx context.Context, report *Report, rules []catalog.ConfigCheckRule, idx *FSIndex, o *ProbeOptions) []string {
	cache := map[string]string{}
	var configFiles []string

	for _, rule := range rules {
		files := collectConfigFiles(idx, rule.FilePatterns, o)
		configFiles = append(configFiles, files...)

		subset := map[string]string{}
		for _, fp := range files {
			if _, ok := cache[fp]; !ok {
				if content, ok := ReadFileLimited(fp, 1000000); ok {
					cache[fp] = content
				} else {
					cache[fp] = "" // mark unreadable
				}
			}
			if cache[fp] != "" {
				subset[fp] = cache[fp]
			}
		}
		if len(subset) == 0 {
			continue
		}

		hits, err := sfaudit.NewEngine("codeaudit-config", subset).Run(ctx, rule.ID)
		if err != nil {
			report.Status = "partial"
			continue
		}

		if rule.ReportWhenAbsent {
			// Absence rules report one finding per collected file without hits
			// (e.g. web.xml missing security-constraint).
			hitFiles := map[string]bool{}
			for _, h := range hits {
				hitFiles[h.File] = true
			}
			for fp := range subset {
				if !hitFiles[fp] {
					report.AddFinding(rule.ID, rule.Severity, rule.Title, rule.Recommendation,
						[]Evidence{{File: fp, Snippet: "no security-constraint defined"}})
				}
			}
			continue
		}

		for _, h := range hits {
			snippet := h.Snippet
			if rule.MaskValue {
				snippet = maskValueInSnippet(snippet)
			}
			report.AddFinding(rule.ID, rule.Severity, rule.Title, rule.Recommendation,
				[]Evidence{{File: h.File, Line: h.Line, Snippet: snippet}})
		}
	}

	return dedupStrings(configFiles)
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

// maskValueInSnippet masks the value part (after the first = or :) of a
// single-line evidence snippet, mirroring the previous behavior of reporting
// masked credential values.
func maskValueInSnippet(line string) string {
	idx := strings.IndexAny(line, "=:")
	if idx < 0 {
		return line
	}
	key := line[:idx+1]
	value := strings.TrimSpace(line[idx+1:])
	if value == "" {
		return line
	}
	return key + " " + maskSecretValue(value)
}

// maskSecretValue masks the middle part of a secret for display.
func maskSecretValue(v string) string {
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}
