package loop_ssa_api_discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

const taskFakeSHA512 = "3c5e7a5b0f8d9e2a1c4b6d8f0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b8d0e2a4c6b"

func TestIsValidHexCredentialOutput_RejectsTaskFakeHash(t *testing.T) {
	require.False(t, isValidHexCredentialOutput(taskFakeSHA512, "sha512"))
}

func TestIsValidHexCredentialOutput_AcceptsRealSHA512(t *testing.T) {
	res, err := transformCredentialGoParams("sha512", "potian123", "", "", "", "", false)
	require.NoError(t, err)
	require.True(t, isValidHexCredentialOutput(res.Output, "sha512"))
}

func TestCheckLoginPasswordTransformBlocked_RejectsHandwrittenHash(t *testing.T) {
	rt := &Runtime{
		UserAuthCredentialGroups: []UserCredentialGroup{
			{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "admin", Password: "potian123"}}},
		},
	}
	action := testLoginPOSTActionWithPassword("admin", taskFakeSHA512)
	msg := checkLoginPasswordTransformBlocked(rt, nil, AuthRealmAdmin, action)
	require.Contains(t, msg, "login POST blocked")
}

func TestCheckLoginPasswordTransformBlocked_RequiresTransformCredentialCall(t *testing.T) {
	res, err := transformCredentialGoParams("sha512", "potian123", "", "", "", "", false)
	require.NoError(t, err)

	rt := &Runtime{
		UserAuthCredentialGroups: []UserCredentialGroup{
			{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "admin", Password: "potian123"}}},
		},
	}
	action := testLoginPOSTActionWithPassword("admin", res.Output)
	msg := checkLoginPasswordTransformBlocked(rt, nil, AuthRealmAdmin, action)
	require.Contains(t, msg, "transform_credential")
}

func TestCheckLoginPasswordTransformBlocked_AllowsAfterTransformCredential(t *testing.T) {
	res, err := transformCredentialGoParams("sha512", "potian123", "", "", "", "", false)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("test-transform-guard", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)
	recordCredentialTransform(loop, res)

	rt := &Runtime{
		UserAuthCredentialGroups: []UserCredentialGroup{
			{GroupID: "admin", Accounts: []UserCredentialAccount{{Username: "admin", Password: "potian123"}}},
		},
	}
	action := testLoginPOSTActionWithPassword("admin", res.Output)
	msg := checkLoginPasswordTransformBlocked(rt, loop, AuthRealmAdmin, action)
	require.Empty(t, msg)
}

func testLoginPOSTActionWithPassword(username, passwordHash string) *aicommon.Action {
	maker := aicommon.NewActionMaker("do_http_request")
	postParams := "username=" + username +
		"&password=" + passwordHash +
		"&secureLogin=true&returnUrl=&encoding=sha512"
	raw := `{"@action":"do_http_request","method":"POST","url":"http://127.0.0.1/admin/login","post-params":` + jsonString(postParams) + `}`
	return maker.ReadFromReader(context.Background(), strings.NewReader(raw))
}

func TestRecordCredentialTransform_PersistsOnLoop(t *testing.T) {
	loop, err := reactloops.NewReActLoop("test-transform-cache", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)
	res, err := transformCredentialGoParams("sha512", "test123", "", "", "", "", false)
	require.NoError(t, err)
	recordCredentialTransform(loop, res)
	got := lookupStoredCredentialTransform(loop, "sha512", "test123")
	require.Equal(t, res.Output, got)
}
