package codeaudit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/catalog"
)

// ProbeProject probes the project build system, frameworks, and CMS products.
func ProbeProject(target string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	buildSystem := detectBuildSystem(idx)
	frameworks := detectFrameworks(idx, o)
	cmsProducts := detectCmsProducts(idx, o)
	recommended := recommendTools(frameworks, cmsProducts, o)

	report := NewReport("codeaudit/probe", root, o.Language)
	report.Artifacts = map[string]any{
		"build_system":          buildSystem,
		"detected_frameworks":   frameworks,
		"detected_cms_products": cmsProducts,
		"recommended_tools":     recommended,
		"scan_root":             root,
		"audit_options":         o,
	}

	parts := []string{}
	parts = append(parts, fmt.Sprintf("build system: %s", buildSystem))
	if len(frameworks) > 0 {
		names := []string{}
		for _, f := range frameworks {
			names = append(names, f.Display)
		}
		parts = append(parts, fmt.Sprintf("frameworks: %s", strings.Join(names, ", ")))
	}
	if len(cmsProducts) > 0 {
		names := []string{}
		for _, c := range cmsProducts {
			names = append(names, c.Display)
		}
		parts = append(parts, fmt.Sprintf("cms: %s", strings.Join(names, ", ")))
	}
	parts = append(parts, fmt.Sprintf("recommended tools: %d", len(recommended)))
	report.Summary = strings.Join(parts, "; ")

	return report.Finish(start, idx.FilesScanned, o)
}

// resolveScanRoot resolves the actual scan root, handling monorepo expansion.
func resolveScanRoot(target string, opts *ProbeOptions) string {
	abs, err := filepath.Abs(target)
	if err != nil {
		return target
	}
	// If scope-modules are specified and target looks like a submodule,
	// try to find the monorepo root by looking for sibling modules.
	if opts.ResolveMonorepoRoot && len(opts.ScopeModules) > 0 {
		parent := filepath.Dir(abs)
		if _, err := os.Stat(parent); err == nil {
			// Check if parent contains sibling module dirs
			entries, err := os.ReadDir(parent)
			if err == nil {
				foundCount := 0
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					for _, mod := range opts.ScopeModules {
						if e.Name() == mod {
							foundCount++
							break
						}
					}
				}
				if foundCount >= 2 {
					return parent
				}
			}
		}
	}
	return abs
}

// detectBuildSystem detects the build system from file presence.
func detectBuildSystem(idx *FSIndex) string {
	hasPom := len(idx.FindByExactName("pom.xml")) > 0
	hasGradle := len(idx.FindByExactName("build.gradle")) > 0 || len(idx.FindByExactName("build.gradle.kts")) > 0
	switch {
	case hasPom && hasGradle:
		return "mixed"
	case hasPom:
		return "maven"
	case hasGradle:
		return "gradle"
	default:
		return "unknown"
	}
}

// detectFrameworks detects frameworks based on file and content markers.
func detectFrameworks(idx *FSIndex, o *ProbeOptions) []FrameworkDetection {
	threshold := o.MinConfidence
	if threshold <= 0 {
		threshold = catalog.GetModeThreshold(o.DetectionMode)
	}

	var includeSet map[string]bool
	if len(o.IncludeFrameworks) > 0 {
		includeSet = map[string]bool{}
		for _, f := range o.IncludeFrameworks {
			includeSet[f] = true
		}
	}
	excludeSet := map[string]bool{}
	for _, f := range o.ExcludeFrameworks {
		excludeSet[f] = true
	}

	var out []FrameworkDetection
	for _, sig := range catalog.JavaFrameworkCatalog {
		if excludeSet[sig.Name] {
			continue
		}

		confidence := 0.0

		// Check file markers
		fileHits := 0
		for _, marker := range sig.FileMarkers {
			if len(idx.FindByName(marker)) > 0 {
				fileHits++
			}
		}
		if fileHits > 0 {
			confidence += 0.35
		}

		// Check content markers in relevant files
		contentHit := false
		strongHit := false
		filesToCheck := getFilesForContentCheck(idx, sig)
		for _, fp := range filesToCheck {
			content, ok := ReadFileLimited(fp, 1000000)
			if !ok {
				continue
			}
			for _, marker := range sig.ContentMarkers {
				if strings.Contains(content, marker) {
					contentHit = true
					break
				}
			}
			if contentHit {
				for _, marker := range sig.StrongContentMarkers {
					if strings.Contains(content, marker) {
						strongHit = true
						break
					}
				}
				break
			}
		}
		if contentHit {
			confidence += 0.25
		}
		if strongHit {
			confidence += 0.15
		}

		if includeSet != nil && !includeSet[sig.Name] && confidence < 1.0 {
			continue
		}

		if confidence >= threshold {
			out = append(out, FrameworkDetection{
				Name:       sig.Name,
				Display:    sig.Display,
				Confidence: roundTo2(confidence),
			})
		}
	}

	return out
}

// getFilesForContentCheck returns files relevant to content marker checking.
func getFilesForContentCheck(idx *FSIndex, sig catalog.FrameworkSignal) []string {
	// Look at pom.xml, build.gradle, and config files
	var files []string
	for _, marker := range sig.FileMarkers {
		files = append(files, idx.FindByName(marker)...)
	}
	// Also check Java source files (limited)
	if len(files) == 0 {
		files = append(files, idx.FindByExtension(".java")...)
		// Limit to first 20 java files for performance
		if len(files) > 20 {
			files = files[:20]
		}
	}
	return files
}

// detectCmsProducts detects CMS products based on fingerprints.
func detectCmsProducts(idx *FSIndex, o *ProbeOptions) []CmsDetection {
	threshold := o.CmsMinConfidence
	if threshold <= 0 {
		threshold = catalog.GetModeThreshold(o.DetectionMode)
	}

	forcedSet := map[string]bool{}
	for _, id := range o.CmsProducts {
		if id != "" {
			forcedSet[id] = true
		}
	}

	var out []CmsDetection
	for _, fp := range catalog.JavaCmsCatalog {
		if len(forcedSet) > 0 && !forcedSet[fp.ID] {
			// Only check forced products if any are specified
			continue
		}

		confidence := 0.0

		// Check file markers (directory or file presence)
		fileHits := 0
		for _, marker := range fp.FileMarkers {
			for _, path := range idx.AllFiles {
				if strings.Contains(path, marker) {
					fileHits++
					break
				}
			}
		}
		if fileHits > 0 {
			confidence += 0.35
		}

		// Check content markers (regex)
		// Scan pom.xml, build.gradle, and known config files
		checkFiles := []string{}
		checkFiles = append(checkFiles, idx.FindByExactName("pom.xml")...)
		checkFiles = append(checkFiles, idx.FindByExactName("build.gradle")...)
		checkFiles = append(checkFiles, idx.FindByName("application")...)
		contentHit := false
		for _, cf := range checkFiles {
			content, ok := ReadFileLimited(cf, 1000000)
			if !ok {
				continue
			}
			for _, marker := range fp.ContentMarkers {
				re := regexpCompileCached(marker)
				if re == nil {
					// Fall back to plain string match
					if strings.Contains(content, marker) {
						contentHit = true
						break
					}
					continue
				}
				if re.MatchString(content) {
					contentHit = true
					break
				}
			}
			if contentHit {
				break
			}
		}
		if contentHit {
			confidence += 0.35
		}

		minScore := fp.MinScore
		if minScore <= 0 {
			minScore = threshold
		}

		if confidence >= minScore {
			out = append(out, CmsDetection{
				ID:         fp.ID,
				Display:    fp.Display,
				Family:     fp.Family,
				Confidence: roundTo2(confidence),
			})
		}
	}

	return out
}

// recommendTools generates a list of recommended tools based on detected frameworks and CMS.
func recommendTools(frameworks []FrameworkDetection, cmsProducts []CmsDetection, o *ProbeOptions) []string {
	tools := []string{
		"java_maven_gradle_dependencies",
		"java_hardcoded_secrets_scan",
	}

	// Add CMS audit tool if CMS detected
	if len(cmsProducts) > 0 {
		tools = append(tools, "java_cms_product_audit")
	}

	// Add framework-specific tools
	seen := map[string]bool{}
	for _, fw := range frameworks {
		for _, sig := range catalog.JavaFrameworkCatalog {
			if sig.Name == fw.Name {
				if sig.ArchTool != "" && !seen[sig.ArchTool] {
					tools = append(tools, sig.ArchTool)
					seen[sig.ArchTool] = true
				}
				if sig.ConfigTool != "" && !seen[sig.ConfigTool] {
					tools = append(tools, sig.ConfigTool)
					seen[sig.ConfigTool] = true
				}
				break
			}
		}
	}

	return tools
}

// roundTo2 rounds a float64 to 2 decimal places.
func roundTo2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
