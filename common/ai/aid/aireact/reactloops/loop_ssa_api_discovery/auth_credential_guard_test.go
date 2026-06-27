package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func testLoginPOSTAction(postParams string) *aicommon.Action {
	maker := aicommon.NewActionMaker("do_http_request")
	raw := `{"@action":"do_http_request","method":"POST","url":"http://127.0.0.1/admin/login","post-params":` + jsonString(postParams) + `}`
	return maker.ReadFromReader(context.Background(), strings.NewReader(raw))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestCheckLoginPOSTBlocked_WhenGroupSatisfied(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{
		Repo:    repo,
		Session: sess,
		UserAuthCredentialGroups: []UserCredentialGroup{
			{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "admin1"}}},
		},
	}
	require.NoError(t, repo.CreateAuthCredential(&store.AuthCredential{
		SessionID:         sess.ID,
		Verified:          true,
		AuthRealm:         AuthRealmAdmin,
		CredentialGroupID: "admin",
		Username:          "admin1",
		HeadersJSON:       `{"Cookie":"PUBLICCMS_ADMIN=1_abc-def-12345678"}`,
	}))

	action := testLoginPOSTAction("username=admin2&password=x")

	msg := checkLoginPOSTBlocked(rt, AuthRealmAdmin, action)
	require.Contains(t, msg, "login POST blocked")
	require.Contains(t, msg, "discovery_select_auth_credential")
}

func TestValidateSessionCookieHeaders_RejectsUsernameAsCookie(t *testing.T) {
	err := validateSessionCookieHeaders(`{"Cookie":"PUBLICCMS_ADMIN=admin1"}`, "admin1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "session token")
}

func TestValidateSessionCookieHeaders_AcceptsSessionToken(t *testing.T) {
	err := validateSessionCookieHeaders(`{"Cookie":"PUBLICCMS_ADMIN=1_c6ff098e-b02e-470e-a497-5fe151ff7a08"}`, "admin1")
	require.NoError(t, err)
}

func TestSupersedeOlderCredentialsInGroup(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}

	old := &store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		CredentialGroupID: "admin", HeadersJSON: `{"Cookie":"OLD=1"}`,
	}
	keep := &store.AuthCredential{
		SessionID: sess.ID, Verified: true, AuthRealm: AuthRealmAdmin,
		CredentialGroupID: "admin", HeadersJSON: `{"Cookie":"NEW=1"}`,
	}
	require.NoError(t, repo.CreateAuthCredential(old))
	require.NoError(t, repo.CreateAuthCredential(keep))

	supersedeOlderCredentialsInGroup(rt, AuthRealmAdmin, "admin", keep.ID)

	updated, err := repo.GetAuthCredential(sess.ID, old.ID)
	require.NoError(t, err)
	require.False(t, updated.Verified)
}

func TestEnforceVerifiedCredentialFromProbe_RejectsBadCookieWithoutProbe(t *testing.T) {
	row := &store.AuthCredential{
		Verified:    true,
		Username:    "admin1",
		HeadersJSON: `{"Cookie":"PUBLICCMS_ADMIN=admin1"}`,
	}
	blocked, msg := enforceVerifiedCredentialFromProbe(row, nil, nil)
	require.True(t, blocked)
	require.Contains(t, msg, "upsert rejected")
	require.False(t, row.Verified)
}
