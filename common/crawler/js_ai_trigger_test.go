package crawler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func triggerSignalsContain(got []string, want string) bool {
	for _, signal := range got {
		if signal == want {
			return true
		}
	}
	return false
}

func TestAssessAIJSTriggerPositiveMatrix(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		sourceURL   string
		contentType string
		wantSignals []string
	}{
		{
			name:        "case-insensitive-call",
			code:        `const endpoint = route; FETCH(endpoint);`,
			wantSignals: []string{"request-sink", "dynamic-request-expression"},
		},
		{
			name:        "minified-no-space-call",
			code:        `let endpoint=base+path;fetch(endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "string-assembly"},
		},
		{
			name: "newline-separated-argument",
			code: "fetch(\n\tendpoint\n)",
			wantSignals: []string{
				"request-sink",
				"dynamic-request-expression",
			},
		},
		{
			name:        "member-bracket-fetch",
			code:        `globalThis["fetch"](endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression"},
		},
		{
			name:        "member-bracket-axios-method",
			code:        `axios['post'](endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression"},
		},
		{
			name:        "unary-conditional-expression",
			code:        `fetch(!enabled?fallback:route)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression"},
		},
		{
			name:        "hex-escaped-path",
			code:        `fetch("\x2fapi\x2fhidden")`,
			wantSignals: []string{"request-sink", "encoded-or-obfuscated"},
		},
		{
			name:        "unicode-escaped-member",
			code:        `globalThis["\u0066etch"](endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "encoded-or-obfuscated"},
		},
		{
			name:        "atob-request-expression",
			code:        `const endpoint=atob(encoded);fetch(endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "encoded-or-obfuscated"},
		},
		{
			name:        "from-char-code-request-expression",
			code:        `const endpoint=String.fromCharCode(47,97,112,105);fetch(endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "encoded-or-obfuscated"},
		},
		{
			name:        "array-join-request-expression",
			code:        `const endpoint=["/api","/joined"].join("");fetch(endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "string-assembly"},
		},
		{
			name:        "webpack-chunk-request-expression",
			code:        `const endpoint=__webpack_require__.u(chunkId);fetch(endpoint)`,
			wantSignals: []string{"request-sink", "dynamic-request-expression", "compiled-chunk-runtime"},
		},
		{
			name: "routes-text-field-composition",
			code: "# generated route registry\n" +
				"base=/service\nversion=/v1\nresource=/routes\naction=/export\n" +
				"method=GET\nnext=/.config/runtime.config\n",
			sourceURL:   "https://example.test/routes.txt",
			contentType: "text/plain",
			wantSignals: []string{"route-field-composition", "config-asset"},
		},
		{
			name:        "json-unicode-runtime-config",
			code:        `{"service_segments":["/gateway","/v1","/runtime","/bootstrap"],"method":["PO","ST"],"chunk_config":"\u002fassets\u002f.config\u002fchunks.config"}`,
			sourceURL:   "https://example.test/.config/runtime.config",
			contentType: "application/json",
			wantSignals: []string{"encoded-or-obfuscated", "config-asset"},
		},
		{
			name:        "json-unicode-chunk-config",
			code:        `{"chunks":{"713":"\u002fassets\u002fchunks\u002f713.compiled.js"},"load_order":[713]}`,
			sourceURL:   "https://example.test/assets/.config/chunks.config",
			contentType: "application/json",
			wantSignals: []string{"encoded-or-obfuscated", "config-asset"},
		},
		{
			name:        "compiled-dynamic-import",
			code:        `(()=>{const _0x8a=["bm9pc2U=","L2Fzc2V0cy9hcHAuaHVnZS5qcw=="];const _0x4d=i=>atob(_0x8a[i^1]);import(_0x4d(0));})();`,
			wantSignals: []string{"dynamic-module-expression", "encoded-or-obfuscated"},
		},
		{
			name:        "compiled-dynamic-require",
			code:        `(()=>{const _0x4d=i=>atob(table[i]);require(_0x4d(0));})();`,
			wantSignals: []string{"dynamic-module-expression", "encoded-or-obfuscated"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceURL := test.sourceURL
			if sourceURL == "" {
				sourceURL = "https://example.test/assets/app.js"
			}
			contentType := test.contentType
			if contentType == "" {
				contentType = "application/javascript"
			}
			got := assessAIJSTrigger(test.code, sourceURL, contentType)
			if got.score < 3 {
				t.Fatalf("expected trigger score >= 3, got score=%d signals=%v", got.score, got.signals)
			}
			for _, want := range test.wantSignals {
				if !triggerSignalsContain(got.signals, want) {
					t.Errorf("missing signal %q: score=%d signals=%v", want, got.score, got.signals)
				}
			}
		})
	}
}

func TestAssessAIJSTriggerNegativeMatrix(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "prefetch-is-not-fetch",
			code: `const endpoint = route;
prefetch(endpoint);`,
		},
		{
			name: "comment-only",
			code: `// fetch(endpoint); atob(secret); __webpack_require__.u(id)
/* axios.post(endpoint); String.fromCharCode(47); webpackChunkapp.push([]); */
const inert = 1;`,
		},
		{
			name: "string-noise",
			code: `const documentation = "fetch(endpoint) atob(secret) __webpack_require__.u(id)";
const sample = 'axios.post(route); webpackChunkapp.push([])';`,
		},
		{
			name: "escaped-string-noise",
			code: `const documentation = "globalThis['\u0066etch'](endpoint); fetch('\\x2fapi')";`,
		},
		{
			name: "identifier-boundaries",
			code: `myfetch(endpoint);
fetcher(endpoint);
notsendBeacon(endpoint);
my__webpack_require__.u(chunkId);`,
		},
		{
			name: "literal-request-needs-no-ai",
			code: `fetch("/api/v1/users")`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := assessAIJSTrigger(test.code, "https://example.test/assets/app.js", "application/javascript")
			if got.score >= 3 {
				t.Fatalf("noise must remain below adaptive threshold: score=%d signals=%v", got.score, got.signals)
			}
		})
	}
}

func TestCompiledChunkDynamicModuleProducesCandidateWindow(t *testing.T) {
	tests := []struct {
		name string
		code string
		call string
	}{
		{
			name: "import-expression",
			code: `(()=>{const _0x8a=["bm9pc2U=","L2Fzc2V0cy9hcHAuaHVnZS5qcw=="];const _0x4d=i=>atob(_0x8a[i^1]);import(_0x4d(0));})();`,
			call: `import(_0x4d(0))`,
		},
		{
			name: "require-expression",
			code: `(()=>{const _0x4d=i=>atob(table[i]);require(_0x4d(0));})();`,
			call: `require(_0x4d(0))`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment := assessAIJSTrigger(test.code, "https://example.test/assets/chunks/runtime.js", "application/javascript")
			if assessment.score < 3 || !triggerSignalsContain(assessment.signals, "dynamic-module-expression") {
				t.Fatalf("dynamic module must trigger AI: score=%d signals=%v", assessment.score, assessment.signals)
			}
			windows := extractAdaptiveURLLikeCandidatesBounded(test.code, 120, 8, 4096)
			if len(windows) == 0 {
				t.Fatal("compiled dynamic module produced no bounded candidate window")
			}
			if !strings.Contains(strings.Join(windows, "\n"), test.call) {
				t.Fatalf("candidate windows lost module call %q: %q", test.call, windows)
			}
		})
	}
}

func TestAIJSInvokerContextIsolation(t *testing.T) {
	type result struct {
		name string
		path string
	}

	results := make(chan result, 2)
	run := func(name, path string) {
		ctx := WithAIJSInvokerContext(context.Background(), func(
			ctx context.Context,
			cfg *AIJSExtractConfig,
			payload string,
			onPath func(string),
		) error {
			if payload != name {
				t.Errorf("%s invoker received payload %q", name, payload)
			}
			onPath(path)
			return nil
		})
		cfg := NewAIJSExtractConfig()
		cfg.runtimeBudget = newAIJSCallBudget(1)
		if !invokeAIJSWithBudget(ctx, cfg, name, func(got string) {
			results <- result{name: name, path: got}
		}) {
			t.Errorf("%s context invoker was not attempted", name)
		}
	}

	var wg sync.WaitGroup
	for _, item := range []result{{name: "ctx-a", path: "/from/context/a"}, {name: "ctx-b", path: "/from/context/b"}} {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(item.name, item.path)
		}()
	}
	wg.Wait()
	close(results)

	got := make(map[string]string)
	for item := range results {
		got[item.name] = item.path
	}
	if got["ctx-a"] != "/from/context/a" || got["ctx-b"] != "/from/context/b" {
		t.Fatalf("context-scoped invokers crossed: %#v", got)
	}
}

func TestAIJSInvokerConfigTakesPrecedenceOverContext(t *testing.T) {
	var contextCalls atomic.Int64
	ctx := WithAIJSInvokerContext(context.Background(), func(
		context.Context,
		*AIJSExtractConfig,
		string,
		func(string),
	) error {
		contextCalls.Add(1)
		return nil
	})

	var configCalls atomic.Int64
	cfg := NewAIJSExtractConfig(withAIJSInvoker(func(
		context.Context,
		*AIJSExtractConfig,
		string,
		func(string),
	) error {
		configCalls.Add(1)
		return nil
	}))
	cfg.runtimeBudget = newAIJSCallBudget(1)

	if !invokeAIJSWithBudget(ctx, cfg, "payload", func(string) {}) {
		t.Fatal("config-scoped invoker was not attempted")
	}
	if got := configCalls.Load(); got != 1 {
		t.Fatalf("config invoker calls=%d, want 1", got)
	}
	if got := contextCalls.Load(); got != 0 {
		t.Fatalf("context invoker calls=%d, want 0", got)
	}
}

func TestAIJSInvokerBudgetConcurrent(t *testing.T) {
	const (
		limit    = 3
		attempts = 32
	)

	var invocations atomic.Int64
	ctx := WithAIJSInvokerContext(context.Background(), func(
		context.Context,
		*AIJSExtractConfig,
		string,
		func(string),
	) error {
		invocations.Add(1)
		return nil
	})
	cfg := NewAIJSExtractConfig()
	cfg.runtimeBudget = newAIJSCallBudget(limit)

	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if invokeAIJSWithBudget(ctx, cfg, "payload", func(string) {}) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := accepted.Load(); got != limit {
		t.Fatalf("accepted calls=%d, want %d", got, limit)
	}
	if got := invocations.Load(); got != limit {
		t.Fatalf("invoker calls=%d, want %d", got, limit)
	}
}

func TestRunAIJSExtractAssetsSharesContextInvokerBudget(t *testing.T) {
	var calls atomic.Int64
	ctx := WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		calls.Add(1)
		onPath("/api/from-context-mock")
		return nil
	})

	var eventsMu sync.Mutex
	var events []AIJSExtractEvent
	cfg := NewAIJSExtractConfig(
		WithAIJS_AdaptiveTrigger(),
		WithAIJS_MaxRequests(1),
		WithAIJS_Observer(func(event AIJSExtractEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, event)
		}),
	)
	assets := []AIJSAsset{
		{SourceURL: "https://example.test/a.js", ContentType: "application/javascript", Body: `fetch(endpointA)`},
		{SourceURL: "https://example.test/b.js", ContentType: "application/javascript", Body: `fetch(endpointB)`},
		{SourceURL: "https://example.test/c.js", ContentType: "application/javascript", Body: `fetch(endpointC)`},
	}

	if err := RunAIJSExtractAssets(ctx, assets, cfg, func(string) {}); err != nil {
		t.Fatalf("RunAIJSExtractAssets failed: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("AI calls=%d, want one shared-budget call", got)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if len(events) != len(assets) {
		t.Fatalf("observer events=%d, want %d: %#v", len(events), len(assets), events)
	}
	requests := 0
	seenSources := make(map[string]bool)
	for _, event := range events {
		requests += event.AIRequests
		seenSources[event.SourceURL] = true
	}
	if requests != 1 {
		t.Fatalf("event AI request total=%d, want 1: %#v", requests, events)
	}
	for _, asset := range assets {
		if !seenSources[asset.SourceURL] {
			t.Errorf("missing observer provenance for %s", asset.SourceURL)
		}
	}
}

func TestAIJSInvokerCallTimeout(t *testing.T) {
	observed := make(chan error, 1)
	ctx := WithAIJSInvokerContext(context.Background(), func(
		ctx context.Context,
		cfg *AIJSExtractConfig,
		payload string,
		onPath func(string),
	) error {
		<-ctx.Done()
		observed <- ctx.Err()
		return ctx.Err()
	})
	cfg := NewAIJSExtractConfig()
	cfg.CallTimeout = 20 * time.Millisecond
	cfg.runtimeBudget = newAIJSCallBudget(1)

	started := time.Now()
	if !invokeAIJSWithBudget(ctx, cfg, "payload", func(string) {}) {
		t.Fatal("timed invocation should count as an attempted AI request")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("AI call timeout was not enforced, elapsed=%s", elapsed)
	}

	select {
	case err := <-observed:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("invoker context error=%v, want DeadlineExceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("invoker did not observe its call timeout")
	}
}
