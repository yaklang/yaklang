package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestJoinMountAndRelativePath(t *testing.T) {
	require.Equal(t, "/admin/dict/save", joinMountAndRelativePath("/admin", "/dict/save"))
	require.Equal(t, "/admin/dict/save", joinMountAndRelativePath("/admin", "dict/save"))
	require.Equal(t, "/admin/dict/save", joinMountAndRelativePath("/admin", "/admin/dict/save"))
	require.Equal(t, "/dict/save", joinMountAndRelativePath("/", "/dict/save"))
}

func TestEnrichStaticRouteHintPath_AdminPackage(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	surface := &AuthSurfaceMapV1{
		SchemaVersion: 1,
		Surfaces: []AuthSurfaceEntry{
			{
				AuthRealm:       "admin",
				PackagePatterns: []string{"com.publiccms.controller.admin.*"},
				PathPrefixes:    []string{"/admin"},
			},
		},
	}
	require.NoError(t, writeArtifactJSON(store.AuthSurfaceMapPath(dir), surface))

	rt := &Runtime{WorkDir: dir}
	job := FeatureWorkJob{
		EntryFile:       "src/com/publiccms/controller/admin/sys/DictAdminController.java",
		PackagePatterns: []string{"com.publiccms.controller.admin.*"},
	}
	hint := StaticRouteHint{
		Method:       "POST",
		PathPattern:  "/dict/save",
		HandlerClass: "com.publiccms.controller.admin.sys.DictAdminController",
	}
	out := enrichStaticRouteHintPath(rt, hint, job)
	require.Equal(t, "/admin/dict/save", out.PathPattern)
}

func TestMergeControllerResultsIntoFeatureMap_PreservesOtherFeatures(t *testing.T) {
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", Label: "F1"},
			{FeatureID: "f2", Label: "F2"},
		},
	}
	apiMap := &FeatureApiMapV1{
		SchemaVersion: 1,
		Features: []FeatureApiMapEntry{
			{
				FeatureID: "f2",
				Label:     "F2",
				Processed: true,
				Apis:      []FeatureApiEntry{{Method: "GET", PathPattern: "/keep"}},
				ApiCount:  1,
			},
		},
	}
	mergeControllerResultsIntoFeatureMap(inv, apiMap, []ControllerVerifyEntry{
		{
			FeatureID: "f1",
			Apis:      []FeatureApiEntry{{Method: "POST", PathPattern: "/admin/new"}},
		},
	})
	require.Len(t, apiMap.Features, 2)
	var f1, f2 *FeatureApiMapEntry
	for i := range apiMap.Features {
		switch apiMap.Features[i].FeatureID {
		case "f1":
			f1 = &apiMap.Features[i]
		case "f2":
			f2 = &apiMap.Features[i]
		}
	}
	require.NotNil(t, f1)
	require.NotNil(t, f2)
	require.Equal(t, 1, f1.ApiCount)
	require.Equal(t, "/admin/new", f1.Apis[0].PathPattern)
	require.Equal(t, 1, f2.ApiCount)
	require.Equal(t, "/keep", f2.Apis[0].PathPattern)
}
