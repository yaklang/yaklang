package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestControllerVerifyExtraManifest(t *testing.T) {
	require.Len(t, ControllerVerifyExtraManifest, 11)
	ids := extraManifestBlockIDs()
	require.Contains(t, ids, "controller_task")
	require.Contains(t, ids, "probe_context")
}

func TestBuildControllerVerifyExtraContainsTaskBlock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writePhase1ContractStubArtifacts(dir))
	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true, CodeRootPath: "/code/root"},
	}
	job := ControllerVerifyJob{
		EntryFile:  "publiccms-core/src/main/java/com/example/FooController.java",
		FeatureID:  "api_core",
		FeatureLabel: "API",
		StaticHints: []StaticRouteHint{
			{Method: "GET", PathPattern: "/api/foo", HandlerClass: "FooController"},
		},
	}
	extra := buildControllerVerifyExtra(rt, job)
	require.Contains(t, extra, "## controller_task")
	require.Contains(t, extra, "FooController.java")
	require.Contains(t, extra, "/api/foo")
	require.Contains(t, extra, "auth_surface_map")
}

func TestCollectControllerVerifyJobs_DedupesControllers(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	inv := &FeatureInventoryV1{
		SchemaVersion: 1,
		Features: []FeatureInventoryEntry{
			{
				FeatureID: "f1", Label: "F1", SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles: []string{
					"mod/AController.java",
					"mod/BController.java",
				},
			},
			{
				FeatureID: "f2", Label: "F2", SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles: []string{"mod/AController.java"},
			},
		},
	}
	require.NoError(t, persistFeatureInventory(&Runtime{WorkDir: dir}, inv))
	hints := StaticRouteHintsReport{
		Hints: []StaticRouteHint{
			{Method: "GET", PathPattern: "/a", FileRelPath: "mod/AController.java", HandlerClass: "A"},
			{Method: "POST", PathPattern: "/b", FileRelPath: "mod/BController.java", HandlerClass: "B"},
		},
	}
	require.NoError(t, writeStaticRouteHintsReport(dir, &hints))

	rt := &Runtime{WorkDir: dir, Session: &store.DiscoverySession{UUID: uuid.NewString()}}
	jobs, err := collectControllerVerifyJobs(rt, inv)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}

func TestMergeControllerResultsIntoFeatureMap(t *testing.T) {
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", Label: "F1"},
		},
	}
	apiMap := &FeatureApiMapV1{SchemaVersion: 1, Features: []FeatureApiMapEntry{}}
	results := []ControllerVerifyEntry{
		{
			FeatureID: "f1", ControllerFile: "a.java",
			Apis: []FeatureApiEntry{{Method: "GET", PathPattern: "/x"}},
		},
		{
			FeatureID: "f1", ControllerFile: "b.java",
			Apis: []FeatureApiEntry{{Method: "GET", PathPattern: "/x"}, {Method: "POST", PathPattern: "/y"}},
		},
	}
	mergeControllerResultsIntoFeatureMap(inv, apiMap, results)
	require.Len(t, apiMap.Features, 1)
	require.True(t, apiMap.Features[0].Processed)
	require.Equal(t, 2, apiMap.Features[0].ApiCount)
}

func TestControllerVerifySubSlug(t *testing.T) {
	s := controllerVerifySubSlug("publiccms-core/src/main/java/com/foo/CmsContentAdminController.java")
	require.True(t, strings.HasPrefix(s, "CmsContent"))
}

func TestHandlerMatchesControllerFile(t *testing.T) {
	require.True(t, handlerMatchesControllerFile("com.foo.CmsContentAdminController", "", "mod/CmsContentAdminController.java"))
	require.True(t, handlerMatchesControllerFile("", "mod/CmsContentAdminController.java", "mod/CmsContentAdminController.java"))
}

func TestControllerVerifyConcurrent_Default(t *testing.T) {
	t.Setenv("YAK_SSA_API_DISCOVERY_CONTROLLER_VERIFY_CONCURRENT", "")
	require.Equal(t, 4, controllerVerifyConcurrent())
}
