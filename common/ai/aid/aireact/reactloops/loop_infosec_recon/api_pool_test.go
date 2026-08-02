package loop_infosec_recon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

type recordingReconAssetSink struct {
	assets []aicommon.AssetResult
}

type sizeLimitedReconAssetSink struct {
	recordingReconAssetSink
	maxPayloadBytes int
}

type selectiveFailReconAssetSink struct {
	recordingReconAssetSink
	rejectIdentity string
}

func (s *recordingReconAssetSink) SubmitRisk(
	context.Context,
	*schema.Risk,
) (aicommon.ResultReceipt, error) {
	return aicommon.ResultReceipt{}, nil
}

func (s *recordingReconAssetSink) SubmitAsset(
	_ context.Context,
	asset aicommon.AssetResult,
) (aicommon.ResultReceipt, error) {
	s.assets = append(s.assets, asset)
	return aicommon.ResultReceipt{
		ResultID:  asset.IdentityKey,
		DedupeKey: asset.IdentityKey,
		BackendID: "job-1",
	}, nil
}

func (s *sizeLimitedReconAssetSink) SubmitAsset(
	ctx context.Context,
	asset aicommon.AssetResult,
) (aicommon.ResultReceipt, error) {
	if len(asset.Payload) > s.maxPayloadBytes {
		return aicommon.ResultReceipt{}, fmt.Errorf(
			"payload exceeds %d bytes",
			s.maxPayloadBytes,
		)
	}
	return s.recordingReconAssetSink.SubmitAsset(ctx, asset)
}

func (s *selectiveFailReconAssetSink) SubmitAsset(
	ctx context.Context,
	asset aicommon.AssetResult,
) (aicommon.ResultReceipt, error) {
	if asset.IdentityKey == s.rejectIdentity {
		return aicommon.ResultReceipt{}, fmt.Errorf("injected asset failure")
	}
	return s.recordingReconAssetSink.SubmitAsset(ctx, asset)
}

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

func TestProbePoolHTTP_DoesNotFollowRedirectsWhenDisabled(t *testing.T) {
	t.Parallel()

	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/outside", http.StatusFound)
	}))
	defer redirectSource.Close()

	parsed, err := url.Parse(redirectSource.URL)
	require.NoError(t, err)
	pool := &APIPool{Entries: []APIPoolEntry{{
		NormalizedURL: redirectSource.URL + "/start",
		Method:        http.MethodGet,
	}}}

	n := probePoolHTTP(
		pool,
		1,
		1,
		true,
		5*time.Second,
		ParseScopeHostSet(parsed.Hostname()),
		false,
	)
	require.Equal(t, 1, n)
	require.Zero(t, redirectedRequests.Load())
	require.True(t, pool.Entries[0].Verified)
	require.Equal(t, http.StatusFound, pool.Entries[0].StatusCode)
}

func TestSubmitVerifiedAPIAssetsPublishesOnlyVerifiedEndpoints(t *testing.T) {
	t.Parallel()

	pool := &APIPool{Entries: []APIPoolEntry{
		{
			NormalizedURL: "https://example.com/api/users",
			Method:        "get",
			Source:        "js_static_extract_ai",
			Confidence:    0.95,
			Evidence:      "fetch('/api/users')",
			Verified:      true,
			StatusCode:    http.StatusOK,
		},
		{
			NormalizedURL: "https://example.com/api/admin",
			Method:        "GET",
			Source:        "js_static_extract_ai",
			Verified:      false,
			StatusCode:    http.StatusForbidden,
		},
	}}
	sink := &recordingReconAssetSink{}

	submitted, err := submitVerifiedAPIAssets(context.Background(), sink, pool)
	require.NoError(t, err)
	require.Equal(t, 1, submitted)
	require.Len(t, sink.assets, 1)

	asset := sink.assets[0]
	require.Equal(t, "http_endpoint", asset.Kind)
	require.Equal(t, "GET https://example.com/api/users", asset.Title)
	require.Equal(t, "https://example.com/api/users", asset.Target)
	require.Equal(
		t,
		"http_endpoint:GET:https://example.com/api/users",
		asset.IdentityKey,
	)
	var payload APIPoolEntry
	require.NoError(t, json.Unmarshal(asset.Payload, &payload))
	require.True(t, payload.Verified)
	require.Equal(t, http.StatusOK, payload.StatusCode)
	require.Equal(t, "js_static_extract_ai", payload.Source)

	var businessPayload map[string]any
	require.NoError(t, json.Unmarshal(asset.Payload, &businessPayload))
	require.Equal(t, "1", businessPayload["schema_version"])
	require.Equal(t, "verified", businessPayload["verification_state"])
	require.Equal(t, true, businessPayload["network_access_performed"])
	require.Equal(t, "https://example.com/api/users", businessPayload["url"])
	require.Equal(t, "https", businessPayload["scheme"])
	require.Equal(t, "example.com", businessPayload["host"])
	require.Equal(t, "443", businessPayload["port"])
	require.Equal(t, "https://example.com/api/users", businessPayload["http_url"])
	require.Equal(t, float64(http.StatusOK), businessPayload["http_status_code"])
}

func TestSubmitVerifiedAPIAssetsBoundsEvidenceAndContinues(t *testing.T) {
	t.Parallel()

	const maxPayloadBytes = 64 * 1024
	pool := &APIPool{Entries: []APIPoolEntry{
		{
			NormalizedURL: "https://example.com/api/large",
			Method:        http.MethodGet,
			Source:        "js_static_extract_ai",
			Evidence:      strings.Repeat("\x01", maxPayloadBytes),
			Verified:      true,
			StatusCode:    http.StatusOK,
		},
		{
			NormalizedURL: "https://example.com/api/after-large",
			Method:        http.MethodPost,
			Source:        "js_static_extract_ai",
			Evidence:      "fetch('/api/after-large')",
			Verified:      true,
			StatusCode:    http.StatusCreated,
		},
	}}
	sink := &sizeLimitedReconAssetSink{maxPayloadBytes: maxPayloadBytes}

	submitted, err := submitVerifiedAPIAssets(context.Background(), sink, pool)
	require.NoError(t, err)
	require.Equal(t, 2, submitted)
	require.Len(t, sink.assets, 2)
	require.LessOrEqual(t, len(sink.assets[0].Payload), maxPayloadBytes)

	var payload APIPoolEntry
	require.NoError(t, json.Unmarshal(sink.assets[0].Payload, &payload))
	require.Less(t, len(payload.Evidence), len(pool.Entries[0].Evidence))
	require.Equal(t, pool.Entries[0].NormalizedURL, payload.NormalizedURL)
}

func TestSubmitVerifiedAPIAssetsContinuesAfterSingleAssetFailure(t *testing.T) {
	t.Parallel()

	pool := &APIPool{Entries: []APIPoolEntry{
		{
			NormalizedURL: "https://example.com/api/fails",
			Method:        http.MethodGet,
			Source:        "test",
			Verified:      true,
			StatusCode:    http.StatusOK,
		},
		{
			NormalizedURL: "https://example.com/api/succeeds",
			Method:        http.MethodPost,
			Source:        "test",
			Verified:      true,
			StatusCode:    http.StatusCreated,
		},
	}}
	sink := &selectiveFailReconAssetSink{
		rejectIdentity: "http_endpoint:GET:https://example.com/api/fails",
	}

	submitted, err := submitVerifiedAPIAssets(context.Background(), sink, pool)
	require.ErrorContains(t, err, "injected asset failure")
	require.Equal(t, 1, submitted)
	require.Len(t, sink.assets, 1)
	require.Equal(
		t,
		"http_endpoint:POST:https://example.com/api/succeeds",
		sink.assets[0].IdentityKey,
	)
}
