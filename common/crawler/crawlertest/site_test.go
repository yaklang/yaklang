package crawlertest

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLayeredSiteGroundTruth(t *testing.T) {
	site := New(t)

	expectedSizes := map[string]int{
		SmallJSPath:      SmallJSSize,
		MediumJSPath:     MediumJSSize,
		LargeJSPath:      LargeJSSize,
		HugeJSPath:       HugeJSSize,
		DynamicChunkPath: dynamicChunkSize,
		WorkerPath:       workerJSSize,
	}
	for path, expected := range expectedSizes {
		asset, ok := site.GroundTruth.Asset(path)
		if !ok {
			t.Fatalf("missing ground-truth asset %s", path)
		}
		if asset.Size != expected {
			t.Fatalf("asset %s size=%d, want %d", path, asset.Size, expected)
		}
	}
	if site.GroundTruth.HugeTargetOffset <= HugeMinimumOffset {
		t.Fatalf("huge target offset=%d must be beyond %d", site.GroundTruth.HugeTargetOffset, HugeMinimumOffset)
	}
	huge, ok := site.GroundTruth.Finding(site.GroundTruth.HugeTargetID)
	if !ok || huge.Value != HugeTargetPath || huge.SourceOffset != site.GroundTruth.HugeTargetOffset {
		t.Fatalf("invalid huge finding: %#v", huge)
	}

	seedURL, err := url.Parse(site.URL)
	if err != nil {
		t.Fatal(err)
	}
	externalURL, err := url.Parse(site.ExternalScriptURL)
	if err != nil {
		t.Fatal(err)
	}
	if seedURL.Hostname() == externalURL.Hostname() {
		t.Fatalf("scope fixture hostnames must differ: seed=%s external=%s", seedURL.Hostname(), externalURL.Hostname())
	}

	counts := site.GroundTruth.CoverageCounts()
	expectedCoverage := map[CoverageLayer]int{
		CoverageCandidate:  20,
		CoverageRequested:  20,
		CoverageStructured: 8,
	}
	for layer, expected := range expectedCoverage {
		if counts[layer] != expected {
			t.Fatalf("coverage oracle %s=%d, want %d: %#v", layer, counts[layer], expected, counts)
		}
	}

	// Every AI-owned asset/config edge is an explicit, source-attributed,
	// shape-free GET. This is the positive scheduling contract: the mock must
	// not rely on a bare file extension being replayed automatically.
	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != ScopeInternal || !finding.RequiresAI || finding.Kind == FindingRequest {
			continue
		}
		if finding.Method != http.MethodGet || len(finding.Headers) != 0 || finding.Body != "" {
			t.Fatalf("AI-owned asset edge %s is not a shape-free GET: %#v", finding.ID, finding)
		}
		if !finding.MustCandidate || !finding.MustRequest {
			t.Fatalf("AI-owned asset edge %s lacks candidate/request oracle layers: %#v", finding.ID, finding)
		}
		if _, ok := site.GroundTruth.Asset(finding.SourceAsset); !ok {
			t.Fatalf("AI-owned asset edge %s has unknown source %s", finding.ID, finding.SourceAsset)
		}
		if _, ok := site.GroundTruth.Asset(finding.Value); !ok {
			t.Fatalf("AI-owned asset edge %s has unknown target %s", finding.ID, finding.Value)
		}
	}

	rootResponse, err := http.Get(site.URLFor(RootPath))
	if err != nil {
		t.Fatal(err)
	}
	rootBody, _ := io.ReadAll(rootResponse.Body)
	_ = rootResponse.Body.Close()
	if !strings.Contains(rootResponse.Header.Get("Link"), HeaderRoutesPath) || bytes.Contains(rootBody, []byte(HeaderRoutesPath)) {
		t.Fatalf("header-only Link fixture leaked into HTML: header=%q body=%q", rootResponse.Header.Get("Link"), rootBody)
	}

	chunkResponse, err := http.Get(site.URLFor(CompiledChunkPath))
	if err != nil {
		t.Fatal(err)
	}
	chunkBody, _ := io.ReadAll(chunkResponse.Body)
	_ = chunkResponse.Body.Close()
	if chunkResponse.Header.Get("SourceMap") != SourceMapPath || bytes.Contains(chunkBody, []byte(SourceMapPath)) {
		t.Fatalf("header-only SourceMap fixture leaked into JavaScript: header=%q", chunkResponse.Header.Get("SourceMap"))
	}

	assertFixtureRequestStatus(t, site, "small header-gated GET", http.MethodGet,
		site.URLFor(SmallTargetPath)+"?region=north", map[string]string{"X-Client-Surface": "catalog-matrix"}, "", http.StatusOK)
	assertFixtureRequestStatus(t, site, "small GET without required header", http.MethodGet,
		site.URLFor(SmallTargetPath)+"?region=north", nil, "", http.StatusNotFound)
	assertFixtureRequestStatus(t, site, "medium structured POST", http.MethodPost,
		site.URLFor(MediumTargetPath), map[string]string{"X-Route-Profile": "escaped"}, `{"dispatch":"priority"}`, http.StatusOK)
}

func assertFixtureRequestStatus(t *testing.T, site *Site, name, method, target string, headers map[string]string, body string, expected int) {
	t.Helper()
	request, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != expected {
		t.Fatalf("%s status=%d, want %d", name, response.StatusCode, expected)
	}
}
