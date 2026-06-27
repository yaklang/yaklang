package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestRunPhase1TechArchProgrammaticFallback(t *testing.T) {
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
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	_, err = RunBuildProjectProfile(rt)
	require.NoError(t, err)
	_, err = BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)

	require.NoError(t, runPhase1TechArchProgrammaticFallback(rt))
	rec, err := loadTechArchitectureRecord(workDir)
	require.NoError(t, err)
	require.Equal(t, "java", rec.Language)
	require.NotEmpty(t, rec.SystemSummary)
	require.FileExists(t, store.TechArchitecturePath(workDir))
}

func TestPersistBusinessFunctionMap_SyncsDB(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	m := &BusinessFunctionMap{
		SchemaVersion: 1,
		Functions: map[string]BusinessFunctionEntry{
			"订单域": {
				Description: "orders",
				ScopePaths:  []string{"order-service/src/main/java/com/acme/order"},
			},
		},
		Coverage: BusinessCoverageResult{Complete: true, Covered: 1, TotalRequired: 1},
	}
	require.NoError(t, persistBusinessFunctionMap(rt, m))
	rows, err := repo.ListBusinessCapabilities(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, rows[0].ScopePathsJSON, "order-service")
}

func TestIsPhase1BusinessCoverageFailed(t *testing.T) {
	require.True(t, IsPhase1BusinessCoverageFailed(&Phase1BusinessCoverageError{Reason: "x"}))
	require.False(t, IsPhase1BusinessCoverageFailed(nil))
}
