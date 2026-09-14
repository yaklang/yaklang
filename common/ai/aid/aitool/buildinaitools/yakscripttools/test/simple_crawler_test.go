package test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/crawler/crawlertest"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const simpleCrawlerToolName = "simple_crawler"

func getSimpleCrawlerTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/http/simple_crawler.yak")
	if err != nil {
		t.Fatalf("failed to read simple_crawler.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(simpleCrawlerToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse simple_crawler.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func execCrawlerTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	// ai-js defaults to auto. Keep every existing EmbedFS test hermetic by
	// installing a per-call no-op invoker; focused tests below replace it with
	// deterministic behavior. This must not use a process-global LiteForge hook.
	ctx := crawler.WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		return nil
	})
	stdout, stderr, _ = execCrawlerToolWithContext(t, ctx, tool, params)
	return stdout, stderr
}

func execCrawlerToolWithContext(t *testing.T, ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string, err error) {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err = tool.Callback(ctx, params, nil, w1, w2)
	if err != nil {
		t.Logf("crawler tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String(), err
}

func TestSimpleCrawler_DefaultAutoUsesContextScopedAIInvoker(t *testing.T) {
	var aiCalls atomic.Int64
	var hiddenRequests atomic.Int64

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><body><div id="app"></div><script src="/assets/app.chunk.js"></script></body></html>`))
		case "/assets/app.chunk.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			// A dynamic request expression plus encoded/config evidence crosses the
			// adaptive trigger threshold. The URL itself is deliberately absent.
			_, _ = w.Write([]byte(`const runtimeConfig={apiBase:atob("L2FwaQ==")};fetch(runtimeConfig.apiBase+window.__tenant);`))
		case "/api/context-mock-only":
			hiddenRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	ctx := crawler.WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		aiCalls.Add(1)
		assert.Assert(t, len(payload) < 256*1024, "adaptive payload must stay bounded, got %d bytes", len(payload))
		if !strings.Contains(payload, "runtimeConfig") || !strings.Contains(payload, "window.__tenant") {
			return nil
		}
		cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
			URL:    baseURL + "/api/context-mock-only",
			Method: http.MethodGet,
		})
		return nil
	})

	stdout, stderr, err := execCrawlerToolWithContext(t, ctx, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  12,
		"max-depth": 3,
		"timeout":   3,
		// Deliberately omit ai-js: this verifies the declared default is auto.
	})
	assert.NilError(t, err, "stderr=%s", stderr)
	assert.Assert(t, aiCalls.Load() >= 1, "the context-scoped mock should handle adaptive AI calls")
	assert.Assert(t, aiCalls.Load() <= 3, "the script-level shared AI budget must cap calls, got %d", aiCalls.Load())
	assert.Equal(t, hiddenRequests.Load(), int64(1), "the mock-only request candidate should be crawled once")
	assert.Assert(t, strings.Contains(stdout, baseURL+"/api/context-mock-only"), "mock candidate missing from crawler output:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "adaptive bounded static analysis"), "default auto mode should be reported:\n%s", stdout)

	for _, section := range []string{
		"=== Crawl Summary ===",
		"=== Requested URLs ===",
		"=== Found URLs ===",
		"=== JavaScript Request Surfaces ===",
		"=== Rendering Assessment ===",
		"=== Follow-up Guidance ===",
	} {
		assert.Assert(t, strings.Contains(stdout, section), "legacy output section missing: %s", section)
	}
	assert.Assert(t, strings.Contains(stdout, "AI call budget:   3"), "auto mode should expose its low-cost model budget:\n%s", stdout)
}

func TestSimpleCrawler_StructuredRequestSurfacesAreReportedButUnsafeShapesAreNotSent(t *testing.T) {
	var safeGETRequests atomic.Int64
	var headerGETRequests atomic.Int64
	var headRequests atomic.Int64
	var postRequests atomic.Int64

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><script src="/assets/request-shapes.js"></script>`))
		case "/assets/request-shapes.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write([]byte(`const runtimeConfig={api:atob("L2FwaQ==")};fetch(runtimeConfig.api+window.__route,{method:"POST",headers:{"X-Flow":"commit"}});`))
		case "/api/public-catalog":
			safeGETRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/header-gated":
			headerGETRequests.Add(1)
			http.Error(w, "must not be replayed", http.StatusTeapot)
		case "/api/metadata":
			headRequests.Add(1)
			http.Error(w, "must not be replayed", http.StatusTeapot)
		case "/api/commit":
			postRequests.Add(1)
			http.Error(w, "must not be replayed", http.StatusTeapot)
		default:
			http.NotFound(w, r)
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	ctx := crawler.WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		if !strings.Contains(payload, "runtimeConfig") || !strings.Contains(payload, `"X-Flow":"commit"`) {
			return nil
		}
		cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
			URL:       baseURL + "/api/public-catalog",
			Method:    http.MethodGet,
			SourceURL: baseURL + "/assets/request-shapes.js",
		})
		cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
			URL:       baseURL + "/api/header-gated?view=full",
			Method:    http.MethodGet,
			Headers:   map[string]string{"X-ColdChain-Module": "quality-ledger"},
			SourceURL: baseURL + "/assets/request-shapes.js",
		})
		cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
			URL:       baseURL + "/api/metadata",
			Method:    http.MethodHead,
			SourceURL: baseURL + "/assets/request-shapes.js",
		})
		cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
			URL:       baseURL + "/api/commit",
			Method:    http.MethodPost,
			Headers:   map[string]string{"X-Flow": "commit"},
			Body:      `{"mode":"reconcile"}`,
			SourceURL: baseURL + "/assets/request-shapes.js",
		})
		return nil
	})

	stdout, stderr, err := execCrawlerToolWithContext(t, ctx, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  12,
		"max-depth": 3,
		"timeout":   3,
	})
	assert.NilError(t, err, "stderr=%s", stderr)
	assert.Equal(t, safeGETRequests.Load(), int64(1), "plain GET finding should enter the normal queue once")
	assert.Equal(t, headerGETRequests.Load(), int64(0), "header-gated GET must remain report-only")
	assert.Equal(t, headRequests.Load(), int64(0), "HEAD must remain report-only under the GET-only queue contract")
	assert.Equal(t, postRequests.Load(), int64(0), "POST must remain report-only")
	for _, expected := range []string{
		"=== JavaScript Request Surfaces ===",
		"[SHAPE-SAFE GET CANDIDATE (scope, redaction, and budgets still apply)] [GET] " + baseURL + "/api/public-catalog",
		"[DISCOVERED, NOT AUTOMATICALLY EXECUTED] [GET] " + baseURL + "/api/header-gated?view=full",
		"X-ColdChain-Module: quality-ledger",
		"[DISCOVERED, NOT AUTOMATICALLY EXECUTED] [HEAD] " + baseURL + "/api/metadata",
		"[DISCOVERED, NOT AUTOMATICALLY EXECUTED] [POST] " + baseURL + "/api/commit",
		`Body: {"mode":"reconcile"}`,
		"Source: " + baseURL + "/assets/request-shapes.js",
		"Request surfaces: 4 structured JavaScript finding(s)",
	} {
		assert.Assert(t, strings.Contains(stdout, expected), "missing structured-surface evidence %q:\n%s", expected, stdout)
	}
}

func TestSimpleCrawler_ExplicitDepthBudgetAndDomainScope(t *testing.T) {
	var externalRequests atomic.Int64
	// Serve localhost on IPv4 and, when available, IPv6 at the same port. This
	// lets the runner's resolver choose either family without requiring IPv6 or
	// a bindable 127.0.0.2 alias.
	externalListener, err := net.Listen("tcp4", "127.0.0.1:0")
	assert.NilError(t, err)
	externalServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vendor/external.js" {
			http.NotFound(w, r)
			return
		}
		externalRequests.Add(1)
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(`globalThis.externalLoaded=true;`))
	})}
	go func() {
		_ = externalServer.Serve(externalListener)
	}()
	_, externalPort, err := net.SplitHostPort(externalListener.Addr().String())
	assert.NilError(t, err)
	if externalIPv6Listener, listenErr := net.Listen("tcp6", "[::1]:"+externalPort); listenErr == nil {
		go func() {
			_ = externalServer.Serve(externalIPv6Listener)
		}()
	}
	t.Cleanup(func() {
		_ = externalServer.Close()
	})
	externalURL := "http://localhost:" + externalPort + "/vendor/external.js"

	seedHost, seedPort := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><script src="` + externalURL + `"></script>`))
	})
	seedURL := "http://" + seedHost + ":" + strconv.Itoa(seedPort)
	tool := getSimpleCrawlerTool(t)
	scopeParent, scopeCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer scopeCancel()
	noOpAIContext := crawler.WithAIJSInvokerContext(scopeParent, func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		return nil
	})
	defaultOutput, defaultStderr, defaultErr := execCrawlerToolWithContext(t, noOpAIContext, tool, aitool.InvokeParams{
		"urls":      seedURL,
		"reqs-max":  8,
		"max-depth": 3,
		"timeout":   3,
		"ai-js":     "yes",
	})
	assert.NilError(t, defaultErr, "stderr=%s", defaultStderr)
	assert.Equal(t, externalRequests.Load(), int64(0),
		"default exact-hostname scope must not fetch another loopback hostname")
	assert.Assert(t, strings.Contains(defaultOutput, "Domain scope:     exact seed hostname(s) only"), "default scope must be visible:\n%s", defaultOutput)

	explicitOutput, explicitStderr, explicitErr := execCrawlerToolWithContext(t, noOpAIContext, tool, aitool.InvokeParams{
		"urls":          seedURL,
		"reqs-max":      8,
		"max-depth":     3,
		"timeout":       3,
		"ai-js":         "yes",
		"scope-domains": "localhost",
	})
	assert.NilError(t, explicitErr, "stderr=%s", explicitStderr)
	assert.Equal(t, externalRequests.Load(), int64(1),
		"explicit localhost scope should fetch the referenced external script exactly once")
	assert.Assert(t, strings.Contains(explicitOutput, "AI call budget:   8"), "explicit yes mode should expose the deeper budget:\n%s", explicitOutput)
	assert.Assert(t, strings.Contains(explicitOutput, "plus explicit allowlist: localhost"), "explicit domain scope must be visible:\n%s", explicitOutput)
	assert.NilError(t, scopeParent.Err(), "scope regression exceeded its eight-second safety deadline")
}

func TestSimpleCrawler_HTTPSFallbackHonorsCLIFlag(t *testing.T) {
	var httpHandlerRequests atomic.Int64
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	assert.NilError(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHandlerRequests.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><title>plain HTTP fallback</title>`))
	})}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	host, port, err := net.SplitHostPort(listener.Addr().String())
	assert.NilError(t, err)
	httpsSeed := "https://" + net.JoinHostPort(host, port) + "/"
	tool := getSimpleCrawlerTool(t)
	fallbackParent, fallbackCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer fallbackCancel()
	baseParams := aitool.InvokeParams{
		"urls":      httpsSeed,
		"reqs-max":  2,
		"max-retry": 0,
		"max-depth": 1,
		"timeout":   1,
		"ai-js":     "no",
	}

	disabledParams := aitool.InvokeParams{}
	for key, value := range baseParams {
		disabledParams[key] = value
	}
	disabledParams["https-fallback"] = false
	_, disabledStderr, disabledErr := execCrawlerToolWithContext(t, fallbackParent, tool, disabledParams)
	assert.NilError(t, disabledErr, "stderr=%s", disabledStderr)
	assert.Equal(t, httpHandlerRequests.Load(), int64(0),
		"https-fallback=false must not retry the HTTPS seed as plain HTTP")

	enabledParams := aitool.InvokeParams{}
	for key, value := range baseParams {
		enabledParams[key] = value
	}
	enabledParams["https-fallback"] = true
	stdout, enabledStderr, enabledErr := execCrawlerToolWithContext(t, fallbackParent, tool, enabledParams)
	assert.NilError(t, enabledErr, "stderr=%s", enabledStderr)
	assert.Equal(t, httpHandlerRequests.Load(), int64(1),
		"https-fallback=true should retry the same endpoint as plain HTTP exactly once")
	assert.Assert(t, strings.Contains(stdout, "transport retries/fallback are not counted separately"),
		"summary must distinguish logical URL visits from physical transport attempts:\n%s", stdout)
	assert.NilError(t, fallbackParent.Err(), "fallback regression exceeded its eight-second safety deadline")
}

func TestSimpleCrawler_LayeredSiteDeterministicPipelineCoverage(t *testing.T) {
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
	evidenceSources := []struct {
		path    string
		needles []string
	}{
		{crawlertest.HeaderRoutesPath, []string{`"catalog":"coldchain-operations"`, `"manifest":"/assets/asset-manifest.json"`}},
		{crawlertest.ManifestPath, []string{`"entrypoints"`, `"runtime_config":"/.config/runtime.config"`}},
		{crawlertest.RoutesPath, []string{"base=/service", "action=/export"}},
		{crawlertest.RuntimeConfigPath, []string{`"service_segments"`, `\u002fassets\u002f.config`}},
		{crawlertest.RuntimeJSPath, []string{"fetch(RuNtImE.cOnFiG)"}},
		{crawlertest.ChunkConfigPath, []string{`\u002fassets\u002fchunks`, `"713"`}},
		{crawlertest.CompiledChunkPath, []string{"atob(_0x8a", "import(_0x4d(0))"}},
		{crawlertest.SourceMapPath, []string{`"webpack://coldchain/src/quality-ledger.ts"`, `"x_runtime_chunk"`}},
		{crawlertest.DynamicChunkPath, []string{`String["from"+"CharCode"]`, `"X-ColdChain-Module":"quality-ledger"`}},
		{crawlertest.WorkerPath, []string{`const _pool=`, `"X-ColdChain-Worker":"ledger-export"`}},
		{crawlertest.SmallJSPath, []string{"const PaRtS=", "fetch(TaRgEt"}},
		{crawlertest.MediumJSPath, []string{"const _escaped=", "fetch(_escaped"}},
		{crawlertest.LargeJSPath, []string{"atob(_0x91", "fetch(_0x2f(0)"}},
		{crawlertest.HugeJSPath, []string{"String.fromCharCode(", `"X-Asset-Proof":"huge-tail"`}},
	}

	var (
		metricsMu        sync.Mutex
		aiCalls          int
		maxPayload       int
		matchedSources   = make(map[string]int)
		payloadViolation string
	)
	parent, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	ctx := crawler.WithAIJSInvokerContext(parent, func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		metricsMu.Lock()
		aiCalls++
		if len(payload) > maxPayload {
			maxPayload = len(payload)
		}
		if len(payload) >= crawlertest.HugeJSSize/20 && payloadViolation == "" {
			payloadViolation = "one AI payload approached the complete oversized bundle"
		}
		if strings.Contains(payload, crawlertest.HugeTargetPath) && payloadViolation == "" {
			payloadViolation = "hidden 5 MiB route appeared verbatim before mocked extraction"
		}
		metricsMu.Unlock()

		sourcePath := ""
		for _, source := range evidenceSources {
			matched := true
			for _, needle := range source.needles {
				if !strings.Contains(payload, needle) {
					matched = false
					break
				}
			}
			if matched {
				sourcePath = source.path
				break
			}
		}
		if sourcePath == "" {
			return nil
		}
		metricsMu.Lock()
		matchedSources[sourcePath]++
		metricsMu.Unlock()

		for _, finding := range findingsBySource[sourcePath] {
			cfg.ReportRequestFinding(crawler.AIJSRequestFinding{
				URL:       layeredSiteFindingURL(site, finding),
				Method:    finding.Method,
				Headers:   finding.Headers,
				Body:      finding.Body,
				SourceURL: site.URLFor(finding.SourceAsset),
			})
		}
		return nil
	})

	started := time.Now()
	stdout, stderr, err := execCrawlerToolWithContext(t, ctx, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":                   site.URLFor(crawlertest.RootPath),
		"reqs-max":               120,
		"urls-max":               200,
		"max-retry":              0,
		"max-depth":              10,
		"timeout":                2,
		"https-fallback":         false,
		"forbid-for-parent-path": "yes",
		"ai-js":                  "yes",
		"ai-js-max-requests":     16,
	})
	elapsed := time.Since(started)
	assert.NilError(t, err, "stderr=%s", stderr)
	assert.Assert(t, elapsed < 10*time.Second, "AID layered crawl exceeded 10s: %s", elapsed)
	assert.NilError(t, parent.Err(), "nine-second safety context fired before completion")

	metricsMu.Lock()
	aiCallCount := aiCalls
	maxPayloadBytes := maxPayload
	matchedSnapshot := make(map[string]int, len(matchedSources))
	for source, count := range matchedSources {
		matchedSnapshot[source] = count
	}
	violation := payloadViolation
	metricsMu.Unlock()
	assert.Equal(t, violation, "")
	assert.Assert(t, aiCallCount > 0 && aiCallCount <= 16, "AI calls=%d exceeded explicit budget", aiCallCount)
	assert.Assert(t, maxPayloadBytes < crawlertest.HugeJSSize/20,
		"max AI payload %d effectively forwarded the oversized bundle", maxPayloadBytes)
	for source := range requiredSources {
		assert.Assert(t, matchedSnapshot[source] > 0, "mock received no source-specific evidence for %s", source)
	}
	assert.Assert(t, strings.Contains(stdout, "AI call budget:   16"), "explicit full-fixture budget missing:\n%s", stdout)

	candidateOutput := simpleCrawlerOutputSection(stdout, "=== Requested URLs ===", "=== JavaScript Request Surfaces ===")
	structuredOutput := simpleCrawlerOutputSection(stdout, "=== JavaScript Request Surfaces ===", "=== Rendering Assessment ===")
	observedCandidates := 0
	observedRequests := 0
	observedStructured := 0
	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || !finding.MustDiscover {
			continue
		}
		expectedURL := layeredSiteFindingURL(site, finding)
		if finding.MustCandidate {
			assert.Assert(t, strings.Contains(candidateOutput, expectedURL),
				"candidate layer missing %s (%s)", finding.ID, expectedURL)
			observedCandidates++
		}
		if finding.MustRequest {
			matched := false
			for _, request := range site.Requests() {
				if request.Scope == crawlertest.ScopeInternal && request.Path == finding.Value &&
					request.Method == finding.Method && request.RawQuery == finding.RawQuery {
					matched = true
					break
				}
			}
			assert.Assert(t, matched, "requested layer missing safe request %s", finding.ID)
			observedRequests++
		}
		if finding.MustStructure {
			assert.Assert(t, strings.Contains(structuredOutput, "] ["+finding.Method+"] "+expectedURL),
				"structured layer missing method/URL for %s", finding.ID)
			lowerStructured := strings.ToLower(structuredOutput)
			for key, value := range finding.Headers {
				assert.Assert(t, strings.Contains(lowerStructured, strings.ToLower(key+": "+value)),
					"structured layer missing header %s for %s", key, finding.ID)
			}
			if finding.Body != "" {
				assert.Assert(t, strings.Contains(structuredOutput, "Body: "+finding.Body),
					"structured layer missing body for %s", finding.ID)
			}
			assert.Assert(t, strings.Contains(structuredOutput, "Source: "+site.URLFor(finding.SourceAsset)),
				"structured layer missing source provenance for %s", finding.ID)
			observedStructured++
		}
	}
	coverage := site.GroundTruth.CoverageCounts()
	assert.Equal(t, observedCandidates, coverage[crawlertest.CoverageCandidate])
	assert.Equal(t, observedRequests, coverage[crawlertest.CoverageRequested])
	assert.Equal(t, observedStructured, coverage[crawlertest.CoverageStructured])

	for _, finding := range site.GroundTruth.Findings {
		if finding.Scope != crawlertest.ScopeInternal || finding.Kind != crawlertest.FindingRequest || finding.MustRequest {
			continue
		}
		assert.Equal(t, site.RequestCount(crawlertest.ScopeInternal, finding.Value), 0,
			"structured-only request must not execute: %s", finding.ID)
	}
	assert.Equal(t, site.RequestCount(crawlertest.ScopeInternal, crawlertest.HugeJSPath), 1)
	assert.Equal(t, site.RequestCount(crawlertest.ScopeExternal, crawlertest.ExternalScriptPath), 0)
	t.Logf("AID layered deterministic pipeline elapsed=%s HTTP=%d AI=%d max_payload=%d coverage=%v",
		elapsed, len(site.Requests()), aiCallCount, maxPayloadBytes, coverage)
}

func layeredSiteFindingURL(site *crawlertest.Site, finding crawlertest.ExpectedFinding) string {
	raw := site.URLFor(finding.Value)
	if finding.RawQuery != "" {
		raw += "?" + finding.RawQuery
	}
	return raw
}

func simpleCrawlerOutputSection(output, startMarker, endMarker string) string {
	start := strings.Index(output, startMarker)
	if start < 0 {
		return ""
	}
	section := output[start+len(startMarker):]
	if end := strings.Index(section, endMarker); end >= 0 {
		section = section[:end]
	}
	return section
}

func TestSimpleCrawler_AIJSNoDisablesAdaptiveInvoker(t *testing.T) {
	var aiCalls atomic.Int64
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><body><script src="/assets/disabled.chunk.js"></script></body></html>`))
		case "/assets/disabled.chunk.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write([]byte(`const runtimeConfig={apiBase:atob("L2FwaQ==")};fetch(runtimeConfig.apiBase+window.__tenant);`))
		default:
			http.NotFound(w, r)
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)
	ctx := crawler.WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		aiCalls.Add(1)
		return nil
	})

	stdout, stderr, err := execCrawlerToolWithContext(t, ctx, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  4,
		"max-depth": 1,
		"timeout":   3,
		"ai-js":     "no",
	})
	assert.NilError(t, err, "stderr=%s", stderr)
	assert.Equal(t, aiCalls.Load(), int64(0), "ai-js=no must not invoke AI")
	assert.Assert(t, strings.Contains(stdout, "JavaScript:      disabled"), "disabled mode should be reported:\n%s", stdout)
}

func TestSimpleCrawler_RejectsOutOfRangeAIRequestBudget(t *testing.T) {
	for _, budget := range []int{-1, 21} {
		t.Run(strconv.Itoa(budget), func(t *testing.T) {
			_, _, err := execCrawlerToolWithContext(t, context.Background(), getSimpleCrawlerTool(t), aitool.InvokeParams{
				"urls":               "http://127.0.0.1:1/",
				"ai-js-max-requests": budget,
			})
			assert.Assert(t, err != nil, "an out-of-range AI request budget must fail before crawling")
		})
	}
}

func TestSimpleCrawler_ParentDeadlineCancelsContextScopedAIInvoker(t *testing.T) {
	var aiCalls atomic.Int64
	var observedCancellation atomic.Bool

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><body><div id="app"></div><script src="/assets/cancel.chunk.js"></script></body></html>`))
		case "/assets/cancel.chunk.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write([]byte(`const runtimeConfig={apiBase:atob("L2FwaQ==")};fetch(runtimeConfig.apiBase+window.__tenant);`))
		default:
			http.NotFound(w, r)
		}
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)
	tool := getSimpleCrawlerTool(t)

	// Cancel only after the invoker is reached. A wall-clock deadline started
	// before Yak compilation makes this test depend on cold CI startup time and
	// can expire before the crawler has exercised the propagation path at all.
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := crawler.WithAIJSInvokerContext(parent, func(
		ctx context.Context,
		cfg *crawler.AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		aiCalls.Add(1)
		cancel()
		<-ctx.Done()
		observedCancellation.Store(true)
		return ctx.Err()
	})

	started := time.Now()
	_, _, _ = execCrawlerToolWithContext(t, ctx, tool, aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  6,
		"max-depth": 1,
		"timeout":   3,
	})
	elapsed := time.Since(started)

	assert.Equal(t, aiCalls.Load(), int64(1), "adaptive analysis should reach the context-scoped mock")
	assert.Assert(t, observedCancellation.Load(), "parent deadline should reach the AI invoker")
	assert.Assert(t, elapsed < 3*time.Second, "deadline propagation took too long: %s", elapsed)
}

// TestSimpleCrawler_CoverageHint nudges the AI toward broad coverage via
// adjust_todo when the crawl discovers >1 subdomain or pending URLs.
//
// This is the fix for a field coverage gap: the crawler found multiple
// subdomains and many URLs, but the AI fixated on ONE subdomain (desk) and
// skipped the rest (mall, id/portal). Rather than hard-coding an attack-surface
// inventory (over-fit to one shape), the crawler now emits a lightweight hint
// pushing the AI to break the remaining surface into multiple todos, so more
// todos drive better depth and completeness.
func TestSimpleCrawler_CoverageHint(t *testing.T) {
	// port is assigned by DebugMockHTTPHandlerFunc below; capture it for the
	// handler closure via a pointer so the landing page can self-link.
	var port int
	host, p := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Landing page links to several internal paths and two other virtual
		// hostnames on the same loopback server, so the crawler discovers
		// multiple subdomains and pending URLs.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><body>
<a href="/portal">portal</a>
<a href="/home">home</a>
<a href="/api/v1/users">users api</a>
<a href="/static/js/app.js">js</a>
<a href="http://desk.localhost:` + strconv.Itoa(port) + `/desk">desk</a>
<a href="http://mall.localhost:` + strconv.Itoa(port) + `/mall">mall</a>
</body></html>`))
	})
	port = p
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getSimpleCrawlerTool(t)
	stdout, _ := execCrawlerTool(t, tool, aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  20,
		"max-depth": 3,
		"timeout":   5,
	})

	// The coverage hint must be emitted (multiple subdomains were discovered).
	assert.Assert(t, strings.Contains(stdout, "[coverage hint]"),
		"should emit the [coverage hint] when >1 subdomain or pending URLs are found; got:\n%s", stdout)

	// The hint must steer toward adjust_todo / multiple todos (not a fixed inventory).
	assert.Assert(t, strings.Contains(stdout, "adjust_todo"),
		"hint should steer the AI toward adjust_todo; got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "todo"),
		"hint should mention breaking work into todos")

	// The crawler output should separate requested and merely discovered URLs,
	// without the noisy duplicated website tree.
	assert.Assert(t, strings.Contains(stdout, "=== Crawl Summary ==="),
		"standard crawl summary should still be present")
	assert.Assert(t, strings.Contains(stdout, "=== Requested URLs ==="),
		"requested URLs section should still be present")
	assert.Assert(t, strings.Contains(stdout, "=== Found URLs ==="),
		"found URLs section should be present")
	assert.Assert(t, strings.Contains(stdout, "=== Follow-up Guidance ==="),
		"follow-up guidance should be present")
	assert.Assert(t, !strings.Contains(stdout, "Website Forest:"),
		"website forest should no longer be emitted")

	// Requested URL records carry compact response metadata useful for triage.
	assert.Assert(t, strings.Contains(stdout, "[200 OK]"),
		"requested URL should include status code and text; got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "type=text/html; charset=utf-8"),
		"requested URL should include content type; got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "bytes="),
		"requested URL should include response body size; got:\n%s", stdout)

	// Guidance must explain how to crawl selected discoveries more deeply.
	assert.Assert(t, strings.Contains(stdout, "Found URLs") && strings.Contains(stdout, "`urls`"),
		"guidance should tell the AI to re-crawl selected Found URLs")
	assert.Assert(t, strings.Contains(stdout, "`reqs-max`") && strings.Contains(stdout, "`max-depth`"),
		"guidance should explain request/depth controls")
}

func TestSimpleCrawler_RedactsDisplayURLsWithoutChangingCrawlIdentity(t *testing.T) {
	var receivedMu sync.Mutex
	receivedRawQuery := ""

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		receivedMu.Lock()
		receivedRawQuery = r.URL.RawQuery
		receivedMu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Location", "/landing?signature=redirect-secret&keep=visible")
		_, _ = w.Write([]byte(`<!doctype html><html><body><div id="root"></div>
<script type="module">globalThis.appReady=true</script>
<a href="/pending?token=pending-one&amp;keep=visible">one</a>
<a href="/pending?token=pending-two&amp;keep=visible">two</a>
</body></html>`))
	})
	seedURL := "http://display-user:display-pass@" + host + ":" + strconv.Itoa(port) + "/?Token=seed-secret&keep=visible"

	stdout, stderr := execCrawlerTool(t, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":           seedURL,
		"reqs-max":       1,
		"urls-max":       10,
		"max-depth":      2,
		"timeout":        3,
		"https-fallback": false,
		"ai-js":          "no",
	})

	receivedMu.Lock()
	gotRawQuery := receivedRawQuery
	receivedMu.Unlock()
	assert.Equal(t, gotRawQuery, "Token=seed-secret&keep=visible",
		"display redaction must not mutate the real request target")

	visible := stdout + "\n" + stderr
	for _, leaked := range []string{
		"display-user", "display-pass", "seed-secret", "pending-one", "pending-two", "redirect-secret",
	} {
		assert.Assert(t, !strings.Contains(visible, leaked), "crawler display leaked %q:\n%s", leaked, visible)
	}
	for _, expected := range []string{
		"Token=[REDACTED]&keep=visible",
		"token=[REDACTED]&keep=visible",
		"location=/landing?signature=[REDACTED]&keep=visible",
		"URLs discovered: 3",
		"Requests sent:   1",
		"Pending:         2",
		"likely client-rendered SPA / JavaScript-heavy page",
	} {
		assert.Assert(t, strings.Contains(stdout, expected), "crawler display lost %q:\n%s", expected, stdout)
	}
	assert.Equal(t, strings.Count(stdout, "/pending?token=[REDACTED]&keep=visible"), 2,
		"two raw token variants must remain two discovery identities even when their display forms match")
}

func TestSimpleCrawler_RejectsScopeDomainOutputControls(t *testing.T) {
	forged := "allowed.example\u2028=== FORGED SECTION ==="
	stdout, stderr, err := execCrawlerToolWithContext(t, context.Background(), getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":          "http://127.0.0.1/",
		"scope-domains": forged,
		"ai-js":         "no",
	})
	if err == nil {
		t.Fatal("scope domain containing a Unicode line separator was accepted")
	}
	visible := stdout + "\n" + stderr + "\n" + err.Error()
	if strings.Contains(visible, forged) || strings.Contains(visible, "\u2028") {
		t.Fatalf("invalid scope-domain control was reflected into output: %q", visible)
	}
	if !strings.Contains(visible, "control characters") {
		t.Fatalf("scope-domain rejection was not actionable: %q", visible)
	}
}

func TestSimpleCrawler_HidesAndDoesNotRequestURLFragments(t *testing.T) {
	var port int
	var fragmentRequestCount atomic.Int64
	host, p := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "#") {
			fragmentRequestCount.Add(1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/" {
			w.Write([]byte(`<a href="#overview">overview</a>
<a href="/home#section-systems">systems</a>
<a href="/home#section-energy">energy</a>`))
			return
		}
		w.Write([]byte("ok"))
	})
	port = p
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	tool := getSimpleCrawlerTool(t)
	stdout, _ := execCrawlerTool(t, tool, aitool.InvokeParams{
		"urls":                   baseURL,
		"reqs-max":               20,
		"max-depth":              3,
		"timeout":                5,
		"forbid-for-parent-path": "yes",
	})

	assert.Equal(t, fragmentRequestCount.Load(), int64(0), "crawler must not send URL fragments to the server")
	assert.Assert(t, strings.Contains(stdout, "Requests sent:   2"),
		"fragment variants should not consume the request budget; got:\n%s", stdout)
	assert.Assert(t, !strings.Contains(stdout, "#section-"),
		"fragment URL should not be shown in crawler output; got:\n%s", stdout)
	assert.Assert(t, !strings.Contains(stdout, "404 Not Found"),
		"fragment-only 404 should not be shown in crawler output; got:\n%s", stdout)
}

func TestSimpleCrawler_ClientRenderingClassificationFixtures(t *testing.T) {
	type fixture struct {
		name        string
		contentType string
		body        string
		want        string
	}
	fixtures := []fixture{
		{
			name:        "react-webpack-shell",
			contentType: "application/octet-stream",
			body: `<!doctype html><html><head><title>Console</title></head><body>
<div id="root"></div><script>window.webpackChunkconsole=[]</script>
<script src="/assets/runtime.aa.js"></script><script src="/assets/app.bb.js"></script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "vue-vite-shell",
			contentType: "text/html",
			body:        `<!doctype html><html><body><div id="app" data-v-app></div><script type="module" src="/assets/index-a1b2.js"></script></body></html>`,
			want:        "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "angular-shell",
			contentType: "text/html",
			body: `<!doctype html><html><body><app-root ng-version="17"><router-outlet></router-outlet></app-root>
<script src="/runtime.123.js"></script><script src="/main.456.js"></script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "next-shell",
			contentType: "text/html",
			body: `<!doctype html><html><body><div id="__next"></div><script id="__NEXT_DATA__" type="application/json">{}</script>
<script src="/_next/static/chunks/main.js"></script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "nuxt-shell",
			contentType: "text/html",
			body: `<!doctype html><html><body><div id="__nuxt"></div><script>window.__NUXT__={}</script>
<script type="module" src="/_nuxt/entry.js"></script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "sveltekit-shell",
			contentType: "text/html",
			body: `<!doctype html><html><body><div id="svelte" data-sveltekit-preload-data="hover"></div>
<script type="module" src="/assets/index-cafe.js"></script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "generic-client-router-shell",
			contentType: "text/html",
			body: `<!doctype html><html><body><div id="app"></div><script type="module">
history.pushState({}, "", "/dashboard"); fetch("/api/v1/me")</script></body></html>`,
			want: "likely client-rendered SPA / JavaScript-heavy page",
		},
		{
			name:        "hydrated-rich-ssr",
			contentType: "text/html",
			body: `<!doctype html><html><body><div id="__next"><main><h1>Rendered documentation</h1><p>` +
				strings.Repeat("This response already contains meaningful server-rendered product and security documentation. ", 8) +
				`</p><a href="/guide">Guide</a><form action="/search"><input name="q"></form></main></div>
<script id="__NEXT_DATA__">{}</script><script src="/_next/static/chunks/main.js"></script></body></html>`,
			want: "possible client-rendered page",
		},
		{
			name:        "server-rendered-docs",
			contentType: "text/html; charset=utf-8",
			body: `<!doctype html><html><body><main><h1>Documentation</h1><p>Server-rendered content.</p>
<a href="/guide">Guide</a><a href="/api">API</a><form action="/search"><input name="q"></form></main></body></html>`,
			want: "no strong client-rendering dependency detected",
		},
		{
			name:        "traditional-ssr-with-scripts",
			contentType: "text/html",
			body: `<!doctype html><html><body><article><h1>News</h1><p>` +
				strings.Repeat("This article and its navigation are fully present in the original HTML response. ", 8) +
				`</p><a href="/archive">Archive</a></article><script src="/jquery.js"></script>
<script src="/analytics.js"></script><script src="/widgets.js"></script></body></html>`,
			want: "no strong client-rendering dependency detected",
		},
	}

	fixtureByPath := make(map[string]fixture, len(fixtures))
	for _, tc := range fixtures {
		fixtureByPath["/"+tc.name] = tc
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc, ok := fixtureByPath[r.URL.Path]; ok {
			w.Header().Set("Content-Type", tc.contentType)
			_, _ = w.Write([]byte(tc.body))
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`console.log("fixture");`))
			return
		}
		http.NotFound(w, r)
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)
	tool := getSimpleCrawlerTool(t)

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _ := execCrawlerTool(t, tool, aitool.InvokeParams{
				"urls":                   baseURL + "/" + tc.name,
				"reqs-max":               8,
				"max-depth":              1,
				"timeout":                5,
				"forbid-for-parent-path": "yes",
			})
			assert.Assert(t, strings.Contains(stdout, "=== Rendering Assessment ==="),
				"crawler should emit a rendering assessment; got:\n%s", stdout)
			assert.Assert(t, strings.Contains(stdout, tc.want),
				"classification mismatch, want %q; got:\n%s", tc.want, stdout)
		})
	}
}

func TestSimpleCrawler_SPAAdviceStopsRepeatedCrawling(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body><div id="root"></div>
<script>window.webpackChunkapp=[]; history.pushState({}, "", "/dashboard")</script>
<script type="module" src="/assets/index-deadbeef.js"></script></body></html>`))
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	stdout, _ := execCrawlerTool(t, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  20,
		"max-depth": 3,
		"timeout":   5,
	})

	for _, want := range []string{
		"does not execute JavaScript",
		"Stop mechanically deepening this crawl",
		"`use_browser` (`op=open`)",
		"referenced JavaScript assets",
		"direct HTTP request",
	} {
		assert.Assert(t, strings.Contains(stdout, want), "SPA advice missing %q; got:\n%s", want, stdout)
	}
	assert.Assert(t, !strings.Contains(stdout, "[coverage hint]"),
		"a SPA shell must not trigger advice to deepen the same crawler; got:\n%s", stdout)
}

func TestSimpleCrawler_BoundsRenderingAssessmentWork(t *testing.T) {
	var landing strings.Builder
	landing.WriteString(`<!doctype html><html><body>`)
	for i := 0; i < 25; i++ {
		landing.WriteString(`<a href="/page/` + strconv.Itoa(i) + `">page</a>`)
	}
	landing.WriteString(`</body></html>`)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(landing.String()))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><html><body><main>plain server page</main></body></html>`))
	})
	baseURL := "http://" + host + ":" + strconv.Itoa(port)

	stdout, _ := execCrawlerTool(t, getSimpleCrawlerTool(t), aitool.InvokeParams{
		"urls":      baseURL,
		"reqs-max":  30,
		"max-depth": 2,
		"timeout":   5,
	})

	assert.Assert(t, strings.Contains(stdout, "Classification: no strong client-rendering dependency detected in 20 assessed HTML response(s)"),
		"rendering assessment should stop after 20 HTML responses; got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "Assessment budget: sampled the first 20 of "),
		"rendering assessment should report its stable work cap without depending on concurrent crawl completion order; got:\n%s", stdout)
}
