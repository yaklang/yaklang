package loop_infosec_recon

import (
	"testing"

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
