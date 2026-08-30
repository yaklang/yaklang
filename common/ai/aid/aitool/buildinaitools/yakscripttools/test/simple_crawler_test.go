package test

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/crawler"
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
		assert.Assert(t, strings.Contains(payload, "runtimeConfig"), "mock should receive the bounded trigger evidence")
		assert.Assert(t, len(payload) < 256*1024, "adaptive payload must stay bounded, got %d bytes", len(payload))
		onPath(baseURL + "/api/context-mock-only")
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
		"=== Rendering Assessment ===",
		"=== Follow-up Guidance ===",
	} {
		assert.Assert(t, strings.Contains(stdout, section), "legacy output section missing: %s", section)
	}
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
