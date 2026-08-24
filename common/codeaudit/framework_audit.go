package codeaudit

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/catalog"
)

// RunFrameworkAudit extracts the architecture baseline for a given framework.
// It identifies entry points, config files, and module structure.
func RunFrameworkAudit(target string, framework string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	// Find the framework signal
	var sig *catalog.FrameworkSignal
	for i := range catalog.JavaFrameworkCatalog {
		if catalog.JavaFrameworkCatalog[i].Name == framework {
			sig = &catalog.JavaFrameworkCatalog[i]
			break
		}
	}

	report := NewReport("codeaudit/framework_audit", root, framework)

	entryPoints := []string{}
	configFiles := []string{}
	moduleInfo := map[string]any{}

	if sig != nil {
		// Find entry points based on framework
		entryPoints = findEntryPoints(idx, framework)

		// Find config files
		for _, marker := range sig.FileMarkers {
			configFiles = append(configFiles, idx.FindByName(marker)...)
		}
		// Deduplicate
		configFiles = dedupStrings(configFiles)

		// Module structure
		modules := detectModules(idx)
		moduleInfo = map[string]any{
			"build_system":  detectBuildSystem(idx),
			"modules":       modules,
			"config_files":   configFiles,
			"entry_points":   entryPoints,
		}
	}

	report.Artifacts = map[string]any{
		"framework":     framework,
		"entry_points":  entryPoints,
		"config_files":  configFiles,
		"audit_options": o,
		"scan_root":     root,
		"module_info":   moduleInfo,
	}

	report.Summary = fmt.Sprintf("framework: %s, entry points: %d, config files: %d", framework, len(entryPoints), len(configFiles))

	return report.Finish(start, idx.FilesScanned, o)
}

// findEntryPoints finds Java entry point classes for a given framework.
func findEntryPoints(idx *FSIndex, framework string) []string {
	var out []string

	switch framework {
	case "spring_boot":
		// Look for @SpringBootApplication annotated classes
		for _, fp := range idx.FindByExtension(".java") {
			content, ok := ReadFileLimited(fp, 100000)
			if !ok {
				continue
			}
			if strings.Contains(content, "@SpringBootApplication") {
				out = append(out, fp)
			}
		}
	case "servlet":
		// Look for web.xml servlet definitions
		for _, fp := range idx.FindByExactName("web.xml") {
			out = append(out, fp)
		}
	case "struts2":
		// Look for struts.xml
		for _, fp := range idx.FindByExactName("struts.xml") {
			out = append(out, fp)
		}
	case "jfinal":
		// Look for JFinalConfig or extends JFinal
		for _, fp := range idx.FindByExtension(".java") {
			content, ok := ReadFileLimited(fp, 100000)
			if !ok {
				continue
			}
			if strings.Contains(content, "JFinalConfig") || strings.Contains(content, "extends JFinal") {
				out = append(out, fp)
			}
		}
	case "play":
		// Look for routes file
		for _, fp := range idx.FindByExactName("routes") {
			out = append(out, fp)
		}
	default:
		// Generic: look for Application-like classes
		for _, fp := range idx.FindByExtension(".java") {
			base := filepath.Base(fp)
			if strings.Contains(base, "Application") || strings.Contains(base, "Main") {
				out = append(out, fp)
			}
		}
	}

	// Limit to reasonable number
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// detectModules detects sub-module directories in the project.
func detectModules(idx *FSIndex) []string {
	var out []string
	seen := map[string]bool{}
	for _, fp := range idx.AllFiles {
		rel, err := filepath.Rel(idx.Root, fp)
		if err != nil {
			continue
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) > 1 {
			mod := parts[0]
			if !seen[mod] && mod != "" && mod != "src" && mod != "lib" {
				seen[mod] = true
				out = append(out, mod)
			}
		}
	}
	return out
}

// dedupStrings removes duplicates from a string slice.
func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
