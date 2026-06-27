package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestComputeCoverageSignal_NilRuntime(t *testing.T) {
	sig, err := ComputeCoverageSignal(nil)
	require.Error(t, err)
	require.Nil(t, sig)
}

func TestComputeCoverageSignal_EmptySession(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	rt := &Runtime{WorkDir: dir}
	sig, err := ComputeCoverageSignal(rt)
	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Equal(t, 0.0, sig.RouteCoveragePct)
	require.Equal(t, 0.0, sig.EntryCoveragePct)
	require.Equal(t, 0, len(sig.DiscoveredRoutes))
	require.Equal(t, 0, len(sig.StaticHarvestRoutes))
}

func TestComputeCoverageSignal_WithStaticHints(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// Write a static_route_hints.json with some hints.
	hints := `{
		"hints": [
			{"method":"GET","path_pattern":"/admin/category/list"},
			{"method":"POST","path_pattern":"/admin/category/save"}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(sub, "static_route_hints.json"), []byte(hints), 0o644))

	rt := &Runtime{WorkDir: dir}
	sig, err := ComputeCoverageSignal(rt)
	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Equal(t, 2, len(sig.StaticHarvestRoutes))
	require.Equal(t, 0, len(sig.DiscoveredRoutes))
	require.Equal(t, 2, len(sig.UndiscoveredRoutes))
	require.Equal(t, 0.0, sig.RouteCoveragePct)
}

func TestComputeCoverageSignal_WithFeatureApiMap(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// Write static_route_hints.json.
	hints := `{"hints":[{"method":"GET","path_pattern":"/admin/category/list"},{"method":"POST","path_pattern":"/admin/category/save"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(sub, "static_route_hints.json"), []byte(hints), 0o644))

	// Write feature_api_map.json with one route discovered.
	apiMap := `{
		"features":[{
			"feature_id":"f1","processed":true,
			"apis":[{"method":"GET","path_pattern":"/admin/category/list"}]
		}]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(sub, "feature_api_map.json"), []byte(apiMap), 0o644))

	rt := &Runtime{WorkDir: dir}
	sig, err := ComputeCoverageSignal(rt)
	require.NoError(t, err)
	require.Equal(t, 2, len(sig.StaticHarvestRoutes))
	require.Equal(t, 1, len(sig.DiscoveredRoutes))
	require.Equal(t, 1, len(sig.UndiscoveredRoutes))
	require.InDelta(t, 50.0, sig.RouteCoveragePct, 0.1)
}

func TestComputeCoverageSignal_HttpEntryCoverage(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// Write registry with 3 http_entry units.
	reg := `{
		"units":[
			{"rel_path":"mod/AController.java","kind_hint":"http_entry"},
			{"rel_path":"mod/BController.java","kind_hint":"http_entry"},
			{"rel_path":"mod/CService.java","kind_hint":"service"}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(sub, "code_unit_registry.json"), []byte(reg), 0o644))

	// Write feature_api_map with one controller processed.
	apiMap := `{
		"features":[{
			"feature_id":"f1","processed":true,
			"apis":[{"method":"GET","path_pattern":"/a","handler_file":"mod/AController.java"}]
		}]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(sub, "feature_api_map.json"), []byte(apiMap), 0o644))

	rt := &Runtime{WorkDir: dir}
	sig, err := ComputeCoverageSignal(rt)
	require.NoError(t, err)
	require.Equal(t, 2, sig.TotalHttpEntries)   // only http_entry units
	require.Equal(t, 1, sig.AnalyzedEntries)    // one controller processed
	require.InDelta(t, 50.0, sig.EntryCoveragePct, 0.1)
	require.Equal(t, 1, len(sig.PendingEntries))
}

func TestTierPendingEntries(t *testing.T) {
	entries := []string{
		"mod/controller/admin/CmsController.java",
		"mod/controller/web/IndexController.java",
		"mod/service/cms/CmsService.java",
		"mod/security/SecurityConfig.java",
		"mod/config/AppConfig.java",
	}
	tiers := tierPendingEntries(entries)
	require.Equal(t, 2, len(tiers.P0)) // controller files
	require.Equal(t, 2, len(tiers.P1)) // service + security
	require.Equal(t, 1, len(tiers.P2)) // config
}

func TestSuggestedReadQueue(t *testing.T) {
	tiers := PriorityTiers{
		P0: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		P1: []string{"k"},
	}
	queue := suggestedReadQueue(tiers)
	require.LessOrEqual(t, len(queue), 8)
	require.Equal(t, "a", queue[0])
	// Should only include P0 up to batch size
	require.Len(t, queue, 8)
}

func TestComputeConfidenceLevel(t *testing.T) {
	tests := []struct {
		name        string
		pct         float64
		undisc      int
		entryPct    float64
		want        string
	}{
		{"high: 90% routes, 95% entries", 90, 2, 95, "high"},
		{"medium: 60% routes", 60, 10, 40, "medium"},
		{"low: 20% routes", 20, 50, 20, "low"},
		{"high boundary: 85% routes, 90% entries", 85, 5, 90, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := &CoverageSignal{
				RouteCoveragePct:  tt.pct,
				EntryCoveragePct:   tt.entryPct,
				UndiscoveredRoutes: make([]string, tt.undisc),
			}
			got := computeConfidenceLevel(sig)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCoverageSignalSummary(t *testing.T) {
	sig := &CoverageSignal{
		RouteCoveragePct:  62.5,
		EntryCoveragePct: 75.0,
		AnalyzedEntries:  6,
		TotalHttpEntries: 8,
	}
	summary := CoverageSignalSummary(sig)
	require.Contains(t, summary, "62.5%")
	require.Contains(t, summary, "75.0%")
	require.Contains(t, summary, "6/8")
}

func TestCoverageSignalForPrompt(t *testing.T) {
	sig := &CoverageSignal{
		RouteCoveragePct: 50.0,
	}
	out := CoverageSignalForPrompt(sig)
	require.Contains(t, out, "50")
	require.NotEmpty(t, out)
}

func TestSummarizeCoverageSignalForReAct(t *testing.T) {
	sig := &CoverageSignal{
		StaticHarvestRoutes: []string{"GET /a", "POST /b"},
		DiscoveredRoutes:    []string{"GET /a"},
		UndiscoveredRoutes: []string{"POST /b"},
		RouteCoveragePct:    50.0,
		TotalHttpEntries:    4,
		AnalyzedEntries:    2,
		EntryCoveragePct:    50.0,
		PriorityTiers: PriorityTiers{
			P0: []string{"C1.java", "C2.java"},
			P1: []string{"S1.java"},
			P2: []string{"Cfg.java"},
		},
		ConfidenceLevel: "medium",
	}
	out := SummarizeCoverageSignalForReAct(sig)
	require.Contains(t, out, "50.0%")
	require.Contains(t, out, "static harvest routes")
	require.Contains(t, out, "P0")
	require.Contains(t, out, "medium")
}

func TestPersistAndLoadCoverageSignal(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sig := &CoverageSignal{
		RouteCoveragePct: 75.0,
		EntryCoveragePct: 60.0,
	}
	require.NoError(t, PersistCoverageSignal(&Runtime{WorkDir: dir}, sig))

	loaded, err := LoadCoverageSignal(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.InDelta(t, 75.0, loaded.RouteCoveragePct, 0.1)
}

func TestSortByPriorityTier(t *testing.T) {
	jobs := []FeatureWorkJob{
		{EntryFile: "mod/config/AppConfig.java", SurfaceKind: SurfaceKindHTTPAPI},
		{EntryFile: "mod/service/cms/CmsService.java", SurfaceKind: SurfaceKindHTTPAPI},
		{EntryFile: "mod/controller/admin/CmsController.java", SurfaceKind: SurfaceKindHTTPAPI},
		{EntryFile: "mod/controller/web/IndexController.java", SurfaceKind: SurfaceKindHTTPAPI},
	}
	sorted := sortByPriorityTier(jobs)
	// P0 (controller) should come first
	require.Contains(t, sorted[0].EntryFile, "controller")
	require.Contains(t, sorted[1].EntryFile, "controller")
	// P1 (service) should be second
	require.Contains(t, sorted[2].EntryFile, "service")
	// P2 (config) should be last
	require.Contains(t, sorted[3].EntryFile, "config")
}

func TestApplyReActQueueUpdate(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	rt := &Runtime{WorkDir: dir}

	pending := []FeatureWorkJob{
		{EntryFile: "mod/A.java"},
		{EntryFile: "mod/B.java"},
		{EntryFile: "mod/C.java"},
	}
	queueUpdate := []string{"mod/C.java", "mod/A.java"}
	reordered := applyReActQueueUpdate(rt, pending, queueUpdate)
	require.Equal(t, "mod/C.java", reordered[0].EntryFile)
	require.Equal(t, "mod/A.java", reordered[1].EntryFile)
	// B should still be at the end
	require.Equal(t, "mod/B.java", reordered[2].EntryFile)
}

func TestApplyReActQueueUpdate_UnknownPaths(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	rt := &Runtime{WorkDir: dir}

	pending := []FeatureWorkJob{
		{EntryFile: "mod/A.java"},
	}
	queueUpdate := []string{"mod/X.java", "mod/Y.java"}
	reordered := applyReActQueueUpdate(rt, pending, queueUpdate)
	// Unknown paths are skipped; original A remains
	require.Len(t, reordered, 1)
	require.Equal(t, "mod/A.java", reordered[0].EntryFile)
}

func TestCoverageRouteKey(t *testing.T) {
	tests := []struct {
		method, path, want string
	}{
		{"get", "/admin/category", "GET admin/category"},
		{"  POST  ", "/api/users", "POST api/users"},
		{"", "/", " "},
		{"GET", "", "GET "},
	}
	for _, tt := range tests {
		got := coverageRouteKey(tt.method, tt.path)
		require.Equal(t, tt.want, got)
	}
}
