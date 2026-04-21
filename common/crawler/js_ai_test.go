package crawler

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// helper: returns true if any block in the candidate stream contains needle
func containsHit(blocks []string, needle string) bool {
	for _, b := range blocks {
		if strings.Contains(b, needle) {
			return true
		}
	}
	return false
}

func TestExtractURLLikeCandidates_BasicHits(t *testing.T) {
	src := `
		// boot routes
		const r1 = "/api/v1/login";
		fetch('/api/v1/users?id=1', {method: 'GET'});
		const cdn = "https://example.com/static/app.js?v=2";
		import("./chunk.async.123.js");
		const ver = "1.2.3"; // version, not a path
	`
	blocks := extractURLLikeCandidates(src, 60)
	assert.NotEmpty(t, blocks)
	assert.True(t, containsHit(blocks, "/api/v1/login"))
	assert.True(t, containsHit(blocks, "/api/v1/users"))
	assert.True(t, containsHit(blocks, "https://example.com/static/app.js"))
	assert.True(t, containsHit(blocks, "chunk.async.123.js"))

	for _, b := range blocks {
		assert.Contains(t, b, "--- candidate ---")
		assert.Contains(t, b, "--- end ---")
	}
}

func TestExtractURLLikeCandidates_EmptyAndNoHits(t *testing.T) {
	assert.Empty(t, extractURLLikeCandidates("", 120))
	assert.Empty(t, extractURLLikeCandidates("hello world, no urls here", 120))
}

func TestExtractURLLikeCandidates_MergesAdjacentWindows(t *testing.T) {
	src := `fetch("/a/b"); fetch("/c/d"); fetch("/e/f");`
	blocks := extractURLLikeCandidates(src, 200)
	assert.Len(t, blocks, 1, "three adjacent hits with large context should collapse into one block")
	assert.True(t, strings.Contains(blocks[0], "/a/b"))
	assert.True(t, strings.Contains(blocks[0], "/c/d"))
	assert.True(t, strings.Contains(blocks[0], "/e/f"))
}

func TestRawCandidateHits_DedupAndStripQuotes(t *testing.T) {
	src := `
		const a = "/foo/bar";
		const b = "/foo/bar";
		const c = '/baz/qux';
	`
	hits := rawCandidateHits(src)
	uniq := map[string]struct{}{}
	for _, h := range hits {
		uniq[h] = struct{}{}
	}
	assert.Contains(t, uniq, "/foo/bar")
	assert.Contains(t, uniq, "/baz/qux")
	for h := range uniq {
		assert.False(t, strings.HasPrefix(h, "'") || strings.HasPrefix(h, "\""), "quotes should be stripped: %q", h)
	}
}

func TestRunAIJSExtract_RawFallbackWhenDirectFeedDisabled(t *testing.T) {
	// When the direct-feed fast path is disabled (SmallInputBytes=0) and the
	// candidate stream is smaller than SkipBelowBytes, AI must NOT be called
	// and raw regex hits are emitted directly.
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var aiCalls int32
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&aiCalls, 1)
		return nil
	}

	cfg := NewAIJSExtractConfig(
		WithAIJS_SmallInputBytes(0),       // disable direct-feed
		WithAIJS_SmallInputTokens(0),      // (both must be disabled together)
		WithAIJS_SkipBelowBytes(1<<20),    // huge => always under threshold
	)
	var got []string
	var mu sync.Mutex
	err := RunAIJSExtract(context.Background(),
		`fetch('/api/v1/login'); fetch('/api/v2/users');`,
		cfg,
		func(p string) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, p)
		},
	)
	assert.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&aiCalls), "AI should not be called on raw-fallback path")

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(got, "|")
	assert.Contains(t, joined, "/api/v1/login")
	assert.Contains(t, joined, "/api/v2/users")
}

func TestRunAIJSExtract_NoCandidatesNoCallOnReducerPath(t *testing.T) {
	// On the reducer path (direct-feed disabled), an input with no regex
	// candidates must not reach the AI step at all.
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var aiCalls int32
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&aiCalls, 1)
		return nil
	}

	cfg := NewAIJSExtractConfig(
		WithAIJS_SmallInputBytes(0),
		WithAIJS_SmallInputTokens(0),
	)
	err := RunAIJSExtract(context.Background(), "no urls here at all just plain prose", cfg, func(p string) {})
	assert.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&aiCalls))
}

func TestRunAIJSExtract_EmptyInputNoCall(t *testing.T) {
	// Empty input must never reach the AI, even on the direct-feed path.
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var aiCalls int32
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&aiCalls, 1)
		return nil
	}
	err := RunAIJSExtract(context.Background(), "", nil, func(p string) {})
	assert.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&aiCalls))
}

func TestRunAIJSExtract_LargeInputSlicedAndFolded(t *testing.T) {
	// Build a payload that is comfortably larger than ChunkBytes so aireducer
	// emits multiple slices; verify that:
	//  - the AI mock is invoked more than once
	//  - DumpWithOverlap markers show up on chunks after the first
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu              sync.Mutex
		payloads        []string
		overlapCount    int
		nonOverlapCount int
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		if strings.Contains(payload, "<|OVERLAP[") && strings.Contains(payload, "<|OVERLAP_END[") {
			overlapCount++
		} else {
			nonOverlapCount++
		}
		// emit one fake path so we can confirm dedup works across slices
		onPath("/from-mock-ai/" + strings.TrimSpace(strings.Split(payload, "\n")[0]))
		return nil
	}

	// Construct ~600KB of pseudo JS with embedded paths.
	var b strings.Builder
	for i := 0; b.Len() < 600*1024; i++ {
		b.WriteString("// fake chunk header\n")
		b.WriteString("const r")
		b.WriteString(strings.Repeat("x", 8))
		b.WriteString(" = '/api/v1/route/")
		b.WriteString(strings.Repeat("p", 32))
		b.WriteString("';\n")
		b.WriteString("/* filler */ ")
		b.WriteString(strings.Repeat(".", 256))
		b.WriteString("\n")
	}
	src := b.String()

	cfg := NewAIJSExtractConfig(
		WithAIJS_ChunkBytes(64*1024),  // small chunks to force multiple slices
		WithAIJS_OverlapBytes(1024),
		WithAIJS_SkipBelowBytes(1024),
		WithAIJS_Concurrency(2),
	)

	var emitted []string
	var emitMu sync.Mutex
	err := RunAIJSExtract(context.Background(), src, cfg, func(p string) {
		emitMu.Lock()
		defer emitMu.Unlock()
		emitted = append(emitted, p)
	})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(payloads), 2, "expected multiple slices for large input, got %d", len(payloads))
	assert.GreaterOrEqual(t, overlapCount, 1, "expected at least one chunk to carry overlap markers")
	assert.GreaterOrEqual(t, nonOverlapCount, 1, "expected the first chunk to have no overlap marker")

	// dedup: even though we emitted one path per slice, paths are unique by
	// content so the count should match the slice count exactly.
	uniq := map[string]struct{}{}
	for _, p := range emitted {
		uniq[p] = struct{}{}
	}
	assert.Equal(t, len(emitted), len(uniq), "RunAIJSExtract must dedup across slices")
}

func TestNewAIJSExtractConfig_Defaults(t *testing.T) {
	c := NewAIJSExtractConfig()
	assert.Equal(t, 80*1024, c.MaxTokens)
	assert.Equal(t, int64(250*1024), c.ChunkBytes)
	assert.Equal(t, 2048, c.OverlapBytes)
	assert.Equal(t, 120, c.ContextBytes)
	assert.Equal(t, 1024, c.SkipBelowBytes)
	assert.Equal(t, 200*1024, c.SmallInputBytes)
	assert.Equal(t, 50*1024, c.SmallInputTokens)
	assert.Equal(t, 2, c.Concurrency)
}

// TestRunAIJSExtract_SlicesFillChunkBytes protects the regression where the
// "--- end ---" separator was interpreted as an every-occurrence trigger, so
// every tiny candidate block became one AI request (~6KB each). After the
// switch to WithSeparatorAsBoundary(true) each slice must pack many candidate
// blocks to approach the configured ChunkBytes.
func TestRunAIJSExtract_SlicesFillChunkBytes(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu       sync.Mutex
		payloads []string
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		return nil
	}

	// Build a realistic-ish webpack-style blob: many small quoted routes
	// separated by noise so that each candidate window is small (~200 bytes)
	// and there are thousands of them; total stream well above ChunkBytes.
	var b strings.Builder
	for i := 0; b.Len() < 1200*1024; i++ {
		b.WriteString("n(123);var X=")
		b.WriteString("'/api/v")
		b.WriteString(strings.Repeat("r", 6))
		b.WriteString("/resource")
		b.WriteString(strings.Repeat("s", 6))
		b.WriteString("';")
		b.WriteString("/* pad */ ")
		b.WriteString(strings.Repeat(".", 200))
		b.WriteString("\n")
	}
	src := b.String()

	chunkBytes := int64(250 * 1024)
	cfg := NewAIJSExtractConfig(
		WithAIJS_ChunkBytes(chunkBytes),
		WithAIJS_MaxTokens(80*1024),
		WithAIJS_OverlapBytes(2048),
		WithAIJS_SkipBelowBytes(1024),
		WithAIJS_Concurrency(1),
	)

	err := RunAIJSExtract(context.Background(), src, cfg, func(p string) {})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(payloads), 2, "expected multiple slices for 1.2MB stream")

	// Each payload should be substantially larger than a single candidate
	// block (~200 bytes). Accept a generous floor (100KB) so the test is not
	// token-shrink brittle; the key regression is the 6KB-per-request bug.
	minBytes := 100 * 1024
	for i, p := range payloads[:len(payloads)-1] { // skip last (tail) slice
		if len(p) < minBytes {
			t.Fatalf("slice %d is only %d bytes, expected >= %d (regression: separator was still emitting one chunk per block)", i, len(p), minBytes)
		}
	}

	// And no slice should wildly exceed ChunkBytes + overlap header budget.
	maxBytes := int(chunkBytes) + 16*1024
	for i, p := range payloads {
		if len(p) > maxBytes {
			t.Fatalf("slice %d is %d bytes, exceeds max %d (chunkBytes + overlap headroom)", i, len(p), maxBytes)
		}
	}
}

func TestNewAIJSExtractConfig_OptionsApplied(t *testing.T) {
	c := NewAIJSExtractConfig(
		WithAIJS_MaxTokens(40000),
		WithAIJS_ChunkBytes(128*1024),
		WithAIJS_OverlapBytes(0),
		WithAIJS_ContextBytes(60),
		WithAIJS_SkipBelowBytes(2048),
		WithAIJS_SmallInputBytes(64*1024),
		WithAIJS_SmallInputTokens(20*1024),
		WithAIJS_Concurrency(4),
	)
	assert.Equal(t, 40000, c.MaxTokens)
	assert.Equal(t, int64(128*1024), c.ChunkBytes)
	assert.Equal(t, 0, c.OverlapBytes)
	assert.Equal(t, 60, c.ContextBytes)
	assert.Equal(t, 2048, c.SkipBelowBytes)
	assert.Equal(t, 64*1024, c.SmallInputBytes)
	assert.Equal(t, 20*1024, c.SmallInputTokens)
	assert.Equal(t, 4, c.Concurrency)
}

func TestSanitizeAIURL_AcceptsValidAbsolute(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://127.0.0.1:8787/api/v1/login", "http://127.0.0.1:8787/api/v1/login"},
		{"  https://Example.com/path?x=1#frag  ", "https://Example.com/path?x=1"},
		{"http://localhost:9000/callback", "http://localhost:9000/callback"},
		{"HTTP://example.com/UpperScheme", "http://example.com/UpperScheme"},
		{"https://[::1]:443/v6", "https://[::1]:443/v6"},
		{"http://momentjs.com/guides/#/warnings/js-date/", "http://momentjs.com/guides/"},
	}
	for _, c := range cases {
		got, ok := sanitizeAIURL(c.in)
		if !ok {
			t.Fatalf("sanitizeAIURL(%q) = false, want ok", c.in)
		}
		if got != c.want {
			t.Fatalf("sanitizeAIURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeAIURL_RejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"  ",
		"http://---html-end---/",
		"http://---js-chunk-end---/",
		"https://yak-html-end/",
		"http://yak-js-end/",
		"javascript:alert(1)",
		"mailto:a@b.c",
		"data:text/plain,abc",
		"//example.com/x",
		"http:///nohost",
		"http://app/internal",
		"http://-bad.example.com/",
		"http://bad-.example.com/",
		"http:// space.com/",
		"http://exa\nmple.com/",
		"not a url at all",
	}
	for _, c := range cases {
		got, ok := sanitizeAIURL(c)
		if ok {
			t.Fatalf("sanitizeAIURL(%q) = (%q, true), want false", c, got)
		}
	}
}

func TestLooksLikeBoundaryLeak(t *testing.T) {
	for _, s := range []string{
		"yak-html-end",
		"YAK-html-end",
		"http://---html-end---/",
		"--- candidate ---",
		"--- end ---",
		"foo /*yak-js-end*/ bar",
		"a---html-end---b",
	} {
		assert.True(t, looksLikeBoundaryLeak(s), "expected leak for %q", s)
	}
	for _, s := range []string{
		"http://example.com/api/v1",
		"/api/users",
		"static/app.js",
		"https://x.test/path?yak=html",
	} {
		assert.False(t, looksLikeBoundaryLeak(s), "expected NO leak for %q", s)
	}
}

func TestRunAIJSExtract_DropsBoundaryMarkerLeaks(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	// Mock the AI step: emit a mix of good URLs and known bad strings that
	// reproduce the regression seen in /tmp/c.txt (boundary marker leaked
	// as "http://---html-end---/", fragment-only links, garbage host).
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		for _, u := range []string{
			"http://---html-end---/",
			"http://yak-html-end/",
			"https://yak-js-end/foo",
			"http://app/internal",
			"javascript:alert(1)",
			"http://127.0.0.1:8787/api/good",
			"http://momentjs.com/guides/#/warnings/js-date/",
			"http://127.0.0.1:8787/api/good",
		} {
			onPath(u)
		}
		return nil
	}

	var (
		mu       sync.Mutex
		emitted  []string
	)
	collect := func(p string) {
		mu.Lock()
		defer mu.Unlock()
		emitted = append(emitted, p)
	}

	var src strings.Builder
	for i := 0; src.Len() < 16*1024; i++ {
		src.WriteString("fetch('/api/v1/item/")
		src.WriteString(strings.Repeat("z", 16))
		src.WriteString("');\n")
		src.WriteString(strings.Repeat(".", 200))
		src.WriteString("\n")
	}

	cfg := NewAIJSExtractConfig(
		WithAIJS_BaseRequest(false, []byte("GET / HTTP/1.1\r\nHost: x.test\r\n\r\n")),
		WithAIJS_ChunkBytes(8*1024),
		WithAIJS_OverlapBytes(0),
		WithAIJS_Concurrency(1),
	)
	err := RunAIJSExtract(context.Background(), src.String(), cfg, collect)
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	for _, e := range emitted {
		assert.False(t, looksLikeBoundaryLeak(e),
			"boundary leak escaped emit: %q", e)
		assert.NotContains(t, e, "javascript:")
		assert.NotContains(t, e, "#")
	}
	// The good URL must be present and deduped exactly once across all slices.
	good := "http://127.0.0.1:8787/api/good"
	count := 0
	for _, e := range emitted {
		if e == good {
			count++
		}
	}
	assert.Equal(t, 1, count, "good url should be emitted exactly once after dedup, got %d (all: %v)", count, emitted)
	// Fragment must be stripped from momentjs link.
	assert.Contains(t, emitted, "http://momentjs.com/guides/")
}

func TestBuildRequestContextBlock_EmptyReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", buildRequestContextBlock(nil))
	assert.Equal(t, "", buildRequestContextBlock(&AIJSExtractConfig{}))
}

func TestBuildRequestContextBlock_HTTPSFormats(t *testing.T) {
	raw := []byte("GET /app/index.html?x=1 HTTP/1.1\r\n" +
		"Host: www.example.com\r\n" +
		"User-Agent: UA-123\r\n" +
		"Cookie: sid=abc\r\n" +
		"\r\n" +
		"SHOULD_NOT_APPEAR_BODY")

	cfg := NewAIJSExtractConfig(WithAIJS_BaseRequest(true, raw))
	block := buildRequestContextBlock(cfg)

	assert.NotEmpty(t, block)
	assert.Contains(t, block, "=== REQUEST CONTEXT ===")
	assert.Contains(t, block, "=== END REQUEST CONTEXT ===")
	assert.Contains(t, block, "scheme: https")
	assert.Contains(t, block, "host: www.example.com")
	assert.Contains(t, block, "base_url: https://www.example.com/app/index.html?x=1")
	assert.Contains(t, block, "request_head:")
	assert.Contains(t, block, "GET /app/index.html?x=1 HTTP/1.1")
	assert.Contains(t, block, "User-Agent: UA-123")
	assert.NotContains(t, block, "SHOULD_NOT_APPEAR_BODY",
		"body must be stripped from request_head block")
}

func TestBuildRequestContextBlock_HTTPAndTruncation(t *testing.T) {
	big := strings.Repeat("A", 10*1024)
	raw := []byte("GET / HTTP/1.1\r\n" +
		"Host: x.test\r\n" +
		"X-Huge: " + big + "\r\n" +
		"\r\n")

	cfg := NewAIJSExtractConfig(
		WithAIJS_BaseRequest(false, raw),
		WithAIJS_RequestHeadMaxBytes(1024),
	)
	block := buildRequestContextBlock(cfg)
	assert.Contains(t, block, "scheme: http")
	assert.Contains(t, block, "host: x.test")
	assert.Contains(t, block, "... (truncated)",
		"request head longer than cap must be marked as truncated")
	assert.Less(t, len(block), 2*1024,
		"final block should respect RequestHeadMaxBytes (got %d bytes)", len(block))
}

// TestRunAIJSExtract_PayloadPrefixedWithRequestContext verifies that when the
// crawler passes WithAIJS_BaseRequest, every AI slice payload starts with the
// REQUEST CONTEXT block so the model can resolve relative paths to absolute
// URLs. Uses the mock invokeLiteForgeForPathsFunc to capture what actually
// gets shipped to the AI.
func TestRunAIJSExtract_PayloadPrefixedWithRequestContext(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu       sync.Mutex
		payloads []string
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		return nil
	}

	// enough to defeat SkipBelowBytes fast path and trigger the reducer
	var b strings.Builder
	for i := 0; b.Len() < 64*1024; i++ {
		b.WriteString("fetch('/api/v1/item/")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString("');\n")
		b.WriteString(strings.Repeat(".", 300))
		b.WriteString("\n")
	}
	src := b.String()

	raw := []byte("GET /boot.html HTTP/1.1\r\n" +
		"Host: unit.example.com\r\n" +
		"\r\n")

	cfg := NewAIJSExtractConfig(
		WithAIJS_BaseRequest(true, raw),
		WithAIJS_ChunkBytes(32*1024),
		WithAIJS_OverlapBytes(0),
		// This test exercises the reducer path; disable direct-feed so the
		// 64KB fixture is sliced rather than shipped in one call.
		WithAIJS_SmallInputBytes(0),
		WithAIJS_SmallInputTokens(0),
		WithAIJS_Concurrency(1),
	)

	err := RunAIJSExtract(context.Background(), src, cfg, func(p string) {})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, payloads, "expected at least one AI slice")
	for i, p := range payloads {
		head := p
		if len(head) > 200 {
			head = head[:200]
		}
		if !strings.HasPrefix(p, "=== REQUEST CONTEXT ===") {
			t.Fatalf("payload %d must start with REQUEST CONTEXT block, got: %q", i, head)
		}
		assert.Contains(t, p, "scheme: https", "payload %d missing scheme", i)
		assert.Contains(t, p, "host: unit.example.com", "payload %d missing host", i)
		assert.Contains(t, p, "=== END REQUEST CONTEXT ===")
	}
}

// TestRunAIJSExtract_NoContextWhenRequestRawEmpty keeps the legacy behavior:
// callers who do not set WithAIJS_BaseRequest still get the raw candidate
// payload (no REQUEST CONTEXT prefix), so existing integrations do not see a
// behavior change until they opt in.
func TestRunAIJSExtract_NoContextWhenRequestRawEmpty(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu       sync.Mutex
		payloads []string
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		return nil
	}

	var b strings.Builder
	for i := 0; b.Len() < 8*1024; i++ {
		b.WriteString("fetch('/legacy/")
		b.WriteString(strings.Repeat("y", 10))
		b.WriteString("');\n")
		b.WriteString(strings.Repeat(".", 300))
		b.WriteString("\n")
	}

	cfg := NewAIJSExtractConfig(
		WithAIJS_ChunkBytes(4*1024),
		WithAIJS_OverlapBytes(0),
		WithAIJS_SkipBelowBytes(512),
		// Force reducer path so this test still exercises the legacy
		// payload format (raw candidate stream, no REQUEST CONTEXT) even
		// after the small-input direct-feed default kicked in.
		WithAIJS_SmallInputBytes(0),
		WithAIJS_SmallInputTokens(0),
		WithAIJS_Concurrency(1),
	)

	err := RunAIJSExtract(context.Background(), b.String(), cfg, func(p string) {})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, payloads)
	for i, p := range payloads {
		assert.False(t, strings.HasPrefix(p, "=== REQUEST CONTEXT ==="),
			"payload %d must NOT have REQUEST CONTEXT prefix without WithAIJS_BaseRequest", i)
	}
}

// TestLooksLikePathCandidate_AcceptsAndRejects verifies the heuristic that
// guards emit() against AI hallucinations of bare identifiers as URL
// candidates. Only multi-segment paths and references with a known file
// extension pass; single-segment HTML tag names / HTTP methods / header
// names are rejected.
func TestLooksLikePathCandidate_AcceptsAndRejects(t *testing.T) {
	for _, s := range []string{
		"/api/v1/login",
		"api/v1/users",
		"/misc/response/fetch/basic.action",
		"deep.js",
		"/deep.js",
		"sub/foo.json",
		"list.do",
		"a/b",
		"static/img/logo.png",
	} {
		assert.True(t, looksLikePathCandidate(s), "expected %q to be a valid path candidate", s)
	}
	for _, s := range []string{
		"",
		"   ",
		"POST",
		"GET",
		"HackedJS",
		"AAA",
		"div",
		"body",
		"script",
		"/script",
		"/div",
		"/body",
		"/app",
		"?q=1",
		"foo-bar-baz",
	} {
		assert.False(t, looksLikePathCandidate(s), "expected %q to be rejected as a path candidate", s)
	}
}

// TestRunAIJSExtract_DropsAIHallucinatedBareIdentifiers verifies that when
// the AI (mock) returns noisy bare identifiers - HTTP methods, header
// names, HTML tag names, single-segment "/tag" paths - the emit filter
// drops them before they reach onPath. The legitimate multi-segment URL
// must still come through.
func TestRunAIJSExtract_DropsAIHallucinatedBareIdentifiers(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		for _, u := range []string{
			"POST", "HackedJS", "AAA",
			"script", "div", "body",
			"/script", "/div", "/body", "/app",
			"/misc/response/fetch/basic.action",
			"deep.js",
			"http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/deep.js",
		} {
			onPath(u)
		}
		return nil
	}

	var (
		mu      sync.Mutex
		emitted []string
	)
	err := RunAIJSExtract(context.Background(), "var deepUrl = 'deep.js';", nil, func(p string) {
		mu.Lock()
		defer mu.Unlock()
		emitted = append(emitted, p)
	})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(emitted, "|")
	for _, bad := range []string{
		"|POST|", "|HackedJS|", "|AAA|",
		"|script|", "|div|", "|body|",
		"|/script|", "|/div|", "|/body|", "|/app|",
	} {
		assert.NotContains(t, "|"+joined+"|", bad,
			"bare-identifier hallucination must be filtered out")
	}
	assert.Contains(t, emitted, "/misc/response/fetch/basic.action",
		"multi-segment path candidate must pass")
	assert.Contains(t, emitted, "deep.js",
		"file-extension bare candidate must pass")
	assert.Contains(t, emitted, "http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/deep.js",
		"absolute URL candidate must pass")
}

// TestExtractURLLikeCandidates_FunctionAndAssignmentPatterns verifies the
// expanded regex set added for the AI JS extractor: function-call style
// triggers (fetch / xhr.open / axios / new XHR / new URL / new WebSocket /
// import / require / $.ajax / ky / got / superagent / request), assignment
// fields (url= / baseURL: / endpoint: / action: / src=), and the
// quote-wrapped file-name fallback (`'deep.js'` with no leading slash).
func TestExtractURLLikeCandidates_FunctionAndAssignmentPatterns(t *testing.T) {
	src := `
		// function-call style
		fetch('/api/v1/login');
		fetch("/api/v1/users?id=1", {method: 'GET'});
		const xhr = new XMLHttpRequest();
		xhr.open('POST', 'deep.js', true);
		xhr.send('aaa=1');
		axios.get('/api/users');
		axios('/api/posts');
		ky.post('/api/orders');
		got('/api/items');
		request.get('/api/search');
		$.ajax({url: '/api/list'});
		$.getJSON("/api/json/feed");
		const ws = new WebSocket('wss://x.test/sock');
		const u = new URL('/abs/path', base);
		const es = new EventSource('/stream/events');
		import('./async-chunk.js').then(_ => {});
		const m = require('./helper.js');

		// assignment-style
		var apiUrl = '/auth/check.action';
		const config = { baseURL: '/v2/api', endpoint: 'list.do', action: 'submit.do' };
		element.src = '/static/img/logo.png';

		// quoted file-name literal (no leading slash)
		var deepUrl = 'deep.js';
		var modPath = 'sub/foo.json';
	`
	blocks := extractURLLikeCandidates(src, 80)
	assert.NotEmpty(t, blocks)

	needles := []string{
		"/api/v1/login",
		"/api/v1/users",
		"deep.js",
		"/api/users",
		"/api/posts",
		"/api/orders",
		"/api/items",
		"/api/search",
		"/api/list",
		"/api/json/feed",
		"wss://x.test/sock",
		"/abs/path",
		"/stream/events",
		"./async-chunk.js",
		"./helper.js",
		"/auth/check.action",
		"/v2/api",
		"list.do",
		"submit.do",
		"foo.json",
	}
	for _, n := range needles {
		assert.True(t, containsHit(blocks, n),
			"expected expanded regex set to expose %q in candidate windows", n)
	}
}

// TestRunAIJSExtract_SmallInputDirectFeed verifies the new direct-feed fast
// path: when the raw input is small enough (under SmallInputBytes AND under
// SmallInputTokens), RunAIJSExtract must skip the regex pre-filter and ship
// the entire source code in a single AI call - this preserves the
// cross-statement context (variable assignment + later use) that gets lost
// after windowed slicing.
func TestRunAIJSExtract_SmallInputDirectFeed(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu        sync.Mutex
		payloads  []string
		callCount int32
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&callCount, 1)
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		// emit a deterministic "good" URL the test can look for
		onPath("http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/deep.js")
		return nil
	}

	// Page-A style code: a variable assignment whose value is later used by
	// fetch and xhr.open. The whole body is well under 200KB / 50K tokens.
	src := `var deepUrl = 'deep.js';
fetch(deepUrl, {method: 'POST', headers: {'HackedJS': 'AAA'}});

var xhr = new XMLHttpRequest();
xhr.open('POST', deepUrl, true);
xhr.setRequestHeader('HackedJS', 'AAA');
xhr.send('aaa=1');
console.log('done');`
	raw := []byte("GET /misc/response/javascript-ssa-ir-basic/basic-fetch.html HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8787\r\n\r\n")

	cfg := NewAIJSExtractConfig(WithAIJS_BaseRequest(false, raw))

	var got []string
	var gotMu sync.Mutex
	err := RunAIJSExtract(context.Background(), src, cfg, func(p string) {
		gotMu.Lock()
		defer gotMu.Unlock()
		got = append(got, p)
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount),
		"direct-feed must call AI exactly once for a small input")

	mu.Lock()
	assert.Len(t, payloads, 1)
	p := payloads[0]
	mu.Unlock()

	assert.True(t, strings.HasPrefix(p, "=== REQUEST CONTEXT ==="),
		"direct-feed payload must still carry the REQUEST CONTEXT block")
	assert.Contains(t, p, "host: 127.0.0.1:8787")
	assert.Contains(t, p, "var deepUrl = 'deep.js';",
		"direct-feed must include the original code verbatim")
	assert.Contains(t, p, "xhr.open('POST', deepUrl, true);",
		"direct-feed must preserve cross-statement context")
	assert.NotContains(t, p, "--- candidate ---",
		"direct-feed must NOT wrap the source into candidate windows")

	gotMu.Lock()
	defer gotMu.Unlock()
	assert.Contains(t, got, "http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/deep.js")
}

// TestRunAIJSExtract_DirectFeedDisabledWhenAboveThreshold confirms that an
// input larger than SmallInputBytes falls back to the regex + reducer
// pipeline instead of taking the direct-feed fast path.
func TestRunAIJSExtract_DirectFeedDisabledWhenAboveThreshold(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu       sync.Mutex
		payloads []string
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
		return nil
	}

	// Build ~120KB so it sits above the small SmallInputBytes we'll set.
	var b strings.Builder
	for i := 0; b.Len() < 120*1024; i++ {
		b.WriteString("fetch('/api/v1/route/")
		b.WriteString(strings.Repeat("a", 32))
		b.WriteString("');\n")
		b.WriteString(strings.Repeat(".", 256))
		b.WriteString("\n")
	}

	cfg := NewAIJSExtractConfig(
		WithAIJS_SmallInputBytes(64*1024),  // 64KB direct-feed cap
		WithAIJS_SmallInputTokens(20*1024),
		WithAIJS_ChunkBytes(32*1024),
		WithAIJS_OverlapBytes(0),
		WithAIJS_SkipBelowBytes(1024),
		WithAIJS_Concurrency(1),
	)

	err := RunAIJSExtract(context.Background(), b.String(), cfg, func(p string) {})
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(payloads), 2,
		"input above SmallInputBytes must take the reducer path and emit multiple slices, got %d", len(payloads))
	for i, p := range payloads {
		assert.Contains(t, p, "--- candidate ---",
			"reducer-path payload %d must wrap matches in candidate windows", i)
	}
}

