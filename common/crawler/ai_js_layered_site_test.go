package crawler

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/crawler/crawlertest"
)

// TestMUSTPASS_AIJSLayeredSite is the hermetic CI contract for adaptive asset
// analysis. Compilation time is outside the SLA; the functional crawl itself
// must finish in less than ten seconds and never contacts a real model or a
// non-loopback service.
func TestMUSTPASS_AIJSLayeredSite(t *testing.T) {
	site := crawlertest.New(t)

	findingsBySource := make(map[string][]crawlertest.ExpectedFinding)
	requiredSources := make(map[string]struct{})
	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || !finding.MustDiscover || !finding.RequiresAI {
			continue
		}
		findingsBySource[finding.SourceAsset] = append(findingsBySource[finding.SourceAsset], finding)
		requiredSources[finding.SourceAsset] = struct{}{}
	}
	evidenceNeedles := map[string][]string{
		crawlertest.HeaderRoutesPath:  {`"catalog":"coldchain-operations"`, `"manifest":"/assets/asset-manifest.json"`},
		crawlertest.ManifestPath:      {`"entrypoints"`, `"runtime_config":"/.config/runtime.config"`},
		crawlertest.RoutesPath:        {"base=/service", "action=/export"},
		crawlertest.RuntimeConfigPath: {`"service_segments"`, `\u002fassets\u002f.config`},
		crawlertest.RuntimeJSPath:     {"fetch(RuNtImE.cOnFiG)"},
		crawlertest.ChunkConfigPath:   {`\u002fassets\u002fchunks`, `"713"`},
		crawlertest.CompiledChunkPath: {"atob(_0x8a", "import(_0x4d(0))"},
		crawlertest.SourceMapPath:     {`"webpack://coldchain/src/quality-ledger.ts"`, `"x_runtime_chunk"`},
		crawlertest.DynamicChunkPath:  {`String["from"+"CharCode"]`, `"X-ColdChain-Module":"quality-ledger"`},
		crawlertest.WorkerPath:        {`const _pool=`, `"X-ColdChain-Worker":"ledger-export"`},
		crawlertest.SmallJSPath:       {"const PaRtS=", "fetch(TaRgEt"},
		crawlertest.MediumJSPath:      {"const _escaped=", "fetch(_escaped"},
		crawlertest.LargeJSPath:       {"atob(_0x91", "fetch(_0x2f(0)"},
		crawlertest.HugeJSPath:        {"String.fromCharCode(", `"X-Asset-Proof":"huge-tail"`},
	}

	var (
		mu             sync.Mutex
		aiCalls        int
		maxPayload     int
		totalEvidence  int
		invokedSources = make(map[string]int)
		matchedSources = make(map[string]int)
		foundRaw       = make(map[string]struct{})
		foundPaths     = make(map[string]struct{})
		structured     []AIJSRequestFinding
		events         []AIJSExtractEvent
	)

	parent, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	ctx := WithAIJSInvokerContext(parent, func(
		ctx context.Context,
		cfg *AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sourcePath := assetPath(cfg.assetSourceURL)
		mu.Lock()
		aiCalls++
		invokedSources[sourcePath]++
		totalEvidence += len(payload)
		if len(payload) > maxPayload {
			maxPayload = len(payload)
		}
		mu.Unlock()

		// Credential values are attached to the seed request on purpose. Every
		// model payload, including external script evidence analyzed on the seed
		// page, must contain only redacted request context.
		for _, secret := range []string{"seed-query-secret", "header-auth-secret", "header-api-secret"} {
			if strings.Contains(payload, secret) {
				t.Errorf("model payload leaked credential %q", secret)
				return nil
			}
		}
		if len(payload) >= 140*1024 {
			t.Errorf("one model payload exceeded the 128 KiB evidence budget plus context: %d", len(payload))
			return nil
		}
		if strings.Contains(payload, crawlertest.HugeTargetPath) {
			t.Errorf("the 5 MiB fixture disclosed its hidden route verbatim")
			return nil
		}

		// The mock is evidence-driven rather than source-name-driven. A trigger
		// without the source expression that explains its finding produces no
		// answer, so the integration test catches broken or starved windows.
		for _, needle := range evidenceNeedles[sourcePath] {
			if !strings.Contains(payload, needle) {
				return nil
			}
		}
		mu.Lock()
		matchedSources[sourcePath]++
		mu.Unlock()

		for _, finding := range findingsBySource[sourcePath] {
			// Asset/config edges use the same structured seam as business
			// request surfaces. A shape-free GET is the positive scheduling
			// contract; no asset is admitted merely because its text resembles
			// a file extension.
			cfg.ReportRequestFinding(AIJSRequestFinding{
				URL:       expectedFindingURL(site, finding),
				Method:    finding.Method,
				Headers:   finding.Headers,
				Body:      finding.Body,
				SourceURL: site.URLFor(finding.SourceAsset),
			})
		}
		return nil
	})

	seed := site.URLFor(crawlertest.RootPath) + "?token=seed-query-secret&keep=visible"
	c, err := NewCrawler(
		seed,
		WithContext(ctx),
		WithForbiddenFromParent(true),
		WithConcurrent(4),
		WithMaxRequestCount(120),
		WithMaxDepth(10),
		WithHeader("Authorization", "Bearer header-auth-secret"),
		WithHeader("X-API-Key", "header-api-secret"),
		WithOnUrlFound(func(raw string) {
			mu.Lock()
			defer mu.Unlock()
			foundRaw[raw] = struct{}{}
			if parsed, parseErr := url.Parse(raw); parseErr == nil {
				foundPaths[parsed.Path] = struct{}{}
			}
		}),
		WithOnAIJSRequestFound(func(finding AIJSRequestFinding) {
			mu.Lock()
			structured = append(structured, finding)
			mu.Unlock()
		}),
		WithAIJSExtract(
			WithAIJS_AdaptiveTrigger(),
			WithAIJS_TriggerThreshold(3),
			WithAIJS_MaxRequests(16),
			WithAIJS_MaxCandidateWindows(96),
			WithAIJS_MaxCandidateBytes(128*1024),
			WithAIJS_ContextBytes(512),
			WithAIJS_CallTimeoutSeconds(2),
			WithAIJS_Concurrency(1),
			WithAIJS_ChunkBytes(48*1024),
			WithAIJS_MaxTokens(12*1024),
			WithAIJS_SmallInputBytes(0),
			WithAIJS_SmallInputTokens(0),
			WithAIJS_Observer(func(event AIJSExtractEvent) {
				mu.Lock()
				events = append(events, event)
				mu.Unlock()
			}),
		),
	)
	require.NoError(t, err)

	started := time.Now()
	require.NoError(t, c.Run())
	elapsed := time.Since(started)
	require.Less(t, elapsed, 10*time.Second, "functional crawler SLA exceeded")
	require.NoError(t, parent.Err(), "the nine-second safety deadline fired before normal completion")

	mu.Lock()
	aiCallCount := aiCalls
	maxPayloadBytes := maxPayload
	totalEvidenceBytes := totalEvidence
	invokedSnapshot := make(map[string]int, len(invokedSources))
	for source, count := range invokedSources {
		invokedSnapshot[source] = count
	}
	matchedSnapshot := make(map[string]int, len(matchedSources))
	for source, count := range matchedSources {
		matchedSnapshot[source] = count
	}
	foundRawSnapshot := make(map[string]struct{}, len(foundRaw))
	for raw := range foundRaw {
		foundRawSnapshot[raw] = struct{}{}
	}
	foundPathSnapshot := make(map[string]struct{}, len(foundPaths))
	for path := range foundPaths {
		foundPathSnapshot[path] = struct{}{}
	}
	eventSnapshot := append([]AIJSExtractEvent(nil), events...)
	structuredSnapshot := append([]AIJSRequestFinding(nil), structured...)
	mu.Unlock()

	require.Positive(t, aiCallCount)
	require.LessOrEqual(t, aiCallCount, 16, "crawler-wide AI budget was exceeded")
	require.Less(t, maxPayloadBytes, 140*1024)
	require.Less(t, maxPayloadBytes, crawlertest.HugeJSSize/20,
		"the complete 5 MiB asset was effectively forwarded to the model")
	t.Logf("invoked sources before ground-truth assertions: %#v", invokedSnapshot)

	for source := range requiredSources {
		require.Positivef(t, invokedSnapshot[source], "adaptive trigger did not analyze %s", source)
		require.Positivef(t, matchedSnapshot[source], "adaptive analysis did not retain sufficient evidence for %s", source)
	}
	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || !finding.MustCandidate {
			continue
		}
		_, ok := foundPathSnapshot[finding.Value]
		require.Truef(t, ok, "missing candidate-layer finding %s (%s) from %s", finding.ID, finding.Value, finding.SourceAsset)
	}

	structuredByKey := make(map[string]AIJSRequestFinding, len(structuredSnapshot))
	for _, finding := range structuredSnapshot {
		structuredByKey[requestFindingKey(finding.Method, finding.URL)] = finding
	}
	for _, expected := range site.GroundTruth.Findings {
		if expected.Scope != crawlertest.ScopeInternal || !expected.MustStructure {
			continue
		}
		key := requestFindingKey(expected.Method, expectedFindingURL(site, expected))
		actual, ok := structuredByKey[key]
		require.Truef(t, ok, "missing structured request-layer finding %s (%s)", expected.ID, key)
		require.Equalf(t, normalizedHeaders(expected.Headers), normalizedHeaders(actual.Headers), "header shape mismatch for %s", expected.ID)
		require.Equalf(t, expected.Body, actual.Body, "body shape mismatch for %s", expected.ID)
		require.Equalf(t, site.URLFor(expected.SourceAsset), actual.SourceURL, "source provenance mismatch for %s", expected.ID)
	}

	// An out-of-scope script remains visible as an asset candidate but is never
	// downloaded or analyzed, so its own hidden request surface cannot leak in.
	_, externalDiscovered := foundRawSnapshot[site.ExternalScriptURL]
	require.True(t, externalDiscovered, "external script asset should be reported as discovered")
	require.Zero(t, site.RequestCount(crawlertest.ScopeExternal, crawlertest.ExternalScriptPath))
	require.Zero(t, site.RequestCount(crawlertest.ScopeExternal, crawlertest.ExternalTargetPath))
	require.Zero(t, invokedSnapshot[crawlertest.ExternalScriptPath])

	// Every internal source asset is fetched at most once, even if both an HTML
	// script tag and a later manifest mention it.
	for _, asset := range site.GroundTruth.Assets {
		if asset.Scope != crawlertest.ScopeInternal || asset.Path == crawlertest.RootPath {
			continue
		}
		require.LessOrEqualf(t, site.RequestCount(crawlertest.ScopeInternal, asset.Path), 1,
			"asset downloaded more than once: %s", asset.Path)
	}
	require.Equal(t, 1, site.RequestCount(crawlertest.ScopeInternal, crawlertest.HugeJSPath))
	require.Greater(t, site.GroundTruth.HugeTargetOffset, crawlertest.HugeMinimumOffset)

	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || !finding.MustRequest {
			continue
		}
		matched := false
		for _, request := range site.Requests() {
			if request.Path == finding.Value && request.Method == finding.Method && request.RawQuery == finding.RawQuery {
				matched = true
				break
			}
		}
		require.Truef(t, matched, "requested-layer finding was not fetched with its safe shape: %s", finding.ID)
	}
	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || finding.Kind != crawlertest.FindingRequest || finding.MustRequest {
			continue
		}
		require.Zero(t, site.RequestCount(crawlertest.ScopeInternal, finding.Value),
			"structured-only operation must not be downgraded to an automatic GET: %s", finding.ID)
	}

	triggeredEvents := 0
	for _, event := range eventSnapshot {
		require.NotContains(t, event.SourceURL, "seed-query-secret",
			"observer metadata must redact signed/query credentials")
		if event.Triggered {
			triggeredEvents++
		}
	}
	require.Positive(t, triggeredEvents)
	t.Logf("adaptive crawl elapsed=%s http_requests=%d ai_calls=%d max_payload=%d total_evidence=%d triggered_assets=%d structured_requests=%d",
		elapsed, len(site.Requests()), aiCallCount, maxPayloadBytes, totalEvidenceBytes, triggeredEvents, len(structuredSnapshot))
}

func expectedFindingURL(site *crawlertest.Site, finding crawlertest.ExpectedFinding) string {
	raw := site.URLFor(finding.Value)
	if finding.RawQuery != "" {
		raw += "?" + finding.RawQuery
	}
	return raw
}

func requestFindingKey(method, rawURL string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(rawURL)
}

func normalizedHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		normalized[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return normalized
}

func assetPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Path == "" {
		return raw
	}
	return parsed.Path
}
