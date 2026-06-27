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
	frameworkToolkitSelectionSchemaVersion = 1
	FrameworkToolkitIDOther                = "other"
	FrameworkToolkitModeFast               = "fast"
	FrameworkToolkitModeFallbackAI         = "fallback_ai"
	frameworkToolkitDetectThreshold        = 0.8
)

// FrameworkToolkitSelectionV1 records router or detect-fallback framework choice.
type FrameworkToolkitSelectionV1 struct {
	SchemaVersion int      `json:"schema_version"`
	GeneratedAt   string   `json:"generated_at"`
	FrameworkID   string   `json:"framework_id"`
	Confidence    float64  `json:"confidence,omitempty"`
	Rationale     string   `json:"rationale,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	Source        string   `json:"source,omitempty"` // react | detect_fallback | resume
}

// ToolkitVerifyReport summarizes programmatic bulk API verification.
type ToolkitVerifyReport struct {
	TotalRecords   int `json:"total_records"`
	Probed         int `json:"probed"`
	Verified       int `json:"verified"`
	Rejected       int `json:"rejected"`
	Skipped        int `json:"skipped"`
	DestructiveSkip int `json:"destructive_skip"`
	Errors         int `json:"errors"`
}

func normalizeFrameworkToolkitID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func persistFrameworkToolkitSelection(rt *Runtime, sel *FrameworkToolkitSelectionV1) error {
	if rt == nil || sel == nil {
		return utils.Error("nil runtime or selection")
	}
	if sel.SchemaVersion == 0 {
		sel.SchemaVersion = frameworkToolkitSelectionSchemaVersion
	}
	if sel.GeneratedAt == "" {
		sel.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	sel.FrameworkID = normalizeFrameworkToolkitID(sel.FrameworkID)
	b, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.FrameworkToolkitSelectionPath(rt.WorkDir), b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactFrameworkToolkitSelection, string(b))
	}
	if rt != nil {
		rt.SelectedFrameworkID = sel.FrameworkID
	}
	return nil
}

func loadFrameworkToolkitSelection(workDir string) (*FrameworkToolkitSelectionV1, error) {
	b, err := os.ReadFile(store.FrameworkToolkitSelectionPath(workDir))
	if err != nil {
		return nil, err
	}
	var sel FrameworkToolkitSelectionV1
	if err := json.Unmarshal(b, &sel); err != nil {
		return nil, err
	}
	return &sel, nil
}

func frameworkToolkitSelectionExists(workDir string) bool {
	_, err := os.Stat(store.FrameworkToolkitSelectionPath(workDir))
	return err == nil
}
