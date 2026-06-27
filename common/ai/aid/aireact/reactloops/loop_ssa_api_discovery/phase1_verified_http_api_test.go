package loop_ssa_api_discovery

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestVerifiedHttpApi_RepositoryUpsert(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))

	row := &store.VerifiedHttpApi{
		SessionID:       sess.ID,
		Method:          "GET",
		PathPattern:     "/api/user",
		Verified:        true,
		Confidence:      80,
		ProbeStatusCode: 200,
		ResponseExcerpt: "ok",
		VerdictReason:   "200 JSON",
		Source:          "test",
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(row))
	require.NotZero(t, row.ID)

	row.Verified = false
	row.RejectReason = "retest"
	row.ProbeStatusCode = 0
	row.ProbeAttemptsJSON = ""
	row.ResponseExcerpt = ""
	require.NoError(t, repo.UpsertVerifiedHttpApi(row))

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].Verified, "merge preserves probe-verified row without incoming probe evidence")

	row.Verified = false
	row.RejectReason = "retest"
	row.ProbeStatusCode = 404
	row.ProbeAttemptsJSON = `[{"status":404}]`
	require.NoError(t, repo.UpsertVerifiedHttpApi(row))
	rows, err = repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.False(t, rows[0].Verified)
}

func TestPhase1PrepBundle_WriteAndGate(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	dir := t.TempDir()
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodeRootPath: dir, CodePathOK: true, TargetReachable: true,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}

	bundle := &Phase1PrepBundle{Version: 1, SessionUUID: sess.UUID, Tasks: map[string]Phase1PrepTask{}}
	require.NoError(t, writePhase1PrepBundle(dir, bundle))

	cands := map[string]any{"candidates": []map[string]any{{"method": "GET", "path_pattern": "/x"}}, "count": 1}
	b, _ := json.MarshalIndent(cands, "", "  ")
	require.NoError(t, writeJSONFile(store.RouteCandidatesPath(dir), b))

	ep := &store.HttpEndpoint{SessionID: sess.ID, Method: "GET", PathPattern: "/x", Source: "test"}
	require.NoError(t, repo.CreateHttpEndpoint(ep))

	require.Error(t, verifyPhase1ApiVerificationGate(rt))

	vha := &store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/x",
		Verified: false, RejectReason: "no target",
		ProbeStatusCode: 404, ProbeAttemptsJSON: `[{"status":404}]`,
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(vha))
	require.NoError(t, writePhase1ContractStubArtifacts(dir))
	decision := &CoverageSignalDecision{Verdict: "finish", Reasoning: "contract test stub", SignalJSON: "{}"}
	decB, _ := json.MarshalIndent(decision, "", "  ")
	require.NoError(t, repo.UpsertPhaseArtifact(sess.ID, "coverage_signal_decision", string(decB)))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/a",
		Verified: true, FullSampleURL: "http://127.0.0.1:8080/a",
		ProbeStatusCode: 200, ProbeAttemptsJSON: `[{"status":200}]`,
	}))
	require.NoError(t, verifyPhase1ApiVerificationGate(rt))
}

func TestAutoMigrate_VerifiedHttpApi(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	require.True(t, repo.DB().HasTable(&store.VerifiedHttpApi{}))
}

func openTestRepoForPhase1(t *testing.T) (*store.Repository, func()) {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	return repo, func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
}
