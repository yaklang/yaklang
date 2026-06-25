package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	frontendAPISchemaVersion  = 1
	SourceFrontendHarvest     = "frontend_harvest"
	frontendHarvestProvenance   = "frontend_regex_harvest"
)

type FrontendAPIParam struct {
	Name     string `json:"name"`
	Location string `json:"location,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type FrontendAPICall struct {
	CallID            string             `json:"call_id"`
	Method            string             `json:"method"`
	PathRaw           string             `json:"path_raw"`
	PathResolved      string             `json:"path_resolved,omitempty"`
	SourceFile        string             `json:"source_file"`
	LineHint          int                `json:"line_hint,omitempty"`
	ClientLib         string             `json:"client_lib,omitempty"`
	AuthRealmHint     string             `json:"auth_realm_hint,omitempty"`
	Params            []FrontendAPIParam `json:"params,omitempty"`
	LinkedHandlerHint string             `json:"linked_handler_hint,omitempty"`
	Confidence        string             `json:"confidence,omitempty"`
}

type FrontendAPIClientModule struct {
	File     string `json:"file"`
	Export   string `json:"export,omitempty"`
	BasePath string `json:"base_path,omitempty"`
}

type FrontendAPIHarvestCandidate struct {
	FileRelPath string `json:"file_rel_path"`
	HitCount    int    `json:"hit_count"`
	Priority    int    `json:"priority,omitempty"`
}

type FrontendAPIHarvestReport struct {
	SchemaVersion   int                           `json:"schema_version"`
	GeneratedAt     time.Time                     `json:"generated_at"`
	FrontendRoots   []string                      `json:"frontend_roots,omitempty"`
	BaseURLPatterns []string                      `json:"base_url_patterns,omitempty"`
	Calls           []FrontendAPICall             `json:"calls"`
	Candidates      []FrontendAPIHarvestCandidate `json:"harvest_candidates,omitempty"`
	Stats           FrontendAPIStats              `json:"stats"`
	FullPath        string                        `json:"full_report_path,omitempty"`
	Warnings        []string                      `json:"warnings,omitempty"`
}

type FrontendAPIInventory struct {
	SchemaVersion    int                       `json:"schema_version"`
	GeneratedAt      string                    `json:"generated_at"`
	FrontendRoots    []string                  `json:"frontend_roots,omitempty"`
	APIClientModules []FrontendAPIClientModule `json:"api_client_modules,omitempty"`
	Calls            []FrontendAPICall         `json:"calls"`
	Stats            FrontendAPIStats          `json:"stats"`
	FullPath         string                    `json:"full_report_path,omitempty"`
	BootstrapNote    string                    `json:"bootstrap_note,omitempty"`
}

type FrontendAPIStats struct {
	Calls        int `json:"calls"`
	FilesScanned int `json:"files_scanned"`
}

func persistFrontendAPIHarvest(rt *Runtime, rep *FrontendAPIHarvestReport) error {
	if rt == nil || rep == nil {
		return utils.Error("nil harvest report")
	}
	rep.SchemaVersion = frontendAPISchemaVersion
	rep.GeneratedAt = time.Now().UTC()
	rep.FullPath = store.FrontendAPIHarvestPath(rt.WorkDir)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(rep.FullPath, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactFrontendAPIHarvest, string(b))
	}
	return nil
}

func loadFrontendAPIHarvest(workDir string) (*FrontendAPIHarvestReport, error) {
	b, err := os.ReadFile(store.FrontendAPIHarvestPath(workDir))
	if err != nil {
		return nil, err
	}
	var rep FrontendAPIHarvestReport
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

func persistFrontendAPIInventory(rt *Runtime, inv *FrontendAPIInventory) error {
	if rt == nil || inv == nil {
		return utils.Error("nil inventory")
	}
	inv.SchemaVersion = frontendAPISchemaVersion
	if strings.TrimSpace(inv.GeneratedAt) == "" {
		inv.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	inv.FullPath = store.FrontendAPIInventoryPath(rt.WorkDir)
	inv.Stats.Calls = len(inv.Calls)
	b, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(inv.FullPath, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactFrontendAPIInventory, string(b))
	}
	return nil
}

func loadFrontendAPIInventory(workDir string) (*FrontendAPIInventory, error) {
	b, err := os.ReadFile(store.FrontendAPIInventoryPath(workDir))
	if err != nil {
		return nil, err
	}
	var inv FrontendAPIInventory
	if err := json.Unmarshal(b, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func frontendCallsToStaticHints(calls []FrontendAPICall) []StaticRouteHint {
	out := make([]StaticRouteHint, 0, len(calls))
	for _, c := range calls {
		path := strings.TrimSpace(c.PathResolved)
		if path == "" {
			path = normURLPath(c.PathRaw)
		}
		if path == "" || isWildcardRoutePattern(path) {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(c.Method))
		if method == "" {
			method = "GET"
		}
		out = append(out, StaticRouteHint{
			Method:       method,
			PathPattern:  path,
			HandlerClass: strings.TrimSpace(c.LinkedHandlerHint),
			FileRelPath:  c.SourceFile,
			Source:       SourceFrontendHarvest,
		})
	}
	return dedupeStaticHints(out)
}

func mergeFrontendHarvestIntoStaticHints(workDir string, calls []FrontendAPICall) error {
	hints := frontendCallsToStaticHints(calls)
	if len(hints) == 0 {
		return nil
	}
	rep, err := readStaticRouteHintsReport(workDir)
	if err != nil {
		rep = &StaticRouteHintsReport{
			GeneratedAt: time.Now().UTC(),
			Hints:       []StaticRouteHint{},
			FullPath:    store.StaticRouteHintsPath(workDir),
		}
	}
	rep.Hints = append(rep.Hints, hints...)
	rep.Hints = dedupeStaticHints(rep.Hints)
	rep.Count = len(rep.Hints)
	found := false
	for _, s := range rep.SourcesRun {
		if s == SourceFrontendHarvest {
			found = true
			break
		}
	}
	if !found {
		rep.SourcesRun = append(rep.SourcesRun, SourceFrontendHarvest)
	}
	return writeStaticRouteHintsReport(workDir, rep)
}
