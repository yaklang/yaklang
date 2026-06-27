package loop_ssa_api_discovery

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedPhase1UnitGateFixtures writes registry, inventory, work progress, feature_api_map,
// and coverage_signal_decision for gate tests.
func seedPhase1UnitGateFixtures(t *testing.T, rt *Runtime, entryFile string, apis []FeatureApiEntry) {
	t.Helper()
	if rt == nil {
		t.Fatal("nil runtime")
	}
	reg := &CodeUnitRegistryV1{
		SchemaVersion: 1,
		Units:         []CodeUnitEntry{{RelPath: entryFile}},
	}
	require.NoError(t, persistCodeUnitRegistry(rt, reg))
	inv := &FeatureInventoryV1{
		SchemaVersion: 1,
		Features: []FeatureInventoryEntry{{
			FeatureID:       "api",
			Label:           "API",
			SurfaceKind:     SurfaceKindHTTPAPI,
			PackagePatterns: []string{"*.controller.*"},
			EntryFiles:      []string{entryFile},
		}},
	}
	inv.Coverage = evaluateFeatureEntryFilesCoverage(reg, inv)
	require.NoError(t, persistFeatureInventory(rt, inv))
	require.NoError(t, saveFeatureWorkProgress(rt.WorkDir, featureWorkProgress{
		Entries: []featureWorkProgressEntry{{
			EntryFile: entryFile,
			JobKind:   SurfaceKindHTTPAPI,
			Status:    featureWorkStatusDone,
		}},
	}))
	require.NoError(t, persistFeatureApiMap(rt, &FeatureApiMapV1{
		SchemaVersion: 1,
		Features: []FeatureApiMapEntry{{
			FeatureID: "api",
			Processed: true,
			Apis:      apis,
			ApiCount:  len(apis),
		}},
	}))

	// Seed coverage_signal_decision so verifyCoverageSignalVerdict does not block the gate.
	if rt.Repo != nil && rt.Session != nil {
		decision := &CoverageSignalDecision{
			Verdict:    "continue",
			Reasoning:  "test: coverage signal seeded",
			SignalJSON: "{}",
		}
		b, _ := json.MarshalIndent(decision, "", "  ")
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, "coverage_signal_decision", string(b))
	}
}
