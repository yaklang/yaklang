package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const integrationPublicCMSRoot = "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/real-cms/PublicCMS/publiccms-parent"

// publicCMSMockServer simulates login + CSRF + route probing for toolkit integration tests.
type publicCMSMockServer struct {
	csrfToken     string
	loggedIn      bool
	loginCookie   string
	csrfFetchHits int
	probeStats    struct {
		getOK, postOK, postCSRF403, other int
	}
}

func (m *publicCMSMockServer) handle(toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	if toolName != "do_http_request" {
		return &aitool.ToolResult{Success: true, Data: &aitool.ToolExecutionResult{Stdout: "ok"}}, false, nil
	}
	method := strings.ToUpper(strings.TrimSpace(fmt.Sprint(params["method"])))
	rawURL := strings.TrimSpace(fmt.Sprint(params["url"]))
	u, _ := url.Parse(rawURL)
	path := u.Path
	if path == "" {
		path = "/"
	}

	if method == "POST" && strings.Contains(path, "/admin/login") {
		form := pickStringParam(params, "post-params", "body")
		if strings.Contains(form, "username=") && strings.Contains(form, "password=") && strings.Contains(form, "encoding=sha512") {
			m.loggedIn = true
			m.loginCookie = fmt.Sprintf("PUBLICCMS_ADMIN=1_%s; JSESSIONID=mock-jsession", m.csrfToken)
			return &aitool.ToolResult{
				Success: true,
				Data: &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 302 Found\r\nLocation: /admin/\r\nSet-Cookie: PUBLICCMS_ADMIN=1_" + m.csrfToken + "; Path=/\r\nSet-Cookie: JSESSIONID=mock-jsession; Path=/\r\n\r\n"},
			}, false, nil
		}
		return &aitool.ToolResult{
			Success: true,
			Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 200 OK\r\n\r\nlogin failed"},
		}, false, nil
	}

	if method == "GET" && (path == "/admin/" || path == "/admin/index" || strings.HasPrefix(path, "/admin/login")) {
		m.csrfFetchHits++
		if !m.loggedIn {
			return &aitool.ToolResult{
				Success: true,
				Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 302 Found\r\nLocation: /admin/login\r\n\r\n"},
			}, false, nil
		}
		html := fmt.Sprintf(`HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<input type="hidden" name="_csrf" value="%s"/>`, m.csrfToken)
		return &aitool.ToolResult{Success: true, Data: &aitool.ToolExecutionResult{Stdout: html}}, false, nil
	}

	if strings.HasPrefix(path, "/admin/") {
		if !m.loggedIn {
			return &aitool.ToolResult{
				Success: true,
				Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 401 Unauthorized\r\n\r\n"},
			}, false, nil
		}
		if method == "GET" {
			q := u.Query().Get("_csrf")
			if q == "" {
				q, _ = params["query-params"].(string)
				if strings.Contains(q, "_csrf=") {
					if vals, err := url.ParseQuery(q); err == nil {
						q = vals.Get("_csrf")
					}
				} else {
					q = ""
				}
			}
			if q != "" && q != m.csrfToken {
				return &aitool.ToolResult{
					Success: true,
					Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 404 Not Found\r\n\r\n"},
				}, false, nil
			}
			if q == m.csrfToken {
				m.probeStats.postOK++
			} else {
				m.probeStats.getOK++
			}
			return &aitool.ToolResult{
				Success: true,
				Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\nok"},
			}, false, nil
		}
		post := pickStringParam(params, "post-params", "body")
		if strings.Contains(post, "_csrf="+url.QueryEscape(m.csrfToken)) || strings.Contains(post, "_csrf="+m.csrfToken) {
			m.probeStats.postOK++
			return &aitool.ToolResult{
				Success: true,
				Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 200 OK\r\n\r\nsaved"},
			}, false, nil
		}
		if method == "POST" || method == "PUT" || method == "DELETE" {
			m.probeStats.postCSRF403++
			return &aitool.ToolResult{
				Success: true,
				Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 403 Forbidden\r\n\r\ncsrf token invalid or missing"},
			}, false, nil
		}
	}
	m.probeStats.other++
	return &aitool.ToolResult{
		Success: true,
		Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 404 Not Found\r\n\r\n"},
	}, false, nil
}

func TestFrameworkToolkitIntegration_PublicCMSCatalog(t *testing.T) {
	if _, err := os.Stat(integrationPublicCMSRoot); err != nil {
		t.Skip("PublicCMS benchmark not available: ", integrationPublicCMSRoot)
	}

	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	repo, cleanup := openIntegrationTestRepo(t, dir)
	defer cleanup()

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
		CodeRootPath:    integrationPublicCMSRoot,
		CodePathOK:      true,
		Language:        "java",
		TargetReachable: true,
		TargetScheme:    "http",
		TargetHost:      "192.168.1.4",
		TargetPort:      "8080",
		TargetRaw:       "http://192.168.1.4:8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{
		Repo:                repo,
		Session:             sess,
		WorkDir:             workDir,
		UserAuthUsername:    "admin",
		UserAuthPassword:    "Admin@2024!",
		FrameworkToolkitEnabled: true,
	}

	// Extract real catalog from PublicCMS source.
	catalog, err := RunCombinedProgrammaticAPIExtraction(rt)
	require.NoError(t, err)
	require.Greater(t, catalog.Stats.Total, 50)
	t.Logf("catalog total=%d csrf=%d backend_only=%d merged=%d",
		catalog.Stats.Total, catalog.Stats.WithCsrf, catalog.Stats.BackendOnly, catalog.Stats.MergedBoth)

	// Limit probe set for integration runtime (still representative).
	var probeCatalog CombinedAPICatalog
	probeCatalog.Records = pickRepresentativeRecords(catalog, 40)
	t.Logf("probe sample=%d (GET + CSRF POST mix)", len(probeCatalog.Records))

	mock := &publicCMSMockServer{csrfToken: "integration-csrf-token-abc123"}
	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(_ context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		return mock.handle(toolName, params)
	}

	ctx := context.Background()
	require.NoError(t, acquirePublicCMSCredentials(ctx, inv, rt))
	require.True(t, hasVerifiedAuthCredential(rt))

	creds, err := repo.ListAuthCredentials(sess.ID)
	require.NoError(t, err)
	require.NotEmpty(t, creds)
	_, tok, ok := getCsrfTokenForCredential(workDir, creds[0].ID)
	require.True(t, ok, "csrf should be prefetched during acquirePublicCMSCredentials")
	require.Equal(t, mock.csrfToken, tok)

	report, err := runFrameworkToolkitBulkVerify(ctx, inv, rt, &probeCatalog, "integration_test")
	require.NoError(t, err)
	require.Greater(t, report.Probed, 0)

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)

	var verified, csrfReject, otherReject int
	for _, row := range rows {
		if row.Verified {
			verified++
		} else if row.RejectReason == "csrf_required" {
			csrfReject++
			t.Logf("csrf_required: method=%s path=%s status=%d", row.Method, row.PathPattern, row.ProbeStatusCode)
		} else if row.RejectReason != "" {
			otherReject++
		}
	}

	t.Logf("bulk_verify report: probed=%d verified=%d rejected=%d destructive_skip=%d errors=%d",
		report.Probed, report.Verified, report.Rejected, report.DestructiveSkip, report.Errors)
	t.Logf("verified_http_apis: verified=%d csrf_required=%d other_reject=%d total_rows=%d",
		verified, csrfReject, otherReject, len(rows))
	t.Logf("mock HTTP: get_ok=%d post_ok=%d post_csrf403=%d csrf_prefetch_hits=%d",
		mock.probeStats.getOK, mock.probeStats.postOK, mock.probeStats.postCSRF403, mock.csrfFetchHits)

	require.Greater(t, verified, 5, "expected meaningful verified count with CSRF prefetch")
	require.Equal(t, 0, csrfReject, "CSRF prefetch should eliminate csrf_required rejects in integration mock")
	require.Greater(t, mock.probeStats.postOK, 0, "expected some CSRF POST probes to succeed")
}

func openIntegrationTestRepo(t *testing.T, dir string) (*store.Repository, func()) {
	t.Helper()
	dbPath := filepath.Join(dir, "integration.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	return repo, func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
}

func pickRepresentativeRecords(catalog *CombinedAPICatalog, max int) []CombinedAPIRecord {
	if catalog == nil || max <= 0 {
		return nil
	}
	var out []CombinedAPIRecord
	add := func(rec CombinedAPIRecord) {
		if len(out) >= max {
			return
		}
		if !strings.HasPrefix(rec.Path, "/admin/") {
			return
		}
		for _, existing := range out {
			if existing.Method == rec.Method && existing.Path == rec.Path {
				return
			}
		}
		out = append(out, rec)
	}
	for _, rec := range catalog.Records {
		if rec.RequiresCsrf() && rec.Method == "POST" {
			add(rec)
		}
	}
	for _, rec := range catalog.Records {
		if rec.RequiresCsrf() && rec.Method == "GET" {
			add(rec)
		}
	}
	for _, rec := range catalog.Records {
		if rec.Method == "GET" && strings.HasPrefix(rec.Path, "/admin/") {
			add(rec)
		}
	}
	for _, rec := range catalog.Records {
		add(rec)
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}
