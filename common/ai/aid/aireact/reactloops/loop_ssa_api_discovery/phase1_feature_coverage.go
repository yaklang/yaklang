package loop_ssa_api_discovery

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// collectControllerScopeUnits returns java_package units that host HTTP controllers.
func collectControllerScopeUnits(inv *JavaBusinessScopeInventory) []JavaScopeUnit {
	if inv == nil {
		return nil
	}
	var out []JavaScopeUnit
	for _, mod := range inv.Modules {
		for _, u := range mod.ScopeUnits {
			if u.Kind != scopeUnitKindJavaPackage {
				continue
			}
			if u.DomainSegment == "controller" || strings.Contains(u.Path, "/controller/") {
				out = append(out, u)
			}
		}
	}
	return out
}

func featureInventoryAssignedPatterns(inv *FeatureInventoryV1) []string {
	if inv == nil {
		return nil
	}
	var patterns []string
	for _, f := range inv.Features {
		patterns = append(patterns, f.PackagePatterns...)
		for _, ef := range EntryFilesForFeature(f) {
			if ef = strings.TrimSpace(ef); ef != "" {
				patterns = append(patterns, ef)
			}
		}
	}
	return patterns
}

func isFQCNClassPattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.Contains(pattern, "*") || strings.Contains(pattern, "/") {
		return false
	}
	parts := strings.Split(pattern, ".")
	if len(parts) < 2 {
		return false
	}
	return isJavaTypeName(parts[len(parts)-1])
}

// featurePatternCoversScopeUnit matches filesystem scope paths against package/file patterns.
func featurePatternCoversScopeUnit(unitPath, pattern string) bool {
	unitPath = normScopePath(unitPath)
	pattern = strings.TrimSpace(pattern)
	if unitPath == "" || pattern == "" {
		return false
	}
	if isFQCNClassPattern(pattern) {
		return false
	}
	if strings.Contains(pattern, "/") {
		p := normScopePath(pattern)
		p = strings.TrimSuffix(p, "/*")
		p = strings.TrimSuffix(p, "*")
		return isScopePathDescendantOrEqual(unitPath, p)
	}
	suffix := javaPatternToPathSuffix(pattern)
	if suffix == "" {
		return false
	}
	if unitPath == suffix || strings.HasSuffix(unitPath, "/"+suffix) {
		return true
	}
	return strings.Contains(unitPath, "/"+suffix+"/") || strings.HasSuffix(unitPath, suffix)
}

func javaPatternToPathSuffix(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.TrimPrefix(pattern, "*.")
	pattern = strings.TrimSuffix(pattern, ".*")
	pattern = strings.TrimSuffix(pattern, "*")
	if pattern == "" {
		return ""
	}
	parts := strings.Split(pattern, ".")
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if isJavaTypeName(last) {
			parts = parts[:len(parts)-1]
		}
	}
	return strings.Join(parts, "/")
}

func isJavaTypeName(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

func evaluateFeatureEntryFilesCoverage(registry *CodeUnitRegistryV1, inv *FeatureInventoryV1) FeatureCoverageResult {
	res := FeatureCoverageResult{Policy: "code_unit_registry_entry_files"}
	if registry == nil || len(registry.Units) == 0 {
		return res
	}
	res.TotalRequired = len(registry.Units)
	registrySet := registryRelPathSet(registry)
	assigned, dupes, outside := collectAssignedEntryFiles(inv, registrySet)
	for rel := range assigned {
		if _, ok := registrySet[rel]; ok {
			res.Covered++
		}
	}
	for rel := range registrySet {
		if _, ok := assigned[rel]; !ok {
			res.UncoveredPaths = append(res.UncoveredPaths, rel)
		}
	}
	for rel := range dupes {
		res.UncoveredPaths = append(res.UncoveredPaths, rel+"(duplicate)")
	}
	for rel := range outside {
		res.UncoveredPaths = append(res.UncoveredPaths, rel+"(not_in_registry)")
	}
	sort.Strings(res.UncoveredPaths)
	res.Complete = res.TotalRequired > 0 &&
		res.Covered == res.TotalRequired &&
		len(dupes) == 0 &&
		len(outside) == 0 &&
		len(res.UncoveredPaths) == 0
	return res
}

func registryRelPathSet(registry *CodeUnitRegistryV1) map[string]struct{} {
	out := make(map[string]struct{}, len(registry.Units))
	for _, u := range registry.Units {
		rel := normEntryFileRef(u.RelPath)
		if rel != "" {
			out[rel] = struct{}{}
		}
	}
	return out
}

func collectAssignedEntryFiles(inv *FeatureInventoryV1, registrySet map[string]struct{}) (assigned, dupes, outside map[string]struct{}) {
	assigned = map[string]struct{}{}
	dupes = map[string]struct{}{}
	outside = map[string]struct{}{}
	if inv == nil {
		return assigned, dupes, outside
	}
	seen := map[string]struct{}{}
	for _, f := range inv.Features {
		for _, ef := range EntryFilesForFeature(f) {
			rel := normEntryFileRef(ef)
			if rel == "" {
				continue
			}
			if _, ok := registrySet[rel]; !ok {
				outside[rel] = struct{}{}
				continue
			}
			if _, ok := seen[rel]; ok {
				dupes[rel] = struct{}{}
				continue
			}
			seen[rel] = struct{}{}
			assigned[rel] = struct{}{}
		}
	}
	return assigned, dupes, outside
}

func normEntryFileRef(rel string) string {
	return filepath.ToSlash(strings.TrimSpace(rel))
}

func evaluateFeatureInventoryCoverage(rt *Runtime, inv *FeatureInventoryV1) FeatureCoverageResult {
	res := FeatureCoverageResult{Policy: "controller_java_packages"}
	javaInv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
	if javaInv == nil {
		n := len(inv.Features)
		res.Complete = n > 0
		res.TotalRequired = n
		res.Covered = n
		return res
	}
	required := collectControllerScopeUnits(javaInv)
	if len(required) == 0 {
		required = collectRequiredScopeUnits(javaInv)
		res.Policy = "all_java_packages_fallback"
	}
	res.TotalRequired = len(required)
	patterns := featureInventoryAssignedPatterns(inv)
	for _, u := range required {
		if scopeUnitCoveredByFeaturePatterns(u.Path, patterns) {
			res.Covered++
		} else {
			res.UncoveredPaths = append(res.UncoveredPaths, u.Path)
		}
	}
	res.Complete = res.TotalRequired > 0 && res.Covered >= res.TotalRequired
	return res
}

func scopeUnitCoveredByFeaturePatterns(unitPath string, patterns []string) bool {
	for _, p := range patterns {
		if featurePatternCoversScopeUnit(unitPath, p) {
			return true
		}
	}
	return false
}

func formatFeatureCoverageFeedback(res FeatureCoverageResult) string {
	if res.Complete {
		return fmt.Sprintf("entry_files coverage ok: %d/%d", res.Covered, res.TotalRequired)
	}
	msg := fmt.Sprintf("entry_files coverage incomplete: %d/%d registry units still uncovered or invalid", res.Covered, res.TotalRequired)
	if len(res.UncoveredPaths) == 0 {
		return msg
	}
	msg += "\nuncovered_paths:"
	limit := len(res.UncoveredPaths)
	if limit > 25 {
		limit = 25
	}
	for i := 0; i < limit; i++ {
		msg += "\n- " + res.UncoveredPaths[i]
	}
	if len(res.UncoveredPaths) > limit {
		msg += fmt.Sprintf("\n... +%d more", len(res.UncoveredPaths)-limit)
	}
	msg += "\nHint: assign every code_unit_registry rel_path exactly once via entry_files; set surface_kind=http_api|code_only"
	return msg
}
