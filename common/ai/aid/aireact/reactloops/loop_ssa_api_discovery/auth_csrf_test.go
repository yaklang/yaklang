package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestExtractCsrfFromHTML_HiddenInput(t *testing.T) {
	html := `<form><input type="hidden" name="_csrf" value="abc-123-token"/></form>`
	pn, tok := extractCsrfFromHTML(html)
	require.Equal(t, "_csrf", pn)
	require.Equal(t, "abc-123-token", tok)
}

func TestApplyCsrfTokenToHTTPParams_PostAndQuery(t *testing.T) {
	params := aitool.InvokeParams{"method": "POST"}
	notes := applyCsrfTokenToHTTPParams(params, "_csrf", "tok999")
	require.NotEmpty(t, notes)
	post, _ := params["post-params"].(string)
	require.Contains(t, post, "_csrf=tok999")
	q, _ := params["query-params"].(string)
	require.NotContains(t, q, "_csrf=")
}

func TestApplyCsrfTokenToHTTPParams_GetQueryOnly(t *testing.T) {
	params := aitool.InvokeParams{"method": "GET"}
	notes := applyCsrfTokenToHTTPParams(params, "_csrf", "tok999")
	require.NotEmpty(t, notes)
	q, _ := params["query-params"].(string)
	require.Contains(t, q, "_csrf=tok999")
	post, _ := params["post-params"].(string)
	require.NotContains(t, post, "_csrf=")
}

func TestExtractCsrfTokenFromPublicCMSAdminCookie(t *testing.T) {
	headersJSON := `{"Cookie":"JSESSIONID=abc; PUBLICCMS_ADMIN=1_df120bcc-bba3-469e-873c-2da87253efe7"}`
	pn, tok, ok := extractCsrfTokenFromPublicCMSAdminCookie(headersJSON)
	require.True(t, ok)
	require.Equal(t, "_csrf", pn)
	require.Equal(t, "df120bcc-bba3-469e-873c-2da87253efe7", tok)
}

func TestStripManualCsrfFromParams(t *testing.T) {
	params := aitool.InvokeParams{
		"method":       "POST",
		"post-params":  "a=1&_csrf=wrong",
		"query-params": "_csrf=wrong",
	}
	notes := stripManualCsrfFromParams(params, "_csrf")
	require.NotEmpty(t, notes)
	post, _ := params["post-params"].(string)
	require.NotContains(t, post, "_csrf")
	q, _ := params["query-params"].(string)
	require.NotContains(t, q, "_csrf")
	require.Contains(t, post, "a=1")
}

func TestApplyCachedCsrfForCredentialIfRequired_SkipsUnlessRequired(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, store.SubDirName()), 0o755))
	require.NoError(t, writeJSONFile(store.AuthCsrfTokensPath(dir), []byte(`{"schema_version":1,"entries":[{"credential_id":1,"param_name":"_csrf","token":"savedtok"}]}`)))

	rt := &Runtime{WorkDir: dir}
	params := aitool.InvokeParams{"method": "GET", "url": "http://127.0.0.1/admin/cmsContent/list"}
	notes := applyCachedCsrfForCredentialIfRequired(rt, 1, params, false)
	require.Empty(t, notes)
	q, _ := params["query-params"].(string)
	require.NotContains(t, q, "_csrf=")

	notes = applyCachedCsrfForCredentialIfRequired(rt, 1, params, true)
	require.NotEmpty(t, notes)
	q, _ = params["query-params"].(string)
	require.Contains(t, q, "_csrf=savedtok")
}

func TestRequiresCsrfForHTTPParams_FromCatalog(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	catalog := &CombinedAPICatalog{
		SchemaVersion: 1,
		Records: []CombinedAPIRecord{
			{Method: "GET", Path: "/admin/cmsContent/list"},
			{Method: "GET", Path: "/admin/sysSite/delete", Auth: CombinedAPIAuth{Mechanisms: []string{"csrf_token"}}},
			{Method: "POST", Path: "/admin/cmsContent/save", Auth: CombinedAPIAuth{Mechanisms: []string{"csrf_token"}}},
		},
	}
	b, err := json.MarshalIndent(catalog, "", "  ")
	require.NoError(t, err)
	require.NoError(t, writeJSONFile(store.CombinedAPICatalogPath(dir), b))

	rt := &Runtime{WorkDir: dir}
	require.False(t, requiresCsrfForHTTPParams(rt, aitool.InvokeParams{
		"method": "GET",
		"url":    "http://192.168.1.4:8080/admin/cmsContent/list",
	}))
	require.True(t, requiresCsrfForHTTPParams(rt, aitool.InvokeParams{
		"method": "GET",
		"url":    "http://192.168.1.4:8080/admin/sysSite/delete?id=1",
	}))
	require.True(t, requiresCsrfForHTTPParams(rt, aitool.InvokeParams{
		"method": "POST",
		"url":    "http://192.168.1.4:8080/admin/cmsContent/save",
	}))
}

func TestProbeResultFromBulkVerify_Csrf404NotNotFound(t *testing.T) {
	rt := &Runtime{Session: &store.DiscoverySession{TargetScheme: "http", TargetHost: "127.0.0.1", TargetPort: "8080"}}
	rec := CombinedAPIRecord{
		Method: "GET",
		Path:   "/admin/sysSite/delete",
		Auth:   CombinedAPIAuth{Mechanisms: []string{"csrf_token"}},
	}
	pr := probeResultFromBulkVerify(rt, rec, "", 404, 1, "test")
	require.False(t, pr.Verified)
	require.Equal(t, "csrf_required", pr.RejectReason)
	require.Contains(t, pr.VerdictReason, "@Csrf")
}

func TestApplyCachedCsrfForCredential(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, store.SubDirName()), 0o755))
	require.NoError(t, writeJSONFile(store.AuthCsrfTokensPath(dir), []byte(`{"schema_version":1,"entries":[{"credential_id":1,"param_name":"_csrf","token":"savedtok"}]}`)))

	rt := &Runtime{WorkDir: dir}
	params := aitool.InvokeParams{"method": "GET"}
	notes := applyCachedCsrfForCredential(rt, 1, params)
	require.NotEmpty(t, notes)
	q, _ := params["query-params"].(string)
	require.Contains(t, q, "_csrf=savedtok")
}

func TestCaptureCsrfFromHTTPResponse_Persists(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{WorkDir: dir}
	cred := &store.AuthCredential{ID: 5, AuthRealm: "admin"}
	html := `<html><body><input name="_csrf" value="persist-me"/></body></html>`
	msg, err := captureCsrfFromHTTPResponse(rt, cred, "http://127.0.0.1/admin/", html)
	require.NoError(t, err)
	require.Contains(t, msg, "csrf_auto_capture")
	_, tok, ok := getCsrfTokenForCredential(dir, 5)
	require.True(t, ok)
	require.Equal(t, "persist-me", tok)
}

func TestPrefetchCsrfTokensForSession(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{
		UUID:            "csrf-prefetch-test",
		TargetReachable: true,
		TargetScheme:    "http",
		TargetHost:      "127.0.0.1",
		TargetPort:      "8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	cred := &store.AuthCredential{
		SessionID:   sess.ID,
		AuthType:    "cookie_session",
		AuthRealm:   AuthRealmAdmin,
		Username:    "admin",
		Verified:    true,
		HeadersJSON: `{"Cookie":"PUBLICCMS_ADMIN=abc; JSESSIONID=s1"}`,
	}
	require.NoError(t, repo.CreateAuthCredential(cred))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(_ context.Context, toolName string, _ aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		require.Equal(t, "do_http_request", toolName)
		html := `HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<input type="hidden" name="_csrf" value="prefetched-token"/>`
		return &aitool.ToolResult{
			Success: true,
			Data:    &aitool.ToolExecutionResult{Stdout: html},
		}, false, nil
	}
	n, warns := PrefetchCsrfTokensForSession(context.Background(), inv, rt)
	require.Empty(t, warns)
	require.Equal(t, 1, n)
	_, tok, ok := getCsrfTokenForCredential(dir, cred.ID)
	require.True(t, ok)
	require.Equal(t, "prefetched-token", tok)
}
