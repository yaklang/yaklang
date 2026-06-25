package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestEnsureBusinessFunctionMapFromFeatureInventory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	inv := &FeatureInventoryV1{
		SchemaVersion: 1,
		Features: []FeatureInventoryEntry{
			{FeatureID: "admin", Label: "Admin", PackagePatterns: []string{"*.controller.admin.*"}},
		},
		Coverage: FeatureCoverageResult{Policy: "controller_java_packages", TotalRequired: 1, Covered: 1, Complete: true},
	}
	require.NoError(t, persistFeatureInventory(&Runtime{WorkDir: dir}, inv))

	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{UUID: uuid.NewString(), Language: "java"},
	}
	require.NoError(t, EnsureBusinessFunctionMapFromFeatureInventory(rt))
	require.FileExists(t, store.BusinessFunctionMapPath(dir))
	m, err := loadBusinessFunctionMap(dir)
	require.NoError(t, err)
	require.True(t, m.Coverage.Complete)
	require.Contains(t, m.Functions, "admin")
}

func TestCountPhase1CanonicalRoutes_UsesFeatureApiMapVerifiedOnly(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	seedPhase1UnitGateFixtures(t, rt, "mod/FooController.java", []FeatureApiEntry{
		{Method: "GET", PathPattern: "/apis", Verified: true},
		{Method: "POST", PathPattern: "/upload", Verified: true},
		{Method: "GET", PathPattern: "/skip", Verified: false},
	})
	require.Equal(t, 2, countPhase1CanonicalRoutes(rt))
}

func TestSyncFeatureApiMapToVerifiedHttpApis(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, persistFeatureApiMap(&Runtime{WorkDir: dir}, &FeatureApiMapV1{
		SchemaVersion: 1,
		Features: []FeatureApiMapEntry{{
			FeatureID: "api", Processed: true,
			Apis: []FeatureApiEntry{{Method: "GET", PathPattern: "/apis", Verified: false, RejectReason: "auth_required_skipped"}},
		}},
	}))
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	require.NoError(t, SyncFeatureApiMapToVerifiedHttpApis(rt))
	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "auth_required_skipped", rows[0].RejectReason)
}

func TestRunPhase1FullApiVerificationGate_FailFastWithoutGapRepair(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, writePhase1ContractStubArtifacts(dir))

	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: dir, CodePathOK: true, TargetReachable: true,
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	decision := &CoverageSignalDecision{Verdict: "finish", Reasoning: "test stub", SignalJSON: "{}"}
	decB, _ := json.MarshalIndent(decision, "", "  ")
	require.NoError(t, repo.UpsertPhaseArtifact(sess.ID, "coverage_signal_decision", string(decB)))
	inv := newFakeInvoker(t)
	err := RunPhase1FullApiVerificationGate(context.Background(), inv, rt)
	require.Error(t, err)
	require.True(t, IsPhase1VerificationGateFailed(err))
	require.Contains(t, err.Error(), "探测覆盖")
	require.Empty(t, inv.lastToolName)
}
