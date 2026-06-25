package loop_ssa_api_discovery

import (
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// BuildFeatureInventoryFromRegistry synthesizes feature_inventory from code_unit_registry http_entry units.
func BuildFeatureInventoryFromRegistry(rt *Runtime) (*FeatureInventoryV1, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	jobs, err := buildJobsFromHttpEntryRegistry(rt)
	if err != nil {
		return nil, err
	}
	inv := &FeatureInventoryV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Features:      []FeatureInventoryEntry{},
		Coverage: FeatureCoverageResult{
			Policy:        "code_unit_registry_http_entry",
			TotalRequired: len(jobs),
			Covered:       len(jobs),
			Complete:      true,
		},
	}
	for _, j := range jobs {
		fid := j.FeatureID
		if fid == "" {
			fid = "http_entry_" + normEntryFileRef(j.EntryFile)
		}
		inv.Features = append(inv.Features, FeatureInventoryEntry{
			FeatureID:       fid,
			Label:           filepath.Base(j.EntryFile),
			SurfaceKind:     SurfaceKindHTTPAPI,
			EntryFiles:      []string{j.EntryFile},
			PackagePatterns: append([]string(nil), j.PackagePatterns...),
		})
	}
	return inv, nil
}

// BackfillFeatureInventoryFromRegistry writes feature_inventory when directory BFS was skipped.
func BackfillFeatureInventoryFromRegistry(rt *Runtime) error {
	inv, err := BuildFeatureInventoryFromRegistry(rt)
	if err != nil {
		return err
	}
	if len(inv.Features) == 0 {
		log.Warnf("ssa_api_discovery: registry backfill produced 0 features")
	}
	if err := persistFeatureInventory(rt, inv); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: feature_inventory backfilled from code_unit_registry features=%d", len(inv.Features))
	return nil
}
