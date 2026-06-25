package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestIsProbeDestructivePath(t *testing.T) {
	yes, reason := isProbeDestructivePath("/logout")
	require.True(t, yes)
	require.Contains(t, reason, "destructive")
	yes, _ = isProbeDestructivePath("/admin/clearCache")
	require.True(t, yes)
	yes, _ = isProbeDestructivePath("/changePassword")
	require.True(t, yes)
	no, _ := isProbeDestructivePath("/cmsCategory/save")
	require.False(t, no)
}

func TestEnrichProbeCandidateContext_ProvidesRawHintsOnly(t *testing.T) {
	dir := t.TempDir()
	codeRoot := filepath.Join(dir, "code")
	handlerRel := "src/main/java/com/example/controller/admin/DemoController.java"
	handlerPath := filepath.Join(codeRoot, handlerRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(handlerPath), 0o755))
	src := `package com.example.controller.admin;
@Controller
@RequestMapping("demo")
public class DemoController {
    @PostMapping("save")
    @Csrf
    public String save(String name) { return "ok"; }
}`
	require.NoError(t, os.WriteFile(handlerPath, []byte(src), 0o644))

	plan := CodeReadingPlan{
		DiscoveredAPIs: []DiscoveredAPI{{
			Method:        "GET",
			PathPattern:   "/demo/save",
			HandlerClass:  "com.example.controller.admin.DemoController",
			HandlerFile:   handlerRel,
			HandlerSymbol: "save",
			CodeEvidence:  "static_hint fallback:static_hint",
		}},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))
	planBytes, err := json.MarshalIndent(plan, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.CodeReadingPlanPath(dir), planBytes, 0o644))

	rpJSON := `{"schema_version":1,"url_spaces":[{"id":"admin","mount_prefix":"/admin"}]}`
	require.NoError(t, os.WriteFile(store.RoutingProfilePath(dir), []byte(rpJSON), 0o644))

	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: codeRoot, CodePathOK: true,
		TargetRaw: "http://127.0.0.1:8080", TargetReachable: true,
		RoutingProfileJSON: rpJSON,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}

	ctx := &ProbeCandidateContext{Method: "GET", PathPattern: "/demo/save"}
	enrichProbeCandidateContext(rt, ctx)

	require.Contains(t, ctx.CodeSnippet, "@PostMapping", "code_snippet must be loaded")
	require.NotEmpty(t, ctx.URLSpace, "url_space must be resolved")
	require.NotEmpty(t, ctx.AuthSelectionJSON, "auth_selection_json must be provided as raw hint")
	require.NotEmpty(t, ctx.RoutingProfileExcerpt, "routing_profile_excerpt must be provided as raw hint")

	var authSel map[string]any
	require.NoError(t, json.Unmarshal([]byte(ctx.AuthSelectionJSON), &authSel))
	_, hasMultiAuth := authSel["multi_auth"]
	require.True(t, hasMultiAuth, "auth_selection_json must keep multi_auth signal")
}
