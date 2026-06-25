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

func TestEvaluatePhase1AuthReadiness_Success(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	profile := &ProjectProfileV1{
		EntryPoints: []ProjectEntryPoint{{Kind: "security_config", RelPath: "SecurityConfig.java"}},
	}
	b, _ := json.MarshalIndent(profile, "", "  ")
	require.NoError(t, os.WriteFile(store.ProjectProfilePath(workDir), b, 0o644))

	_, err = writeAuthState(rt, authStateSuccess, "login ok")
	require.NoError(t, err)
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, AuthType: "cookie_session", Verified: true,
		HeadersJSON: `{"Cookie":"JSESSIONID=abc"}`,
	}))

	ready, reason := EvaluatePhase1AuthReadiness(rt)
	require.True(t, ready, reason)
}

func TestEvaluatePhase1AuthReadiness_FailedWithoutCredential(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess, UserAuthPassword: "secret"}

	_, err = writeAuthState(rt, authStateFailed, "login rejected")
	require.NoError(t, err)

	ready, reason := EvaluatePhase1AuthReadiness(rt)
	require.False(t, ready)
	require.Contains(t, reason, "auth_state")
}

func TestMergeFullVerifyPlanAfterAuth_IncludesStaticHints(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	// Auth-only plan (4 login routes scenario)
	loginPlan := &CodeReadingPlan{
		DiscoveredAPIs: []DiscoveredAPI{
			{Method: "POST", PathPattern: "/admin/login.do", CodeEvidence: "auth"},
		},
	}
	require.NoError(t, PersistCodeReadingPlan(rt, loginPlan))

	hints := StaticRouteHintsReport{
		Hints: []StaticRouteHint{
			{Method: "GET", PathPattern: "/api/user/list", FileRelPath: "UserController.java", Source: "static"},
			{Method: "POST", PathPattern: "/api/user/save", FileRelPath: "UserController.java", Source: "static"},
		},
	}
	hb, _ := json.MarshalIndent(hints, "", "  ")
	require.NoError(t, os.WriteFile(store.StaticRouteHintsPath(workDir), hb, 0o644))

	require.NoError(t, MergeFullVerifyPlanAfterAuth(rt))
	plan, err := LoadCodeReadingPlan(workDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(plan.DiscoveredAPIs), 3)
}

func TestWritePhase1AuthFailureReport_Structured(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	ev := &AuthEvidenceRecord{
		Verified:           false,
		VerificationDetail: "sha512 mismatch",
		LoginEndpoints: []AuthLoginEndpoint{
			{Method: "POST", Path: "/admin/login", ContentType: "application/x-www-form-urlencoded", ProbeAttempted: true},
		},
	}
	eb, _ := json.MarshalIndent(ev, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(workDir), eb, 0o644))
	_, _ = writeAuthState(rt, authStateFailed, "login failed")

	require.NoError(t, WritePhase1AuthFailureReport(rt, "auth_state is not success"))
	require.FileExists(t, store.Phase1AuthFailureReportPath(workDir))
	require.FileExists(t, store.Phase1AuthFailureReportMDPath(workDir))

	art, err := repo.GetPhaseArtifact(sess.ID, store.ArtifactPhase1AuthFailure)
	require.NoError(t, err)
	require.Contains(t, art.PayloadJSON, "sha512 mismatch")
}

func TestIsPhase1AuthFailed(t *testing.T) {
	require.True(t, IsPhase1AuthFailed(&Phase1AuthFailedError{Reason: "x"}))
	require.False(t, IsPhase1AuthFailed(nil))
}
