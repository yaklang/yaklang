package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Phase1ReconOutput is persisted to phase1_recon.json (legacy worklist seed for staged reading helpers).
type Phase1ReconOutput struct {
	FilesScanned          []string           `json:"files_scanned"`
	EndpointsExtracted    int                `json:"endpoints_extracted"`
	DependenciesExtracted int                `json:"dependencies_extracted"`
	RoutingFacts          []RoutingFact      `json:"routing_facts,omitempty"`
	NextWorklist          []WorklistSeedItem `json:"next_worklist"`
	Summary               string             `json:"summary,omitempty"`
}

func persistPhase1ReconOutput(rt *Runtime, out *Phase1ReconOutput) error {
	if rt == nil || out == nil {
		return utils.Error("nil recon output")
	}
	out.NextWorklist = sanitizeReconWorklist(rt, out.NextWorklist)
	if len(out.NextWorklist) == 0 {
		return utils.Error("recon worklist empty after removing glob paths")
	}
	path := store.Phase1ReconPath(rt.WorkDir)
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactPhase1Recon, string(b))
	}
	log.Infof("ssa_api_discovery: phase1_recon worklist=%d endpoints_reported=%d",
		len(out.NextWorklist), out.EndpointsExtracted)
	return nil
}

func loadPhase1ReconOutput(workDir string) (*Phase1ReconOutput, error) {
	b, err := os.ReadFile(store.Phase1ReconPath(workDir))
	if err != nil {
		return nil, err
	}
	var out Phase1ReconOutput
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// runPhase1ReconProgrammaticFallback seeds worklist from backend_scope (used by tests and legacy recovery).
func runPhase1ReconProgrammaticFallback(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	out := &Phase1ReconOutput{
		FilesScanned:       []string{},
		NextWorklist:       expandReconWorklistFromScope(rt),
		Summary:            "programmatic fallback from backend_scope",
		EndpointsExtracted: 0,
	}
	return persistPhase1ReconOutput(rt, out)
}

func isGlobWorklistPath(rel string) bool {
	rel = strings.TrimSpace(rel)
	return strings.Contains(rel, "*") || strings.Contains(rel, "?")
}

func sanitizeReconWorklist(rt *Runtime, items []WorklistSeedItem) []WorklistSeedItem {
	seen := map[string]struct{}{}
	var out []WorklistSeedItem
	add := func(item WorklistSeedItem) {
		rel := filepath.ToSlash(strings.TrimSpace(item.RelPath))
		if rel == "" || isGlobWorklistPath(rel) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		item.RelPath = rel
		out = append(out, item)
	}
	for _, item := range items {
		add(item)
	}
	if len(out) > 0 {
		return out
	}
	return expandReconWorklistFromScope(rt)
}

func expandReconWorklistFromScope(rt *Runtime) []WorklistSeedItem {
	if rt == nil {
		return nil
	}
	var seed []WorklistSeedItem
	seen := map[string]struct{}{}
	appendItem := func(item WorklistSeedItem) {
		rel := filepath.ToSlash(strings.TrimSpace(item.RelPath))
		if rel == "" || isGlobWorklistPath(rel) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		item.RelPath = rel
		seed = append(seed, item)
	}
	scope, _ := loadBackendScope(rt.WorkDir)
	if scope != nil {
		for _, c := range scope.ControllerFileCandidates {
			cat := worklistCategoryAPIHandler
			pri := 3
			if isAuthEntryPath(c.RelPath) || strings.Contains(strings.ToLower(c.RelPath), "login") {
				cat = worklistCategoryAuthEntry
				pri = 2
			}
			appendItem(WorklistSeedItem{RelPath: c.RelPath, Reason: c.Reason, Category: cat, Priority: pri})
		}
	}
	if rt.Session != nil && rt.Session.CodePathOK {
		configPaths, _ := searchFilesUnderCodeRoot(rt.Session.CodeRootPath, fileSearchOpts{
			Suffix: ".java", NameContains: "config", MaxResults: 30,
		})
		for _, p := range configPaths {
			lower := strings.ToLower(p)
			if strings.Contains(lower, "adminconfig") || strings.Contains(lower, "apiconfig") ||
				strings.Contains(lower, "webmvcconfigurer") || strings.HasSuffix(lower, "webconfig.java") {
				appendItem(WorklistSeedItem{RelPath: p, Reason: "WebMvcConfigurer routing config", Category: worklistCategoryRoutingConfig, Priority: 1})
			}
		}
	}
	return seed
}
