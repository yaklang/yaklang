package loop_ssa_api_discovery

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const (
	livePublicCMSRoot   = "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/real-cms/PublicCMS/publiccms-parent"
	livePublicCMSTarget = "http://192.168.1.4:8080"
	livePublicCMSUser   = "admin"
	livePublicCMSPass   = "potian123"
)

func TestFrameworkToolkitLive_PublicCMSRemote(t *testing.T) {
	if os.Getenv("YAK_SSA_SKIP_LIVE") == "1" {
		t.Skip("YAK_SSA_SKIP_LIVE=1")
	}
	if _, err := os.Stat(livePublicCMSRoot); err != nil {
		t.Skip("PublicCMS source not available: ", livePublicCMSRoot)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(livePublicCMSTarget + "/admin/login")
	if err != nil {
		t.Skip("remote target unreachable: ", livePublicCMSTarget, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		t.Skip("remote target unhealthy: ", resp.StatusCode)
	}

	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	repo, cleanup := openIntegrationTestRepo(t, dir)
	defer cleanup()

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
		CodeRootPath:    livePublicCMSRoot,
		CodePathOK:      true,
		Language:        "java",
		TargetReachable: true,
		TargetScheme:    "http",
		TargetHost:      "192.168.1.4",
		TargetPort:      "8080",
		TargetRaw:       livePublicCMSTarget,
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{
		Repo:                    repo,
		Session:                 sess,
		WorkDir:                 workDir,
		UserAuthUsername:        livePublicCMSUser,
		UserAuthPassword:        livePublicCMSPass,
		FrameworkToolkitEnabled: true,
	}

	catalog, err := RunCombinedProgrammaticAPIExtraction(rt)
	require.NoError(t, err)
	t.Logf("catalog total=%d csrf=%d", catalog.Stats.Total, catalog.Stats.WithCsrf)

	probeCatalog := &CombinedAPICatalog{Records: pickRepresentativeRecords(catalog, 25)}
	t.Logf("live probe sample=%d target=%s", len(probeCatalog.Records), livePublicCMSTarget)

	inv := newLiveHTTPInvoker()
	ctx := context.Background()
	require.NoError(t, acquirePublicCMSCredentials(ctx, inv, rt))
	require.True(t, hasVerifiedAuthCredential(rt))

	creds, err := repo.ListAuthCredentials(sess.ID)
	require.NoError(t, err)
	require.NotEmpty(t, creds)
	_, tok, ok := getCsrfTokenForCredential(workDir, creds[0].ID)
	require.True(t, ok, "csrf token should be prefetched against live target")
	t.Logf("csrf prefetched param=_csrf token_len=%d", len(tok))

	report, err := runFrameworkToolkitBulkVerify(ctx, inv, rt, probeCatalog, "live_publiccms")
	require.NoError(t, err)

	rows, err := repo.ListVerifiedHttpApis(sess.ID)
	require.NoError(t, err)
	var verified, csrfReject, otherReject int
	for _, row := range rows {
		if row.Verified {
			verified++
		} else if row.RejectReason == "csrf_required" {
			csrfReject++
		} else if row.RejectReason != "" {
			otherReject++
		}
	}
	t.Logf("live bulk_verify: probed=%d verified=%d rejected=%d destructive_skip=%d errors=%d",
		report.Probed, report.Verified, report.Rejected, report.DestructiveSkip, report.Errors)
	t.Logf("live verified_http_apis: verified=%d csrf_required=%d other_reject=%d rows=%d",
		verified, csrfReject, otherReject, len(rows))

	require.Greater(t, report.Probed, 0)
	require.True(t, hasVerifiedAuthCredential(rt))
	require.Less(t, csrfReject, report.Probed/2, "csrf prefetch should keep csrf_required minority on live target")
}
