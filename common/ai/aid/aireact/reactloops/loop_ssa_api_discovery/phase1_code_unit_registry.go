package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	codeUnitRegistrySchemaVersion = 1
	codeUnitKindHintHTTPEntry     = "http_entry"
	codeUnitKindHintService       = "service"
	codeUnitKindHintConfig        = "config"
	codeUnitKindHintOther         = "other"
)

var reJavaPackageLine = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)

// CodeUnitRegistryV1 lists every assignable source file for F1 entry_files coverage.
type CodeUnitRegistryV1 struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   string         `json:"generated_at"`
	Language      string         `json:"language,omitempty"`
	CodeRoot      string         `json:"code_root"`
	Units         []CodeUnitEntry `json:"units"`
}

// CodeUnitEntry is one analyzable source file in the registry.
type CodeUnitEntry struct {
	RelPath       string   `json:"rel_path"`
	KindHint      string   `json:"kind_hint"`
	PackagePath   string   `json:"package_path,omitempty"`
	DomainSegment string   `json:"domain_segment,omitempty"`
	ClassNames    []string `json:"class_names,omitempty"`
}

// BuildCodeUnitRegistry scans backend source trees and writes code_unit_registry.json.
//
// IMPORTANT: If a filtered registry already exists (written by FilterCodeUnitRegistryByEndpoints
// in Stage 2.5), this function skips the full rebuild to preserve the filtered version.
// The filtered registry is much smaller and contains only endpoint-related files, which
// prevents context-window explosions in downstream ReAct loops (F1 FeatureInventory, CoverageSignal).
// For projects with thousands of Java files (e.g. PublicCMS), the full registry can be 300KB+
// while the filtered version is typically <30KB.
func BuildCodeUnitRegistry(rt *Runtime) (*CodeUnitRegistryV1, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	if !rt.Session.CodePathOK {
		return nil, utils.Error("code_path not ok")
	}
	// Guard: if a filtered registry already exists, skip full rebuild.
	// This prevents the filtered registry (written by FilterCodeUnitRegistryByEndpoints)
	// from being overwritten by the full unfiltered scan.
	if existingReg, err := loadCodeUnitRegistry(rt.WorkDir); err == nil && existingReg != nil {
		if len(existingReg.Units) > 0 && isFilteredRegistry(existingReg) {
			log.Infof("ssa_api_discovery: code_unit_registry already filtered (%d units); skipping full rebuild",
				len(existingReg.Units))
			return existingReg, nil
		}
	}
	codeRoot := filepath.Clean(rt.Session.CodeRootPath)
	lang := strings.ToLower(strings.TrimSpace(rt.Session.Language))
	if lang == "" {
		lang = "java"
	}
	reg := &CodeUnitRegistryV1{
		SchemaVersion: codeUnitRegistrySchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Language:      lang,
		CodeRoot:      codeRoot,
	}
	roots := []string{"."}
	if scope, err := loadBackendScope(rt.WorkDir); err == nil && scope != nil {
		for _, r := range scope.BackendRoots {
			r = strings.TrimSpace(filepath.ToSlash(r))
			if r != "" {
				roots = append(roots, r)
			}
		}
	}
	seen := map[string]struct{}{}
	for _, rootRel := range dedupeStrings(roots) {
		abs := codeRoot
		if rootRel != "." && rootRel != "" {
			abs = filepath.Join(codeRoot, filepath.FromSlash(rootRel))
		}
		units, err := scanCodeUnitsUnderRoot(codeRoot, abs, lang)
		if err != nil {
			log.Warnf("ssa_api_discovery: code_unit_registry scan %s: %v", rootRel, err)
			continue
		}
		for _, u := range units {
			if _, ok := seen[u.RelPath]; ok {
				continue
			}
			seen[u.RelPath] = struct{}{}
			reg.Units = append(reg.Units, u)
		}
	}
	if err := persistCodeUnitRegistry(rt, reg); err != nil {
		return reg, err
	}
	log.Infof("ssa_api_discovery: code_unit_registry units=%d", len(reg.Units))
	return reg, nil
}

func scanCodeUnitsUnderRoot(codeRoot, scanAbs, lang string) ([]CodeUnitEntry, error) {
	codeRoot = filepath.Clean(codeRoot)
	scanAbs = filepath.Clean(scanAbs)
	if _, err := os.Stat(scanAbs); err != nil {
		return nil, err
	}
	var out []CodeUnitEntry
	switch lang {
	case "java", "":
		javaRoot := filepath.Join(scanAbs, "src", "main", "java")
		if _, err := os.Stat(javaRoot); err != nil {
			javaRoot = scanAbs
		}
		err := filepath.Walk(javaRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".java") {
				return nil
			}
			rel, err := filepath.Rel(codeRoot, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if skipJavaSourceRelPath(rel) {
				return nil
			}
			if strings.Contains(strings.ToLower(rel), "/resources/generator/") {
				return nil
			}
			entry := CodeUnitEntry{
				RelPath:    rel,
				KindHint:   inferCodeUnitKindHint(rel),
				ClassNames: []string{strings.TrimSuffix(filepath.Base(path), ".java")},
			}
			if b, readErr := os.ReadFile(path); readErr == nil {
				if m := reJavaPackageLine.FindStringSubmatch(string(b)); len(m) > 1 {
					entry.PackagePath = strings.ReplaceAll(m[1], ".", "/")
				}
			}
			entry.DomainSegment = domainSegmentFromPath(rel)
			out = append(out, entry)
			return nil
		})
		return out, err
	default:
		return nil, nil
	}
}

func inferCodeUnitKindHint(rel string) string {
	lower := strings.ToLower(filepath.ToSlash(rel))
	base := strings.ToLower(filepath.Base(lower))
	switch {
	case strings.Contains(lower, "/controller/") || strings.HasSuffix(base, "controller.java"):
		return codeUnitKindHintHTTPEntry
	case strings.Contains(lower, "/config/") || strings.HasSuffix(base, "config.java") || strings.Contains(base, "configuration"):
		return codeUnitKindHintConfig
	case strings.Contains(lower, "/service/") || strings.HasSuffix(base, "service.java"):
		return codeUnitKindHintService
	default:
		return codeUnitKindHintOther
	}
}

func domainSegmentFromPath(rel string) string {
	lower := strings.ToLower(filepath.ToSlash(rel))
	for _, seg := range []string{"controller", "service", "config", "repository", "mapper", "handler"} {
		if strings.Contains(lower, "/"+seg+"/") {
			return seg
		}
	}
	return ""
}

func persistCodeUnitRegistry(rt *Runtime, reg *CodeUnitRegistryV1) error {
	if reg == nil {
		return utils.Error("nil registry")
	}
	if err := writeArtifactJSON(store.CodeUnitRegistryPath(rt.WorkDir), reg); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(reg, "", "  ")
	persistPhaseArtifact(rt, store.ArtifactCodeUnitRegistry, string(b))
	return nil
}

func loadCodeUnitRegistry(workDir string) (*CodeUnitRegistryV1, error) {
	var reg CodeUnitRegistryV1
	return loadJSONArtifact(store.CodeUnitRegistryPath(workDir), &reg)
}

// isFilteredRegistry returns true if reg appears to be a filtered registry produced
// by FilterCodeUnitRegistryByEndpoints rather than a full unfiltered scan.
//
// Detection heuristic: the filtered registry keeps only endpoint-related files
// (controllers, services, configs) and is typically much smaller than the full
// scan. We use a conservative threshold: if the unit count is below 500, we
// consider it filtered. Large projects with thousands of files (e.g. PublicCMS
// with ~3000 Java files) will have a full registry far above this threshold,
// while the filtered version typically has <100 entries.
//
// Additionally, the filtered registry's Units are a subset derived from a
// previous unfiltered scan, so if len(Units) == 0 we fall back to assuming filtered
// to preserve the existing behavior.
func isFilteredRegistry(reg *CodeUnitRegistryV1) bool {
	if reg == nil {
		return false
	}
	// Conservative heuristic: filtered registries are small subsets.
	// Full scans of large projects easily exceed 500 units.
	const filteredRegistrySizeThreshold = 500
	if len(reg.Units) > filteredRegistrySizeThreshold {
		return false
	}
	return true
}

// FilterCodeUnitRegistryByEndpoints filters the code unit registry to only include
// files that are related to API endpoints. This significantly reduces the workload
// for downstream analysis by focusing on controller/service/entity code.
//
// Strategy:
// 1. Load unified_endpoints.json to get endpoint file paths
// 2. Load code_unit_registry.json to get all code units
// 3. Filter to keep only:
//    - Files that appear in unified_endpoints (direct match)
//    - Files in the same directory as endpoints (related files like services/entities)
//    - Config and initializer files
func FilterCodeUnitRegistryByEndpoints(rt *Runtime) (*CodeUnitRegistryV1, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}

	// Load unified endpoints
	inventory, err := loadUnifiedEndpointsInventory(rt.WorkDir)
	if err != nil {
		return nil, err
	}

	if len(inventory.Endpoints) == 0 {
		log.Warnf("ssa_api_discovery: no endpoints found, skipping filter")
		return nil, nil
	}

	// Load code unit registry
	reg, err := loadCodeUnitRegistry(rt.WorkDir)
	if err != nil {
		return nil, err
	}

	// Build set of endpoint file paths
	endpointFiles := make(map[string]bool)
	endpointDirs := make(map[string]bool)
	for _, ep := range inventory.Endpoints {
		if ep.FilePath != "" {
			endpointFiles[ep.FilePath] = true
			// Extract directory
			dir := filepath.Dir(ep.FilePath)
			if dir != "." && dir != "" {
				endpointDirs[dir] = true
			}
		}
	}

	// Filter code units
	var filtered []CodeUnitEntry
	for _, unit := range reg.Units {
		keep := false

		// Keep if directly matched with endpoint file
		if endpointFiles[unit.RelPath] {
			keep = true
		}

		// Keep if in endpoint directory
		if !keep {
			unitDir := filepath.Dir(unit.RelPath)
			if endpointDirs[unitDir] {
				keep = true
			}
		}

		// Keep config files (they're lightweight and important)
		if !keep && unit.KindHint == codeUnitKindHintConfig {
			keep = true
		}

		// Keep files in certain critical packages
		if !keep {
			for _, criticalPath := range []string{"initializer", "filter", "interceptor", "handler"} {
				if strings.Contains(unit.RelPath, criticalPath) {
					keep = true
					break
				}
			}
		}

		if keep {
			// Mark http_entry kind hint for endpoint files
			if endpointFiles[unit.RelPath] {
				unit.KindHint = codeUnitKindHintHTTPEntry
			}
			filtered = append(filtered, unit)
		}
	}

	// Create filtered registry
	filteredReg := &CodeUnitRegistryV1{
		SchemaVersion: reg.SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Language:      reg.Language,
		CodeRoot:      reg.CodeRoot,
		Units:         filtered,
	}

	// Persist filtered registry
	if err := persistCodeUnitRegistry(rt, filteredReg); err != nil {
		return filteredReg, err
	}

	log.Infof("ssa_api_discovery: code_unit_registry filtered: %d -> %d (endpoint files: %d)",
		len(reg.Units), len(filtered), len(endpointFiles))

	return filteredReg, nil
}
