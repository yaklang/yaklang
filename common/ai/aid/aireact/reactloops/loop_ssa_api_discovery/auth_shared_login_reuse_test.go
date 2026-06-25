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

func TestSharedLoginReuse_CreatesAPIAliasFromAdminCredential(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	ev0 := AuthEvidenceRecord{
		MultiAuth: true,
		LoginEndpoints: []AuthLoginEndpoint{
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAdmin, MountPrefix: "/admin"},
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAPI, MountPrefix: "/"},
		},
	}
	b, _ := json.MarshalIndent(ev0, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), b, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID:   sess.ID,
		Verified:    true,
		AuthRealm:   AuthRealmAdmin,
		MountPrefix: "/admin",
		LoginPath:   "/admin/login",
		HeadersJSON: `{"Cookie":"PUBLICCMS_ADMIN=1_test"}`,
	}))

	require.NoError(t, RefreshAuthEvidenceFromDB(rt))

	ev, err := loadAuthEvidenceFromWorkDir(dir)
	require.NoError(t, err)
	require.True(t, ev.Verified, "shared login should satisfy api realm")
	require.Contains(t, ev.VerificationDetail, "all 2 required auth realm")

	rows, err := repo.ListAuthCredentials(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	var apiCred *store.AuthCredential
	for i := range rows {
		if NormalizeAuthRealm(rows[i].AuthRealm) == AuthRealmAPI {
			apiCred = &rows[i]
		}
	}
	require.NotNil(t, apiCred)
	require.True(t, apiCred.Verified)
	require.Contains(t, apiCred.Notes, "shared_login_reuse")
	require.Equal(t, `{"Cookie":"PUBLICCMS_ADMIN=1_test"}`, apiCred.HeadersJSON)
}

func TestRealmSatisfiedViaSharedLogin_BeforeAliasCreation(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		LoginPath: "/admin/login", HeadersJSON: `{"Cookie":"A=1"}`,
	}))

	ev := &AuthEvidenceRecord{
		LoginEndpoints: []AuthLoginEndpoint{
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAdmin},
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAPI},
		},
	}
	require.True(t, realmSatisfiedViaSharedLogin(rt, ev, AuthRealmAPI))
	require.False(t, hasDirectVerifiedCredentialForRealm(rt, AuthRealmAPI))
}

func TestEnsureSharedLoginCredentialAliases_Idempotent(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		LoginPath: "/admin/login", HeadersJSON: `{"Cookie":"A=1"}`,
	}))

	ev := &AuthEvidenceRecord{
		LoginEndpoints: []AuthLoginEndpoint{
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAdmin},
			{Method: "POST", Path: "/admin/login", AuthRealm: AuthRealmAPI},
		},
	}
	n1 := EnsureSharedLoginCredentialAliases(rt, ev)
	n2 := EnsureSharedLoginCredentialAliases(rt, ev)
	require.Equal(t, 1, n1)
	require.Equal(t, 0, n2)
}
