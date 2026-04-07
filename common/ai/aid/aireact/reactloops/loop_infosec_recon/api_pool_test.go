package loop_infosec_recon

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	u, err := NormalizeURL("https://Example.COM/path#frag", "")
	require.NoError(t, err)
	require.Contains(t, u, "example.com")
	require.Contains(t, u, "/path")
	require.NotContains(t, u, "#")

	u2, err := NormalizeURL("/api/v1/x", "https://a.com/")
	require.NoError(t, err)
	require.Equal(t, "https://a.com/api/v1/x", u2)
}

func TestMergeFindings_Dedupe(t *testing.T) {
	p := &APIPool{Entries: []APIPoolEntry{}}
	added, errs := MergeFindings(p, "https://x.com/", []struct {
		URL, Method, Source, Evidence string
		Confidence                    float64
	}{
		{URL: "https://x.com/a", Method: "GET", Source: "t1", Evidence: "e1"},
		{URL: "https://x.com/a", Method: "GET", Source: "t2", Evidence: "e2"},
		{URL: "/b", Method: "POST", Source: "t3", Evidence: "e3"},
	})
	require.Len(t, errs, 0)
	require.Equal(t, 2, added)
	require.Len(t, p.Entries, 2)
}

func TestExtractFromJSReport(t *testing.T) {
	raw := []byte(`{
	  "apis_final": [
	    {"full_url": "https://ex.com/u", "http_method": "GET", "evidence": "e"}
	  ],
	  "apis_merged_map": {
	    "k": {"full_url": "https://ex.com/v", "http_method": "POST", "evidence": "m"}
	  }
	}`)
	got := ExtractFromJSReport(raw)
	require.GreaterOrEqual(t, len(got), 2)
}

func TestMergeFindings_ScopeHosts(t *testing.T) {
	p := &APIPool{Entries: []APIPoolEntry{}}
	added, errs := MergeFindings(p, "", []struct {
		URL, Method, Source, Evidence string
		Confidence                    float64
	}{
		{URL: "https://keep.com/a", Method: "GET", Source: "s", Evidence: "e"},
		{URL: "https://drop.com/b", Method: "GET", Source: "s", Evidence: "e"},
	}, "keep.com")
	require.Empty(t, errs)
	require.Equal(t, 1, added)
	require.Len(t, p.Entries, 1)
	require.Contains(t, p.Entries[0].NormalizedURL, "keep.com")
}

func TestProbePoolHTTP_VerifiedSemantics(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	nfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer nfSrv.Close()

	p := &APIPool{Entries: []APIPoolEntry{
		{NormalizedURL: okSrv.URL + "/a", Method: "GET"},
		{NormalizedURL: nfSrv.URL + "/b", Method: "GET"},
	}}
	n := ProbePoolHTTP(p, 10, 2, false, 5*time.Second, nil)
	require.Equal(t, 2, n)

	var okEntry, nfEntry *APIPoolEntry
	for i := range p.Entries {
		e := &p.Entries[i]
		if strings.HasPrefix(e.NormalizedURL, okSrv.URL) {
			okEntry = e
		}
		if strings.HasPrefix(e.NormalizedURL, nfSrv.URL) {
			nfEntry = e
		}
	}
	require.NotNil(t, okEntry)
	require.True(t, okEntry.Verified)
	require.Empty(t, okEntry.ProbeError)

	require.NotNil(t, nfEntry)
	require.False(t, nfEntry.Verified)
	require.NotEmpty(t, nfEntry.ProbeError)
}

func TestProbePoolHTTP_RespectsScopeHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	hostname := u.Hostname()

	p := &APIPool{Entries: []APIPoolEntry{
		{NormalizedURL: srv.URL, Method: "GET"},
		{NormalizedURL: "https://other-scope.example/path", Method: "GET"},
	}}
	allowed := ParseScopeHostSet(hostname)
	n := ProbePoolHTTP(p, 10, 1, false, 5*time.Second, allowed)
	require.Equal(t, 1, n)

	var srvEntry, other *APIPoolEntry
	for i := range p.Entries {
		e := &p.Entries[i]
		if strings.HasPrefix(e.NormalizedURL, srv.URL) {
			srvEntry = e
		}
		if strings.Contains(e.NormalizedURL, "other-scope.example") {
			other = e
		}
	}
	require.NotNil(t, srvEntry)
	require.True(t, srvEntry.Verified)
	require.NotNil(t, other)
	require.False(t, other.Verified)
	require.Zero(t, other.StatusCode)
}
