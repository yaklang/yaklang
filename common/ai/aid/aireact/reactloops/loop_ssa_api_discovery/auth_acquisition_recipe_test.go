package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestHeadersJSONToText(t *testing.T) {
	json := `{"Cookie":"PHPSESSID=abc123","X-CSRF-Token":"tok456"}`
	text := HeadersJSONToText(json)
	require.Contains(t, text, "Cookie: PHPSESSID=abc123")
	require.Contains(t, text, "X-CSRF-Token: tok456")
}

func TestHeadersJSONToText_Empty(t *testing.T) {
	require.Empty(t, HeadersJSONToText(""))
	require.Empty(t, HeadersJSONToText("{}"))
}

func TestHeadersTextToJSON(t *testing.T) {
	text := "Authorization: Bearer xyz\r\nX-API-Key: key123"
	jsonStr := HeadersTextToJSON(text)
	require.Contains(t, jsonStr, "Authorization")
	require.Contains(t, jsonStr, "Bearer xyz")
	require.Contains(t, jsonStr, "key123")
}

func TestHeadersTextToMap(t *testing.T) {
	text := "Cookie: session=abc\r\nX-Token: 123"
	m := HeadersTextToMap(text)
	require.Equal(t, "session=abc", m["Cookie"])
	require.Equal(t, "123", m["X-Token"])
}

func TestSyncCredentialHeaderFields_FromJSON(t *testing.T) {
	cred := &store.AuthCredential{
		HeadersJSON: `{"Authorization":"Bearer tok","X-Custom":"val"}`,
	}
	SyncCredentialHeaderFields(cred)
	require.NotEmpty(t, cred.HeadersText)
	require.NotEmpty(t, cred.HeaderName)
	require.NotEmpty(t, cred.HeaderValue)
}

func TestSyncCredentialHeaderFields_FromLegacy(t *testing.T) {
	cred := &store.AuthCredential{
		HeaderName:  "Authorization",
		HeaderValue: "Bearer token123",
	}
	SyncCredentialHeaderFields(cred)
	require.Contains(t, cred.HeadersJSON, "Authorization")
	require.Contains(t, cred.HeadersJSON, "Bearer token123")
	require.NotEmpty(t, cred.HeadersText)
}

func TestBuildAuthHeaderCLIArg(t *testing.T) {
	cred := &store.AuthCredential{
		HeaderName:  "Cookie",
		HeaderValue: "session=abc",
	}
	arg := BuildAuthHeaderCLIArg(cred)
	require.Equal(t, "Cookie: session=abc", arg)
}

func TestBuildAuthHeaderCLIArg_FromJSON(t *testing.T) {
	cred := &store.AuthCredential{
		HeadersJSON: `{"Authorization":"Bearer xyz"}`,
	}
	arg := BuildAuthHeaderCLIArg(cred)
	require.Equal(t, "Authorization: Bearer xyz", arg)
}

func TestBuildAuthHeaderCLIArg_Nil(t *testing.T) {
	require.Empty(t, BuildAuthHeaderCLIArg(nil))
}

func TestGetDefaultCredentialForSession_NilRepo(t *testing.T) {
	rt := &Runtime{}
	require.Nil(t, GetDefaultCredentialForSession(rt))
}
