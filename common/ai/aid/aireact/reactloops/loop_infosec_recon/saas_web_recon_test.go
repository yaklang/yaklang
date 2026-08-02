package loop_infosec_recon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrawlBoundedSaaSReconStaysInsideAuthorizedOrigin(t *testing.T) {
	var outsideRequests atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		outsideRequests.Add(1)
		_, _ = w.Write([]byte(`outside`))
	}))
	defer outside.Close()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body>
				<a href="/next">next</a>
				<a href="` + outside.URL + `/outside">outside</a>
				<script src="/app.js"></script>
				<script src="` + outside.URL + `/outside.js"></script>
			</body></html>`))
		case "/next":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><form action="/api/orders" method="post"></form></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/users")`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()

	stats, findings, err := crawlBoundedSaaSRecon(
		context.Background(),
		newBoundedSaaSReconHTTPClient(),
		server.URL+"/",
		t.TempDir(),
	)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Pages, 2)
	require.Equal(t, 1, stats.Scripts)
	require.LessOrEqual(t, stats.Requests, boundedSaaSReconMaxRequests)
	require.Equal(t, int32(0), outsideRequests.Load())
	require.True(t, containsBoundedSaaSReconFinding(findings, http.MethodGet, server.URL+"/next"))
	require.True(t, containsBoundedSaaSReconFinding(findings, http.MethodPost, server.URL+"/api/orders"))
}

func TestExtractBoundedSaaSReconStaticFindingsUsesCollectorDirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "app.js"),
		[]byte(`
			fetch("/api/users");
			const graph = "/graphql";
			const outside = "https://outside.invalid/api/ignored";
			const otherPort = "https://business.example:9443/api/ignored";
			const image = "/assets/logo.png";
		`),
		0o600,
	))

	findings, filesRead, err := extractBoundedSaaSReconStaticFindings(
		"https://business.example:8443/",
		dir,
	)
	require.NoError(t, err)
	require.Equal(t, 1, filesRead)
	require.True(t, containsBoundedSaaSReconFinding(findings, http.MethodGet, "https://business.example:8443/api/users"))
	require.True(t, containsBoundedSaaSReconFinding(findings, http.MethodGet, "https://business.example:8443/graphql"))
	require.Len(t, findings, 2)
}

func TestFilterBoundedSaaSReconPoolRejectsDifferentPortAndScheme(t *testing.T) {
	pool := &APIPool{Entries: []APIPoolEntry{
		{NormalizedURL: "https://business.example:8443/api/ok"},
		{NormalizedURL: "https://business.example:9443/api/wrong-port"},
		{NormalizedURL: "http://business.example:8443/api/wrong-scheme"},
		{NormalizedURL: "https://other.example:8443/api/wrong-host"},
	}}

	filterBoundedSaaSReconPool(pool, "https://business.example:8443/")
	require.Equal(t, []APIPoolEntry{{NormalizedURL: "https://business.example:8443/api/ok"}}, pool.Entries)
}

func containsBoundedSaaSReconFinding(findings []boundedSaaSReconFinding, method, target string) bool {
	for _, finding := range findings {
		if finding.Method == method && finding.URL == target {
			return true
		}
	}
	return false
}
