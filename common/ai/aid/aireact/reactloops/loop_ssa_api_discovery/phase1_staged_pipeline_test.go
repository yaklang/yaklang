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

func TestRunBuildProjectProfile_MinimalJavaWebapp(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)

	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: abs, CodePathOK: true, Language: "java",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	profile, err := RunBuildProjectProfile(rt)
	require.NoError(t, err)
	require.NotNil(t, profile)
	require.FileExists(t, store.ProjectProfilePath(workDir))
	require.NotEmpty(t, profile.Files)
	require.NotEmpty(t, profile.Frameworks)
	require.Empty(t, profile.WorklistSeed, "worklist comes from phase1_recon, not project_profile")
	require.Contains(t, profile.ContextPath, "unknown") // no context-path in fixture yml

	var loaded ProjectProfileV1
	b, err := os.ReadFile(store.ProjectProfilePath(workDir))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &loaded))
}

func TestAssembleApiCatalogFromStages_Minimal(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	plan := &CodeReadingPlan{
		DiscoveredAPIs: []DiscoveredAPI{
			{Method: "GET", PathPattern: "/api/health", HandlerFile: "HelloController.java", CodeEvidence: "test"},
		},
		URLSpaces: map[string]any{"default": map[string]any{"base_path": "/"}},
	}
	require.NoError(t, PersistCodeReadingPlan(&Runtime{WorkDir: workDir}, plan))
	profile := &ProjectProfileV1{ContextPath: "unknown", GeneratedAt: "now"}
	b, _ := json.MarshalIndent(profile, "", "  ")
	require.NoError(t, os.WriteFile(store.ProjectProfilePath(workDir), b, 0o644))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{TargetScheme: "http", TargetHost: "127.0.0.1", TargetPort: "8080"},
	}
	catalog, err := AssembleApiCatalogFromStages(rt)
	require.NoError(t, err)
	require.Len(t, catalog.Entries, 1)
	require.FileExists(t, store.ApiCatalogPath(workDir))
}

func TestClassifyFileCategory(t *testing.T) {
	require.Equal(t, fileCategoryCode, classifyFileCategory("src/Foo.java", nil))
	require.Equal(t, fileCategoryResource, classifyFileCategory("static/app.js", nil))
	require.Equal(t, fileCategoryBuild, classifyFileCategory("pom.xml", nil))
}

func TestInferBasePathFromHandlerClass_RemovedHeuristic(t *testing.T) {
	require.Equal(t, "", inferBasePathFromHandlerClass("com.example.controller.admin.Foo"))
}
