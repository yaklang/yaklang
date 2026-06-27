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

func TestDetectMultiAuth_FromLoginEndpoints(t *testing.T) {
	ev := &AuthEvidenceRecord{
		LoginEndpoints: []AuthLoginEndpoint{
			{Path: "/admin/login", AuthRealm: AuthRealmAdmin},
			{Path: "/login", AuthRealm: AuthRealmWeb},
		},
	}
	require.True(t, DetectMultiAuth(nil, ev))
}

func TestDetectMultiAuth_SingleRealm(t *testing.T) {
	ev := &AuthEvidenceRecord{
		LoginEndpoints: []AuthLoginEndpoint{{Path: "/admin/login", AuthRealm: AuthRealmAdmin}},
	}
	require.False(t, DetectMultiAuth(nil, ev))
}

func TestResolveCredentialIDForProbe_MatchesRealm(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}

	admin := &store.AuthCredential{
		SessionID: sess.ID, AuthType: "cookie_session", Verified: true,
		AuthRealm: AuthRealmAdmin, URLSpace: "admin_space", MountPrefix: "/admin",
		LoginPath: "/admin/login", HeadersJSON: `{"Cookie":"ADMIN=1"}`,
	}
	web := &store.AuthCredential{
		SessionID: sess.ID, AuthType: "cookie_session", Verified: true,
		AuthRealm: AuthRealmWeb, URLSpace: "public", MountPrefix: "/",
		LoginPath: "/login", HeadersJSON: `{"Cookie":"USER=1"}`,
	}
	require.NoError(t, repo.CreateAuthCredential(admin))
	require.NoError(t, repo.CreateAuthCredential(web))

	id, reason := ResolveCredentialIDForProbe(rt, "com.example.controller.admin.FooController", "admin_space", "/admin", "")
	require.Equal(t, admin.ID, id)
	require.Contains(t, reason, "matched")

	id2, _ := ResolveCredentialIDForProbe(rt, "com.example.controller.web.BarController", "public", "/", "")
	require.Equal(t, web.ID, id2)
}

func TestHasAuthCredentialsSatisfied_MultiAuth(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}

	ev := &AuthEvidenceRecord{
		MultiAuth: true,
		LoginEndpoints: []AuthLoginEndpoint{
			{Path: "/admin/login", AuthRealm: AuthRealmAdmin, ProbeAttempted: true},
			{Path: "/login", AuthRealm: AuthRealmWeb, ProbeAttempted: true},
		},
	}
	require.False(t, HasAuthCredentialsSatisfied(rt, ev))

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))
	require.False(t, HasAuthCredentialsSatisfied(rt, ev))

	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmWeb, HeadersJSON: `{"Cookie":"b=1"}`,
	}))
	require.True(t, HasAuthCredentialsSatisfied(rt, ev))
}

func TestDetectMultiAuthFromRouting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))
	rp := `{"schema_version":1,"url_spaces":[{"id":"admin","mount_prefix":"/admin"},{"id":"public","mount_prefix":"/"}]}`
	require.NoError(t, os.WriteFile(store.RoutingProfilePath(dir), []byte(rp), 0o644))
	rt := &Runtime{WorkDir: dir}
	require.True(t, detectMultiAuthFromRouting(rt))
}

func TestBuildProbeAuthSelectionHint_MultiAuth(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))
	ev := AuthEvidenceRecord{MultiAuth: true, LoginEndpoints: []AuthLoginEndpoint{
		{Path: "/admin/login", AuthRealm: AuthRealmAdmin},
		{Path: "/login", AuthRealm: AuthRealmWeb},
	}}
	b, err := json.MarshalIndent(ev, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.AuthEvidencePath(dir), b, 0o644))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin, HeadersJSON: `{"Cookie":"a=1"}`,
	}))
	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}

	hint := BuildProbeAuthSelectionHint(rt, "com.foo.controller.admin.X", "admin", "/admin", true)
	require.True(t, hint.MultiAuth)
	require.Equal(t, AuthRealmAdmin, hint.HandlerAuthRealm)
	require.NotEmpty(t, hint.AvailableCredentials)
}
