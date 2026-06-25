package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// IngestPhaseArtifactFromPath reads a JSON file and upserts into phase_artifacts.
func IngestPhaseArtifactFromPath(rt *Runtime, kind, path string) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return rt.Repo.UpsertPhaseArtifact(rt.Session.ID, kind, string(b))
}

// GetPhaseArtifactPayload returns JSON payload from DB, falling back to file path.
func GetPhaseArtifactPayload(rt *Runtime, kind, fallbackPath string) (string, error) {
	if rt != nil && rt.Repo != nil && rt.Session != nil {
		if row, err := rt.Repo.GetPhaseArtifact(rt.Session.ID, kind); err == nil && row != nil {
			if payload := stringsTrim(row.PayloadJSON); payload != "" {
				return payload, nil
			}
		}
	}
	if fallbackPath != "" {
		b, err := os.ReadFile(fallbackPath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", utils.Errorf("phase artifact %q not found", kind)
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == ' ' || c == '\n' || c == '\t' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// ExportVulnChecklistJSON writes checklist rows from DB to vuln_checklist.json.
func ExportVulnChecklistJSON(rt *Runtime) (string, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return "", utils.Error("nil runtime")
	}
	rows, err := rt.Repo.ListVulnChecklistItems(rt.Session.ID)
	if err != nil {
		return "", err
	}
	items := storeChecklistToDTO(rows)
	path := store.VulnChecklistPath(rt.WorkDir)
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeJSONFile(path, b); err != nil {
		return "", err
	}
	return path, nil
}

// ExportSessionArtifacts refreshes mirror JSON files from DB artifacts and structured tables.
func ExportSessionArtifacts(rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	sid := rt.Session.ID
	kinds, err := rt.Repo.ListPhaseArtifactKinds(sid)
	if err != nil {
		return err
	}
	pathForKind := map[string]string{
		store.ArtifactStaticRouteHints:   store.StaticRouteHintsPath(rt.WorkDir),
		store.ArtifactAuthSurface:        store.AuthSurfacePath(rt.WorkDir),
		store.ArtifactDependencies:       store.DependenciesInventoryPath(rt.WorkDir),
		store.ArtifactPhase1PrepBundle:   store.Phase1PrepBundlePath(rt.WorkDir),
		store.ArtifactCodeReadingPlan:          store.CodeReadingPlanPath(rt.WorkDir),
		store.ArtifactCodeReadingPlanBuildMeta: store.CodeReadingPlanBuildMetaPath(rt.WorkDir),
		store.ArtifactBackendScope:             store.BackendScopePath(rt.WorkDir),
		store.ArtifactForwardingProfile:  store.ForwardingProfilePath(rt.WorkDir),
		store.ArtifactApiPreanalysisFull: store.ApiPreanalysisReportPath(rt.WorkDir),
		store.ArtifactSyntaxflowSummary:  store.SyntaxflowSummaryPath(rt.WorkDir),
	}
	for _, kind := range kinds {
		dest, ok := pathForKind[kind]
		if !ok {
			continue
		}
		row, err := rt.Repo.GetPhaseArtifact(sid, kind)
		if err != nil || row == nil {
			continue
		}
		if err := writeJSONFile(dest, []byte(row.PayloadJSON)); err != nil {
			return err
		}
	}
	if _, err := ExportVulnChecklistJSON(rt); err != nil {
		return err
	}
	if _, err := writeRouteCandidatesFromDB(rt); err != nil {
		return err
	}
	return nil
}

// LoadCodeReadingPlanForRuntime loads code reading plan from DB artifact, falling back to JSON file.
func LoadCodeReadingPlanForRuntime(rt *Runtime) (*CodeReadingPlan, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	if rt.Repo != nil && rt.Session != nil {
		if payload, err := GetPhaseArtifactPayload(rt, store.ArtifactCodeReadingPlan, store.CodeReadingPlanPath(rt.WorkDir)); err == nil {
			var plan CodeReadingPlan
			if json.Unmarshal([]byte(payload), &plan) == nil {
				return &plan, nil
			}
		}
	}
	return LoadCodeReadingPlan(rt.WorkDir)
}
