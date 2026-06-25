package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestCollectFeatureWorkJobs_MixedSurfaceKinds(t *testing.T) {
	dir := t.TempDir()
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{
				FeatureID: "api", SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles: []string{"mod/FooController.java"},
			},
			{
				FeatureID: "svc", SurfaceKind: SurfaceKindCodeOnly,
				EntryFiles: []string{"mod/BarService.java"}, NoHttpReason: "no HTTP",
			},
		},
	}
	rt := &Runtime{WorkDir: dir, Session: &store.DiscoverySession{UUID: uuid.NewString()}}
	jobs, err := collectFeatureWorkJobs(rt, inv)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	kinds := map[string]string{}
	for _, j := range jobs {
		kinds[j.EntryFile] = j.SurfaceKind
	}
	require.Equal(t, SurfaceKindHTTPAPI, kinds["mod/FooController.java"])
	require.Equal(t, SurfaceKindCodeOnly, kinds["mod/BarService.java"])
}

func TestValidateCodeAnalysisUnitResult(t *testing.T) {
	job := FeatureWorkJob{EntryFile: "mod/BarService.java", FeatureID: "svc"}
	require.NoError(t, validateCodeAnalysisUnitResult(&CodeAnalysisUnitResult{
		EntryFile: "mod/BarService.java", FeatureID: "svc",
		Functions: []CodeAnalysisFunction{{Name: "run"}},
	}, job))
	require.Error(t, validateCodeAnalysisUnitResult(&CodeAnalysisUnitResult{
		EntryFile: "mod/BarService.java", FeatureID: "svc",
	}, job))
	require.NoError(t, validateCodeAnalysisUnitResult(&CodeAnalysisUnitResult{
		EntryFile: "mod/BarService.java", FeatureID: "svc",
		NoCallableReason: "empty config stub",
	}, job))
}

func TestAllRegistryUnitsCompleted(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	rt := &Runtime{WorkDir: dir}
	reg := &CodeUnitRegistryV1{Units: []CodeUnitEntry{{RelPath: "mod/A.java"}}}
	require.NoError(t, persistCodeUnitRegistry(rt, reg))
	ok, _ := allRegistryUnitsCompleted(rt)
	require.False(t, ok)
	require.NoError(t, saveFeatureWorkProgress(dir, featureWorkProgress{
		Entries: []featureWorkProgressEntry{{EntryFile: "mod/A.java", Status: featureWorkStatusDone}},
	}))
	ok, _ = allRegistryUnitsCompleted(rt)
	require.True(t, ok)
}
