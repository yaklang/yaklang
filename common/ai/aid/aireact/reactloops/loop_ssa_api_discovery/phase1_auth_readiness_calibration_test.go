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

func TestEvaluatePhase1AuthCalibrationReadiness_RequiresAllRealms(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	surface := AuthSurfaceMapV1{
		SchemaVersion: 1,
		MultiAuth:     true,
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: AuthRealmAdmin, LoginPath: "/admin/login", LoginMethod: "POST"},
			{AuthRealm: AuthRealmWeb, LoginPath: "/login", LoginMethod: "POST"},
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
					{Method: "POST", Path: "/admin/foo", Passed: true},
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
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))

	ready, reason := EvaluatePhase1AuthCalibrationReadiness(rt)
	require.False(t, ready)
	require.NotEmpty(t, reason)
}

func TestAuthPartialOkEnabled(t *testing.T) {
	t.Setenv("YAK_SSA_AUTH_PARTIAL_OK", "")
	require.False(t, authPartialOkEnabled())
	t.Setenv("YAK_SSA_AUTH_PARTIAL_OK", "1")
	require.True(t, authPartialOkEnabled())
}

func TestShouldSkipDirectoryAnalysis(t *testing.T) {
	t.Setenv("YAK_SSA_SKIP_DIR_ANALYSIS", "")
	require.False(t, shouldSkipDirectoryAnalysis(nil))
	require.True(t, shouldSkipDirectoryAnalysis(&Runtime{SkipDirectoryAnalysis: true}))
	t.Setenv("YAK_SSA_SKIP_DIR_ANALYSIS", "1")
	require.True(t, shouldSkipDirectoryAnalysis(nil))
}
