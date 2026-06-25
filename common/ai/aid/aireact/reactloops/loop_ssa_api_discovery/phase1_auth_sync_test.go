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

func TestRefreshAuthEvidenceFromDB_BindingsAndVerified(t *testing.T) {
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

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		HeadersJSON: `{"Cookie":"ADMIN=1"}`, LoginPath: "/admin/login",
	}))
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmWeb,
		HeadersJSON: `{"Cookie":"WEB=1"}`, LoginPath: "/login",
	}))

	require.NoError(t, RefreshAuthEvidenceFromDB(rt))

	ev, err := loadAuthEvidenceFromWorkDir(dir)
	require.NoError(t, err)
	require.True(t, ev.Verified)
	require.Len(t, ev.CredentialBindings, 2)
	require.True(t, ev.MultiAuth)
}

func TestRefreshAuthEvidenceFromDB_PartialMultiAuth(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	ev0 := AuthEvidenceRecord{
		MultiAuth: true,
		LoginEndpoints: []AuthLoginEndpoint{
			{AuthRealm: AuthRealmAdmin, Path: "/admin/login"},
			{AuthRealm: AuthRealmAPI, Path: "/api/auth/login"},
		},
	}
	b, _ := json.MarshalIndent(ev0, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), b, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		HeadersJSON: `{"Cookie":"A=1"}`,
	}))

	require.NoError(t, RefreshAuthEvidenceFromDB(rt))
	ev, err := loadAuthEvidenceFromWorkDir(dir)
	require.NoError(t, err)
	require.False(t, ev.Verified)
	require.Len(t, ev.CredentialBindings, 1)
	require.Contains(t, ev.VerificationDetail, "partial")
}

func TestPersistAuthMechanismDetail_MergesRealms(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))
	rt := &Runtime{WorkDir: dir}

	require.NoError(t, persistAuthMechanismDetail(rt, &AuthMechanismDetailV1{
		AuthRealm: AuthRealmAdmin, LoginPath: "/admin/login", CodeEvidence: []string{"AdminLogin.java"},
	}))
	require.NoError(t, persistAuthMechanismDetail(rt, &AuthMechanismDetailV1{
		AuthRealm: AuthRealmWeb, LoginPath: "/login", CodeEvidence: []string{"WebLogin.java"},
	}))

	m, err := loadAuthMechanismMap(dir)
	require.NoError(t, err)
	require.Len(t, m.Realms, 2)
	require.Equal(t, "/admin/login", m.Realms[AuthRealmAdmin].LoginPath)
}
