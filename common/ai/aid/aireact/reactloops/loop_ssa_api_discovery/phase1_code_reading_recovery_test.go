package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestRequiredReadFilesForCodeReadingPlan_PrefersDiscoveredAPIHandlers(t *testing.T) {
	rt := &Runtime{Session: &store.DiscoverySession{CodePathOK: true, CodeRootPath: "/tmp/code"}}
	plan := map[string]any{
		"discovered_apis": []any{
			map[string]any{
				"method":       "GET",
				"path_pattern": "/admin/login",
				"handler_file": "publiccms-parent/publiccms-core/src/main/java/com/publiccms/controller/admin/LoginAdminController.java",
			},
		},
	}
	candidates := []string{
		"publiccms-parent/publiccms-common/src/main/java/com/publiccms/common/base/BaseHandler.java",
		"publiccms-parent/publiccms/src/test/resources/generator/java/controller.ftl",
	}
	required := requiredReadFilesForCodeReadingPlan(plan, candidates, rt)
	require.Len(t, required, 1)
	require.Contains(t, required[0], "LoginAdminController.java")
}

func TestEnsureCodeReadingPlanFile_FallbackFromStaticHints(t *testing.T) {
	workDir := t.TempDir()
	codeDir := t.TempDir()
	writeJavaSqliBenchFixture(t, codeDir)

	discoveryDir := filepath.Join(workDir, "ssa_discovery")
	require.NoError(t, os.MkdirAll(discoveryDir, 0o755))

	hints := StaticRouteHintsReport{
		Language: "java",
		Hints: []StaticRouteHint{
			{Method: "GET", PathPattern: "/api/users/search", HandlerClass: "com.bench.sqli.controller.UserController", HandlerMethod: "search", FileRelPath: "src/main/java/com/bench/sqli/controller/UserController.java", Source: "static_java_spring_annotations"},
		},
		Count: 1,
	}
	b, _ := json.MarshalIndent(hints, "", "  ")
	require.NoError(t, os.WriteFile(store.StaticRouteHintsPath(workDir), b, 0o644))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{CodePathOK: true, CodeRootPath: codeDir},
	}
	require.NoError(t, EnsureCodeReadingPlanFile(rt))

	plan, err := LoadCodeReadingPlan(workDir)
	require.NoError(t, err)
	require.Len(t, plan.DiscoveredAPIs, 1)
	require.Equal(t, "/api/users/search", plan.DiscoveredAPIs[0].PathPattern)
}
