package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCredentialGroupsFromUserText_MultiRealm(t *testing.T) {
	in := `code-path: /tmp/p
target: http://127.0.0.1:8080
admin_auth: admin1/pass1, admin2/pass2
user_auth: user1/test123; user2/test456
`
	groups := parseCredentialGroupsFromUserText(in)
	require.Len(t, groups, 2)
	require.Equal(t, "admin", groups[0].GroupID)
	require.Len(t, groups[0].Accounts, 2)
	require.Equal(t, "admin1", groups[0].Accounts[0].Username)
	require.Equal(t, "pass1", groups[0].Accounts[0].Password)
	require.Equal(t, "admin2", groups[0].Accounts[1].Username)
	require.Equal(t, "user", groups[1].GroupID)
	require.Len(t, groups[1].Accounts, 2)
}

func TestParseCredentialGroups_AuthGroupLine(t *testing.T) {
	in := "auth_group: api: app1/key1, app2/key2\n"
	groups := parseCredentialGroupsFromUserText(in)
	require.Len(t, groups, 1)
	require.Equal(t, "api", groups[0].GroupID)
	require.Len(t, groups[0].Accounts, 2)
}

func TestParseUserInput_LegacyAuthBecomesDefaultGroup(t *testing.T) {
	in := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\nauth: admin1/potian123\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Len(t, p.AuthCredentialGroups, 1)
	require.Equal(t, "default", p.AuthCredentialGroups[0].GroupID)
	require.Equal(t, "admin1", p.AuthCredentialGroups[0].Accounts[0].Username)
	u, pass := ResolveUserCredentials(p)
	require.Equal(t, "admin1", u)
	require.Equal(t, "potian123", pass)
}

func TestAccountsForAuthRealm(t *testing.T) {
	groups := []UserCredentialGroup{
		{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "a1", Password: "p1"}}},
		{GroupID: "user", Accounts: []UserCredentialAccount{{Username: "u1", Password: "p2"}, {Username: "u2", Password: "p3"}}},
	}
	admin := AccountsForAuthRealm(groups, "admin")
	require.Len(t, admin, 1)
	require.Equal(t, "a1", admin[0].Username)
	web := AccountsForAuthRealm(groups, "web")
	require.Len(t, web, 2)
	require.Equal(t, "u1", web[0].Username)
	require.Equal(t, "u2", web[1].Username)
}

func TestFormatCredentialGroupHintForRealm(t *testing.T) {
	groups := []UserCredentialGroup{
		{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "a1", Password: "p1"}}},
		{GroupID: "user", Accounts: []UserCredentialAccount{{Username: "u1", Password: "p2"}, {Username: "u2", Password: "p3"}}},
	}
	rt := &Runtime{UserAuthCredentialGroups: groups}

	adminHint := formatCredentialGroupHintForRealm(rt, AuthRealmAdmin)
	require.Contains(t, adminHint, "admin")
	require.Contains(t, adminHint, "a1")
	require.Contains(t, adminHint, "试登顺序")
	require.Contains(t, adminHint, "禁止")
	require.Contains(t, adminHint, "user")

	webHint := formatCredentialGroupHintForRealm(rt, AuthRealmWeb)
	require.Contains(t, webHint, "u1")
	require.Contains(t, webHint, "u2")
	require.NotContains(t, webHint, "username=\"a1\"")
}

func TestValidateFeatureVerifyEntry_AbsolutePath(t *testing.T) {
	require.Error(t, validateFeatureVerifyEntry(nil, &FeatureApiMapEntry{
		FeatureID: "x",
		Apis:      []FeatureApiEntry{{Method: "GET", PathPattern: "content"}},
	}))
}
