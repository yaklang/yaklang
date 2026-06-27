package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestBuildJavaBusinessScopeInventory_SingleModule(t *testing.T) {
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

	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)
	require.Equal(t, "single_module", inv.Layout)
	require.GreaterOrEqual(t, inv.Stats.JavaPackageUnits, 2)
	require.FileExists(t, store.JavaBusinessScopeInventoryPath(workDir))
}

func TestBuildJavaBusinessScopeInventory_MultiModule(t *testing.T) {
	fixture := filepath.Join("testfixtures", "multi_module_maven")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)

	workDir := t.TempDir()
	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{CodeRootPath: abs, CodePathOK: true, Language: "java"},
	}
	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)
	require.Equal(t, "multi_module", inv.Layout)
	require.Len(t, inv.Modules, 3)
	require.GreaterOrEqual(t, inv.Stats.JavaPackageUnits, 4)
}

func TestEvaluateJavaBusinessCoverage_ModuleRootCoversChildren(t *testing.T) {
	fixture := filepath.Join("testfixtures", "multi_module_maven")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)
	rt := &Runtime{Session: &store.DiscoverySession{CodeRootPath: abs, CodePathOK: true, Language: "java"}}
	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)

	cov := evaluateJavaBusinessCoverage(inv, []string{"order-service"})
	require.False(t, cov.Complete)
	require.Greater(t, len(cov.UncoveredUnits), 0)

	full := evaluateJavaBusinessCoverage(inv, []string{
		"order-service",
		"payment-service",
		"common-lib",
	})
	require.True(t, full.Complete, full.Feedback)
}

func TestEvaluateJavaBusinessCoverage_ParentPackageCoversChild(t *testing.T) {
	fixture := filepath.Join("testfixtures", "multi_module_maven")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)
	rt := &Runtime{Session: &store.DiscoverySession{CodeRootPath: abs, CodePathOK: true, Language: "java"}}
	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)

	cov := evaluateJavaBusinessCoverage(inv, []string{
		"order-service/src/main/java/com/acme/order",
		"payment-service/src/main/java/com/acme/payment",
		"common-lib/src/main/java/com/acme/common",
	})
	require.True(t, cov.Complete, cov.Feedback)
}

func TestEvaluateJavaBusinessCoverage_SiblingMissing(t *testing.T) {
	fixture := filepath.Join("testfixtures", "multi_module_maven")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)
	rt := &Runtime{Session: &store.DiscoverySession{CodeRootPath: abs, CodePathOK: true, Language: "java"}}
	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)

	cov := evaluateJavaBusinessCoverage(inv, []string{
		"order-service/src/main/java/com/acme/order",
		"common-lib/src/main/java/com/acme/common",
	})
	require.False(t, cov.Complete)
	require.Contains(t, cov.Feedback, "payment")
}
