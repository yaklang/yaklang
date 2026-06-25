package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestCountPhase1CanonicalRoutes_FromFeatureApiMap(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	entry := "mod/FooController.java"
	rt := &Runtime{
		WorkDir: dir,
		Repo:    repo,
		Session: &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true},
	}
	seedPhase1UnitGateFixtures(t, rt, entry, []FeatureApiEntry{
		{Method: "GET", PathPattern: "/a", Verified: true},
		{Method: "GET", PathPattern: "/b", Verified: true},
		{Method: "GET", PathPattern: "/c", Verified: false},
		{Method: "POST", PathPattern: "/d", Verified: true},
	})
	require.Equal(t, 3, countPhase1CanonicalRoutes(rt))
}

func TestVerifyPhase1GranularGate_RequiresProbeEvidence(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	seedPhase1UnitGateFixtures(t, rt, "mod/FooController.java", []FeatureApiEntry{{
		Method: "GET", PathPattern: "/api/x", Verified: true,
		FullSampleURL: "http://127.0.0.1:8080/api/x", VerdictReason: "hit",
	}})
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/x",
		Verified: false, RejectReason: "auth_required_skipped", Source: "feature_verify",
	}))

	err := verifyPhase1GranularGate(rt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "探测覆盖仅 0 条")
}

func TestVerifyPhase1GranularGate_PassesWithProbeEvidence(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	seedPhase1UnitGateFixtures(t, rt, "mod/FooController.java", []FeatureApiEntry{{
		Method: "GET", PathPattern: "/api/x", Verified: true,
		FullSampleURL: "http://127.0.0.1:8080/api/x", VerdictReason: "hit",
	}})
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/x",
		Verified: true, FullSampleURL: "http://127.0.0.1:8080/api/x",
		ProbeStatusCode: 200, ProbeAttemptsJSON: `[{"status":200}]`,
		Source: "ai_probe",
	}))

	require.NoError(t, verifyPhase1GranularGate(rt))
}
