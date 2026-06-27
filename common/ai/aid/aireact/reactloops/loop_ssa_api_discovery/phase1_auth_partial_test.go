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

func TestAuthGateSatisfied_PartialMode(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	surface := AuthSurfaceMapV1{
		SchemaVersion: 1,
		MultiAuth:     true,
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: AuthRealmAdmin, LoginPath: "/admin/login", PackagePatterns: []string{"*.controller.admin.*"}},
			{AuthRealm: AuthRealmAPI, LoginPath: "/api/login", PackagePatterns: []string{"*.controller.api.*"}},
		},
	}
	sb, _ := json.MarshalIndent(surface, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthSurfaceMapPath(dir), sb, 0o644))
	ev := authEvidenceFromSurfaceMap(&surface)
	eb, _ := json.MarshalIndent(ev, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), eb, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, AllowPartialAuth: true}
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))

	require.False(t, HasAuthCredentialsSatisfied(rt, ev))
	require.True(t, AuthGateSatisfied(rt, ev))
}

func TestEvaluatePhase1AuthCalibrationReadiness_PartialAllowsOneRealm(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	surface := AuthSurfaceMapV1{
		SchemaVersion: 1,
		MultiAuth:     true,
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: AuthRealmAdmin, LoginPath: "/admin/login", PackagePatterns: []string{"*.controller.admin.*"}},
			{AuthRealm: AuthRealmAPI, LoginPath: "/api/login", PackagePatterns: []string{"*.controller.api.*"}},
		},
	}
	sb, _ := json.MarshalIndent(surface, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthSurfaceMapPath(dir), sb, 0o644))

	cal := AuthCalibrationV1{
		SchemaVersion: 1,
		Realms: []AuthCalibrationRealm{
			{
				AuthRealm: AuthRealmAdmin, Calibrated: true,
				Probes: []AuthCalibrationProbe{
					{Method: "GET", Path: "/admin/index", Passed: true},
					{Method: "GET", Path: "/admin/foo", Passed: true},
				},
			},
		},
	}
	cb, _ := json.MarshalIndent(cal, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthCalibrationPath(dir), cb, 0o644))

	ev := authEvidenceFromSurfaceMap(&surface)
	eb, _ := json.MarshalIndent(ev, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), eb, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, AllowPartialAuth: true}
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))
	_, err := writeAuthState(rt, authStatePartial, "partial auth test")
	require.NoError(t, err)

	ready, reason := EvaluatePhase1AuthCalibrationReadiness(rt)
	require.True(t, ready, reason)
	require.Contains(t, reason, "partial")
}

func TestShouldSkipFeatureWorkForPartialAuth(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	surface := AuthSurfaceMapV1{
		SchemaVersion: 1,
		MultiAuth:     true,
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: AuthRealmAdmin, LoginPath: "/admin/login", PackagePatterns: []string{"*.controller.admin.*"}},
			{AuthRealm: AuthRealmAPI, LoginPath: "/api/login", PackagePatterns: []string{"*.controller.api.*"}},
		},
	}
	sb, _ := json.MarshalIndent(surface, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthSurfaceMapPath(dir), sb, 0o644))
	ev := authEvidenceFromSurfaceMap(&surface)
	eb, _ := json.MarshalIndent(ev, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), eb, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, AllowPartialAuth: true}
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))

	adminJob := FeatureWorkJob{
		EntryFile:       "src/com/foo/controller/admin/UserController.java",
		FeatureID:       "admin_users",
		PackagePatterns: []string{"*.controller.admin.*"},
		SurfaceKind:     SurfaceKindHTTPAPI,
	}
	apiJob := FeatureWorkJob{
		EntryFile:       "src/com/foo/controller/api/RestController.java",
		FeatureID:       "api_rest",
		PackagePatterns: []string{"*.controller.api.*"},
		SurfaceKind:     SurfaceKindHTTPAPI,
	}

	skip, _ := ShouldSkipFeatureWorkForPartialAuth(rt, adminJob)
	require.False(t, skip)
	skip, reason := ShouldSkipFeatureWorkForPartialAuth(rt, apiJob)
	require.True(t, skip)
	require.Contains(t, reason, rejectReasonAuthRealmUnavailable)
	require.Contains(t, reason, AuthRealmAPI)
}

func TestParseInput_AllowPartialAuth(t *testing.T) {
	p, err := extractUserInputFields("code_path: /tmp/x\npartial_auth: yes\ntarget: http://127.0.0.1")
	require.NoError(t, err)
	require.True(t, p.AllowPartialAuth)

	p2, err := extractUserInputFields("部分鉴权: 允许\ntarget: http://127.0.0.1")
	require.NoError(t, err)
	require.True(t, p2.AllowPartialAuth)
}

func TestCommitSkippedFeatureWorkForPartialAuth(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, AllowPartialAuth: true}

	job := FeatureWorkJob{
		EntryFile:   "src/ApiController.java",
		FeatureID:   "api_feat",
		FeatureLabel: "API",
		SurfaceKind: SurfaceKindHTTPAPI,
	}
	reason := rejectReasonAuthRealmUnavailable + ":api (partial_auth)"
	require.NoError(t, commitSkippedFeatureWorkForPartialAuth(rt, job, reason))

	apiMap, err := loadFeatureApiMap(dir)
	require.NoError(t, err)
	require.Len(t, apiMap.Features, 1)
	require.Equal(t, reason, apiMap.Features[0].NoApiReason)
	require.True(t, apiMap.Features[0].Processed)
}
