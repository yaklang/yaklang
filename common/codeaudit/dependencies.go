package codeaudit

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/catalog"
	"github.com/yaklang/yaklang/common/sca"
)

// RiskyComponent describes a known-risky component.
type RiskyComponent struct {
	Label string
	Risk  string // "high" / "medium"
	Note  string
}

// RiskyComponents is the known-risky Java component database (keyed by
// Maven groupId:artifactId). Kept exported for compatibility.
var RiskyComponents = map[string]RiskyComponent{
	"com.alibaba:fastjson":                        {Label: "fastjson", Risk: "high", Note: "historical deserialization RCE"},
	"org.apache.shiro:shiro-core":                  {Label: "shiro", Risk: "high", Note: "rememberMe / auth bypass history"},
	"commons-collections:commons-collections":      {Label: "commons-collections3", Risk: "high", Note: "deserialization gadget chain"},
	"org.apache.commons:commons-collections4":      {Label: "commons-collections4", Risk: "medium", Note: "deserialization gadget chain"},
	"com.thoughtworks.xstream:xstream":             {Label: "xstream", Risk: "high", Note: "deserialization issues"},
	"org.apache.logging.log4j:log4j-core":          {Label: "log4j", Risk: "high", Note: "Log4Shell class of issues"},
	"com.fasterxml.jackson.core:jackson-databind":  {Label: "jackson", Risk: "medium", Note: "unsafe polymorphic deserialization when misconfigured"},
	"mysql:mysql-connector-java":                   {Label: "mysql-connector", Risk: "medium", Note: "track version for known driver issues"},
	"com.mysql:mysql-connector-j":                  {Label: "mysql-connector-j", Risk: "medium", Note: "track version for known driver issues"},
}

// riskyComponentsByLanguage dispatches the risky-component tables. Non-Java
// ecosystems ship only name-only entries that are defensible at any version;
// proper version-range matching needs the CVE database bridge (future work).
var riskyComponentsByLanguage = map[string]map[string]RiskyComponent{
	"java": RiskyComponents,
	"node": {
		"node-serialize": {Label: "node-serialize", Risk: "high", Note: "unsanitized unserialize() leads to RCE"},
	},
}

// riskyTableFor returns the risky component table for a language.
func riskyTableFor(language string) map[string]RiskyComponent {
	if table, ok := riskyComponentsByLanguage[language]; ok {
		return table
	}
	return nil
}

// ScanDependencies scans dependencies via SCA and identifies risky components.
func ScanDependencies(target string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	report := NewReport("codeaudit/dependencies", root, o.Language)

	// Run SCA scan
	pkgs, err := sca.ScanLocalFilesystem(root)
	if err != nil {
		report.Status = "partial"
		report.Summary = fmt.Sprintf("SCA scan failed: %v", err)
		report.Artifacts = map[string]any{
			"dependencies":     []map[string]any{},
			"risky_components":  []map[string]any{},
			"audit_options":     o,
			"scan_root":         root,
			"error":             err.Error(),
		}
		filesScanned := countDependencyManifests(idx, o)
		return report.Finish(start, filesScanned, o)
	}

	deps := []map[string]any{}
	risky := []map[string]any{}
	seen := map[string]bool{}

	for _, pkg := range pkgs {
		key := fmt.Sprintf("%s@%s", pkg.Name, pkg.Version)
		if seen[key] {
			continue
		}
		seen[key] = true

		foundBy := ""
		if len(pkg.FromAnalyzer) > 0 {
			foundBy = pkg.FromAnalyzer[0]
		}

		dep := map[string]any{
			"name":     pkg.Name,
			"version":  pkg.Version,
			"found_by": foundBy,
		}
		deps = append(deps, dep)

		if o.RiskyMode != "off" {
			hits := matchRiskyComponent(pkg.Name, pkg.Version, o.Language)
			for _, hit := range hits {
				risky = append(risky, hit)
			}
		}
	}

	report.Artifacts = map[string]any{
		"dependencies":    deps,
		"risky_components": risky,
		"audit_options":    o,
		"scan_root":        root,
	}
	report.Summary = fmt.Sprintf("found %d dependency entries, %d risky component hit(s)", len(deps), len(risky))

	filesScanned := countDependencyManifests(idx, o)
	return report.Finish(start, filesScanned, o)
}

// countDependencyManifests counts the language's dependency manifests found
// in the index.
func countDependencyManifests(idx *FSIndex, o *ProbeOptions) int {
	n := 0
	for _, name := range catalog.LanguageProfileFor(o.Language).DepManifests {
		n += len(idx.FindByExactName(name))
	}
	return n
}

// matchRiskyComponent checks if a package matches any known risky component
// of its language. Keys are either "groupId:artifactId" (Java ecosystems) or
// plain package names (npm and friends).
func matchRiskyComponent(name, version, language string) []map[string]any {
	table := riskyTableFor(language)
	if table == nil {
		return nil
	}
	var results []map[string]any
	for key, comp := range table {
		parts := strings.Split(key, ":")
		if len(parts) == 2 {
			// Match by groupId:artifactId
			artifactID := parts[1]
			if name != key && name != artifactID && !strings.HasSuffix(name, ":"+artifactID) {
				continue
			}
		} else {
			// Plain package-name match
			if name != key {
				continue
			}
		}
		results = append(results, map[string]any{
			"label":      comp.Label,
			"risk":       comp.Risk,
			"note":       comp.Note,
			"package":    name,
			"version":    version,
			"identifier": key,
		})
	}
	return results
}
