package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func writeJavaSqliBenchFixture(t *testing.T, root string) {
	t.Helper()
	dirs := []string{
		"src/main/java/com/bench/sqli/controller",
		"src/main/resources",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project></project>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/main/resources/application.properties"), []byte("server.port=8090\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/main/java/com/bench/sqli/controller/UserController.java"), []byte(`package com.bench.sqli.controller;
@RestController
@RequestMapping("/api/users")
public class UserController {
    @GetMapping("/search")
    public Map search(@RequestParam String keyword) { return null; }
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/main/java/com/bench/sqli/controller/AuthController.java"), []byte(`package com.bench.sqli.controller;
@RestController
@RequestMapping("/api/auth")
public class AuthController {
    @PostMapping("/login")
    public Map login(@RequestBody Map body) { return null; }
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/main/java/com/bench/sqli/controller/OrderController.java"), []byte(`package com.bench.sqli.controller;
@RestController
@RequestMapping("/api/orders")
public class OrderController {
    @GetMapping("/{orderId}")
    public Map getOrder(@PathVariable String orderId) { return null; }
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/main/java/com/bench/sqli/controller/ProductController.java"), []byte(`package com.bench.sqli.controller;
@RestController
@RequestMapping("/api/products")
public class ProductController {
    @GetMapping
    public Map listProducts(@RequestParam(required = false) String category) { return null; }
}
`), 0o644))
}

func TestCollectStaticRouteHints_JavaSqliBench(t *testing.T) {
	workDir := t.TempDir()
	codeDir := t.TempDir()
	writeJavaSqliBenchFixture(t, codeDir)

	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: codeDir, CodePathOK: true, Language: "java",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	rep, err := CollectStaticRouteHints(context.Background(), nil, rt)
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.GreaterOrEqual(t, rep.Count, 4)
	require.FileExists(t, store.StaticRouteHintsPath(workDir))

	paths := map[string]bool{}
	for _, h := range rep.Hints {
		paths[h.Method+" "+h.PathPattern] = true
	}
	require.True(t, paths["GET /api/users/search"])
	require.True(t, paths["POST /api/auth/login"])
	require.True(t, paths["GET /api/orders/{orderId}"])
	require.True(t, paths["GET /api/products"])

	eps, err := repo.ListHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Empty(t, eps, "Phase1A hints must not write http_endpoints")
}

func TestSyncAICodeReadingRoutesToEndpoints(t *testing.T) {
	workDir := t.TempDir()
	codeDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: codeDir, CodePathOK: true, Language: "java",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	plan := map[string]any{
		"discovered_apis": []any{
			map[string]any{
				"method": "GET", "path_pattern": "/api/users/search",
				"handler_file": "src/main/java/com/bench/sqli/controller/UserController.java",
				"handler_symbol": "search", "class_base_path": "/api/users",
				"code_evidence": "@RequestMapping + @GetMapping",
			},
			map[string]any{
				"method": "POST", "path_pattern": "/api/auth/login",
				"handler_file": "src/main/java/com/bench/sqli/controller/AuthController.java",
				"handler_symbol": "login", "class_base_path": "/api/auth",
				"code_evidence": "@PostMapping",
			},
		},
		"read_files_completed": []any{
			"src/main/java/com/bench/sqli/controller/UserController.java",
			"src/main/java/com/bench/sqli/controller/AuthController.java",
		},
		"url_spaces": map[string]any{
			"UserController": map[string]any{"base_path": "/api/users", "methods": []any{"GET"}},
			"AuthController": map[string]any{"base_path": "/api/auth", "methods": []any{"POST"}},
		},
		"read_queue": []any{"src/main/java/com/bench/sqli/controller/UserController.java"},
		"hint_diff": "n/a",
	}
	b, _ := json.MarshalIndent(plan, "", "  ")
	require.NoError(t, writeJSONFile(store.CodeReadingPlanPath(workDir), b))

	n, err := SyncAICodeReadingRoutesToEndpoints(rt)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	eps, err := repo.ListHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Len(t, eps, 2)
	for _, e := range eps {
		require.Equal(t, SourceAICodeRead, e.Source)
	}
}

func TestSupplementStaticRouteHints_DoesNotOverwriteAI(t *testing.T) {
	workDir := t.TempDir()
	codeDir := t.TempDir()
	writeJavaSqliBenchFixture(t, codeDir)

	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: codeDir, CodePathOK: true, Language: "java",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/users/search", Source: SourceAICodeRead,
		Status: store.EndpointStatusPendingValidation,
	}))

	_, err = CollectStaticRouteHints(context.Background(), nil, rt)
	require.NoError(t, err)
	ins, _, err := SupplementStaticRouteHints(context.Background(), nil, rt)
	require.NoError(t, err)
	require.Greater(t, ins, 0)

	eps, err := repo.ListHttpEndpoints(sess.ID)
	require.NoError(t, err)
	for _, e := range eps {
		if e.PathPattern == "/api/users/search" {
			require.Equal(t, SourceAICodeRead, e.Source)
		}
	}
}

func TestVerifyPhase1Gate_RequiresAICatalogCoverage(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	workDir := t.TempDir()
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true, Language: "java",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	require.Error(t, verifyPhase1ApiVerificationGate(rt))

	seedPhase1UnitGateFixtures(t, rt, "mod/StubController.java", []FeatureApiEntry{{
		Method: "GET", PathPattern: "/a", Verified: true,
		FullSampleURL: "http://127.0.0.1:8080/a", VerdictReason: "hit",
	}})
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/a",
		Verified: true, FullSampleURL: "http://127.0.0.1:8080/a",
		ProbeStatusCode: 200, ProbeAttemptsJSON: `[{"status":200}]`,
	}))
	require.NoError(t, verifyPhase1ApiVerificationGate(rt))
}

func TestIsAIPrimaryEndpointSource(t *testing.T) {
	require.True(t, IsAIPrimaryEndpointSource(SourceAICodeRead))
	require.True(t, IsAIPrimaryEndpointSource("ai"))
	require.True(t, IsAIPrimaryEndpointSource("ai_probe"))
	require.False(t, IsAIPrimaryEndpointSource(SourceStaticHint))
}
