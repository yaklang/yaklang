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

func TestPersistPhase1ReconOutput_WritesArtifact(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	out := &Phase1ReconOutput{
		EndpointsExtracted: 2,
		NextWorklist: []WorklistSeedItem{
			{RelPath: "config/AdminConfig.java", Category: worklistCategoryRoutingConfig, Priority: 1},
			{RelPath: "web/LoginController.java", Category: worklistCategoryAuthEntry, Priority: 2},
		},
		Summary: "test recon",
	}
	require.NoError(t, persistPhase1ReconOutput(rt, out))
	require.FileExists(t, store.Phase1ReconPath(workDir))

	loaded, err := loadPhase1ReconOutput(workDir)
	require.NoError(t, err)
	require.Len(t, loaded.NextWorklist, 2)
}

func TestWorklistSeedFromRuntime_PrefersPhase1Recon(t *testing.T) {
	workDir := t.TempDir()
	recon := &Phase1ReconOutput{
		NextWorklist: []WorklistSeedItem{{RelPath: "from_recon.java", Priority: 2}},
	}
	b, _ := json.MarshalIndent(recon, "", "  ")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	require.NoError(t, os.WriteFile(store.Phase1ReconPath(workDir), b, 0o644))

	rt := &Runtime{WorkDir: workDir}
	seed := worklistSeedFromRuntime(rt)
	require.Len(t, seed, 1)
	require.Equal(t, "from_recon.java", seed[0].RelPath)
}

func TestRunPhase1ReconProgrammaticFallback(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	scope := BackendScopeReport{
		ControllerFileCandidates: []struct {
			RelPath string `json:"rel_path"`
			Reason  string `json:"reason"`
		}{{RelPath: "src/HelloController.java", Reason: "controller"}},
	}
	b, _ := json.MarshalIndent(scope, "", "  ")
	require.NoError(t, os.WriteFile(store.BackendScopePath(workDir), b, 0o644))

	rt := &Runtime{WorkDir: workDir}
	require.NoError(t, runPhase1ReconProgrammaticFallback(rt))

	loaded, err := loadPhase1ReconOutput(workDir)
	require.NoError(t, err)
	require.NotEmpty(t, loaded.NextWorklist)
}

func TestBuildCodeReadingPlanFromDB_PrefersExtractSources(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/a", Source: SourceStaticHint, Status: "candidate",
	}))
	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/a", Source: SourceExtractSpring, Status: "candidate",
	}))
	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "POST", PathPattern: "/api/login", Source: SourceExtractSpring, Status: "candidate",
	}))

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	plan, err := BuildCodeReadingPlanFromDB(rt)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(plan.DiscoveredAPIs), 2)
}

func TestPersistPhase1ReconOutput_SanitizesGlobWorklist(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	out := &Phase1ReconOutput{
		NextWorklist: []WorklistSeedItem{
			{RelPath: "publiccms/src/main/java/config/spring/AdminConfig.java", Category: worklistCategoryRoutingConfig, Priority: 1},
			{RelPath: "**/*Controller.java", Category: worklistCategoryAPIHandler, Priority: 3},
		},
	}
	require.NoError(t, persistPhase1ReconOutput(rt, out))
	loaded, err := loadPhase1ReconOutput(workDir)
	require.NoError(t, err)
	require.Len(t, loaded.NextWorklist, 1)
	require.NotContains(t, loaded.NextWorklist[0].RelPath, "*")
}
