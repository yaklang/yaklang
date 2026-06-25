package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestRouteFileCandidateFromPath_ExcludesBareHandler(t *testing.T) {
	rel := "publiccms-common/src/main/java/com/publiccms/common/base/BaseHandler.java"
	got, reason := routeFileCandidateFromPath(rel)
	require.Empty(t, got)
	require.Empty(t, reason)

	rel2 := "publiccms-core/src/main/java/com/publiccms/controller/admin/LoginAdminController.java"
	got2, _ := routeFileCandidateFromPath(rel2)
	require.NotEmpty(t, got2)
}

func TestPublicCMSBackendScopeFilter(t *testing.T) {
	paPath := "/home/murkfox/yakit-projects/aispace/1222_publiccms_java_config_20260602_2ccbb/ssa_discovery/api_preanalysis.json"
	shPath := "/home/murkfox/yakit-projects/aispace/1222_publiccms_java_config_20260602_2ccbb/ssa_discovery/static_route_hints.json"
	if _, err := os.Stat(paPath); err != nil {
		t.Skip("PublicCMS fixture not available locally")
	}

	workDir := t.TempDir()
	discoveryDir := filepath.Join(workDir, "ssa_discovery")
	require.NoError(t, os.MkdirAll(discoveryDir, 0o755))
	paBytes, err := os.ReadFile(paPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.ApiPreanalysisReportPath(workDir), paBytes, 0o644))
	shBytes, err := os.ReadFile(shPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.StaticRouteHintsPath(workDir), shBytes, 0o644))

	var pre struct {
		RouteFileCandidates []struct {
			RelPath string `json:"rel_path"`
		} `json:"route_file_candidates"`
	}
	require.NoError(t, json.Unmarshal(paBytes, &pre))
	require.GreaterOrEqual(t, len(pre.RouteFileCandidates), 100)

	staticFiles := staticHintFileSet(workDir)
	require.GreaterOrEqual(t, len(staticFiles), 70)

	rep := APIPreanalysisReport{
		CodeRoot:            "/tmp/publiccms",
		Language:            "java",
		RouteFileCandidates: make([]struct {
			RelPath string `json:"rel_path"`
			Reason  string `json:"reason"`
		}, len(pre.RouteFileCandidates)),
	}
	for i, c := range pre.RouteFileCandidates {
		rep.RouteFileCandidates[i].RelPath = c.RelPath
	}
	enrichPreanalysisNarrowFields(&rep, workDir)

	require.Less(t, len(rep.ApiRouteFiles), len(pre.RouteFileCandidates))
	require.GreaterOrEqual(t, len(rep.ApiRouteFiles), len(staticFiles))
	require.LessOrEqual(t, len(rep.ApiRouteFiles), 99)

	for _, p := range rep.ApiRouteFiles {
		require.False(t, isHandlerPathMisreport(p), "handler misreport in api_route_files: %s", p)
	}
}

func TestIsNarrowControllerCandidate_StaticHintWins(t *testing.T) {
	static := map[string]struct{}{
		"pkg/FooHandler.java": {},
	}
	ok, reason := isNarrowControllerCandidate("pkg/FooHandler.java", "java", static)
	require.True(t, ok)
	require.Equal(t, "static_hint_ref", reason)
}
