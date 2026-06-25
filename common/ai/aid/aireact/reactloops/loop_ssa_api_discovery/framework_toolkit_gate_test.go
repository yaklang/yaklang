package loop_ssa_api_discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestFrameworkToolkitBulkVerify_MockHTTP(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
		TargetReachable: true,
		TargetScheme:    "http",
		TargetHost:      "127.0.0.1",
		TargetPort:      "8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	catalog := &CombinedAPICatalog{
		Records: []CombinedAPIRecord{
			{Method: "GET", Path: "/admin/cmsCategory/list", HandlerClass: "CmsCategoryAdminController", Confidence: "high"},
		},
	}
	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		return &aitool.ToolResult{
			Success: true,
			Data:    &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\nok"},
		}, false, nil
	}
	report, err := runFrameworkToolkitBulkVerify(context.Background(), inv, rt, catalog, "test")
	require.NoError(t, err)
	require.Equal(t, 1, report.Probed)
	require.Equal(t, 1, report.Verified)

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].Verified)
	require.True(t, store.VerifiedHttpApiHasProbeEvidence(&rows[0]))
}

func TestFrameworkToolkitBulkVerify_GetCsrfMutationUsesQuery(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
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
		HeadersJSON: `{"Cookie":"PUBLICCMS_ADMIN=1_csrf-delete-token; JSESSIONID=s1"}`,
	}
	require.NoError(t, repo.CreateAuthCredential(cred))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	catalog := &CombinedAPICatalog{
		Records: []CombinedAPIRecord{
			{
				Method:       "GET",
				Path:         "/admin/sysSite/delete",
				HandlerClass: "SysSiteAdminController",
				Confidence:   "high",
				Auth:         CombinedAPIAuth{Required: true, Realm: AuthRealmAdmin, Mechanisms: []string{"csrf_token"}},
				Params:       []CombinedAPIParam{{Name: "id", Location: "query"}},
			},
		},
	}

	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(_ context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		require.Equal(t, "do_http_request", toolName)
		method, _ := params["method"].(string)
		require.Equal(t, "GET", method)
		q, _ := params["query-params"].(string)
		require.Contains(t, q, "_csrf=csrf-delete-token")
		return &aitool.ToolResult{Success: true, Data: &aitool.ToolExecutionResult{Stdout: `HTTP/1.1 200 OK\r\n\r\n{"statusCode":"200"}`}}, false, nil
	}

	report, err := runFrameworkToolkitBulkVerify(context.Background(), inv, rt, catalog, "test")
	require.NoError(t, err)
	require.Equal(t, 1, report.Verified)
}

func TestFrameworkToolkitBulkVerify_WithPrefetchedCSRF(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
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
		HeadersJSON: `{"Cookie":"PUBLICCMS_ADMIN=abc"}`,
	}
	require.NoError(t, repo.CreateAuthCredential(cred))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir}

	catalog := &CombinedAPICatalog{
		Records: []CombinedAPIRecord{
			{
				Method:       "POST",
				Path:         "/admin/cmsCategory/save",
				HandlerClass: "CmsCategoryAdminController",
				Confidence:   "high",
				Auth:         CombinedAPIAuth{Required: true, Realm: AuthRealmAdmin, Mechanisms: []string{"csrf_token"}},
				Params:       []CombinedAPIParam{{Name: "pageIndex", Location: "post"}},
			},
		},
	}

	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(_ context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		require.Equal(t, "do_http_request", toolName)
		method, _ := params["method"].(string)
		if method == "GET" {
			html := `HTTP/1.1 200 OK\r\n\r\n<input type="hidden" name="_csrf" value="csrf-save-token"/>`
			return &aitool.ToolResult{Success: true, Data: &aitool.ToolExecutionResult{Stdout: html}}, false, nil
		}
		post, _ := params["post-params"].(string)
		require.Contains(t, post, "_csrf=csrf-save-token")
		return &aitool.ToolResult{Success: true, Data: &aitool.ToolExecutionResult{Stdout: "HTTP/1.1 200 OK\r\n\r\nsaved"}}, false, nil
	}

	report, err := runFrameworkToolkitBulkVerify(context.Background(), inv, rt, catalog, "test")
	require.NoError(t, err)
	require.Equal(t, 1, report.Probed)
	require.Equal(t, 1, report.Verified)

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].Verified)
	require.False(t, strings.Contains(rows[0].RejectReason, "csrf"))
}

func TestWriteFrameworkToolkitGateArtifacts_Offline(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: false}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, SelectedFrameworkID: "publiccms"}

	reg := &CodeUnitRegistryV1{
		SchemaVersion: 1,
		Units: []CodeUnitEntry{{
			RelPath:  "src/FooController.java",
			KindHint: codeUnitKindHintHTTPEntry,
		}},
	}
	require.NoError(t, persistCodeUnitRegistry(rt, reg))
	require.NoError(t, BackfillFeatureInventoryFromRegistry(rt))

	catalog := &CombinedAPICatalog{
		Records: []CombinedAPIRecord{
			{Method: "GET", Path: "/admin/cmsCategory/list", BackendFile: "src/FooController.java", Confidence: "high"},
		},
	}
	report := &ToolkitVerifyReport{TotalRecords: 1, Skipped: 1}
	require.NoError(t, writeFrameworkToolkitGateArtifacts(rt, catalog, report, "publiccms"))
	require.NoError(t, verifyPhase1GranularGate(rt))
}

func TestWriteFrameworkToolkitGateArtifacts_ZeroApiFeaturesGetNoApiReason(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
		TargetReachable: true,
		TargetScheme:    "http",
		TargetHost:      "127.0.0.1",
		TargetPort:      "8080",
		CodePathOK:      true,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess, WorkDir: dir, SelectedFrameworkID: "publiccms"}

	inv := &FeatureInventoryV1{
		SchemaVersion: 1,
		Features: []FeatureInventoryEntry{
			{
				FeatureID:   "http_entry_src/AbstractCkEditorController.java",
				Label:       "AbstractCkEditorController.java",
				SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles:  []string{"src/AbstractCkEditorController.java"},
			},
			{
				FeatureID:   "http_entry_src/package-info.java",
				Label:       "package-info.java",
				SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles:  []string{"src/package-info.java"},
			},
			{
				FeatureID:   "http_entry_src/FooController.java",
				Label:       "FooController.java",
				SurfaceKind: SurfaceKindHTTPAPI,
				EntryFiles:  []string{"src/FooController.java"},
			},
		},
	}
	require.NoError(t, persistFeatureInventory(rt, inv))

	catalog := &CombinedAPICatalog{
		Records: []CombinedAPIRecord{
			{Method: "GET", Path: "/admin/foo/list", BackendFile: "src/FooController.java", HandlerClass: "FooController", Confidence: "high"},
		},
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/admin/foo/list",
		Verified: true, FullSampleURL: "http://127.0.0.1:8080/admin/foo/list",
		ProbeStatusCode: 200, ProbeAttemptsJSON: `[{"status":200}]`,
		Source: "framework_toolkit",
	}))

	decision := &CoverageSignalDecision{Verdict: "finish", Reasoning: "test"}
	persistCoverageSignalDecision(rt, decision)

	report := &ToolkitVerifyReport{TotalRecords: 1, Verified: 1}
	require.NoError(t, writeFrameworkToolkitGateArtifacts(rt, catalog, report, "publiccms"))

	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	require.NoError(t, err)
	reasons := map[string]string{}
	for _, f := range apiMap.Features {
		if f.ApiCount == 0 {
			reasons[f.FeatureID] = f.NoApiReason
		}
	}
	require.Contains(t, reasons["http_entry_src/AbstractCkEditorController.java"], "abstract base class")
	require.Contains(t, reasons["http_entry_src/package-info.java"], "package metadata")
	require.NoError(t, verifyPhase1GranularGate(rt))
}
