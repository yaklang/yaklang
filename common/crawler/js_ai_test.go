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

func TestRunAIJSExtract_FastPathBelowSkipThreshold(t *testing.T) {
	// AI must NOT be called when the candidate stream is smaller than SkipBelowBytes.
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var aiCalls int32
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&aiCalls, 1)
		return nil
	}

	cfg := NewAIJSExtractConfig(WithAIJS_SkipBelowBytes(1 << 20)) // huge threshold => always fast path
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
	assert.Equal(t, int32(0), atomic.LoadInt32(&aiCalls), "AI should not be called on fast path")

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(got, "|")
	assert.Contains(t, joined, "/api/v1/login")
	assert.Contains(t, joined, "/api/v2/users")
}

func TestRunAIJSExtract_NoCandidatesNoCall(t *testing.T) {
	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var aiCalls int32
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		atomic.AddInt32(&aiCalls, 1)
		return nil
	}

	err := RunAIJSExtract(context.Background(), "no urls here at all just plain prose", nil, func(p string) {})
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
	assert.Equal(t, int64(320*1024), c.ChunkBytes)
	assert.Equal(t, 2048, c.OverlapBytes)
	assert.Equal(t, 120, c.ContextBytes)
	assert.Equal(t, 1024, c.SkipBelowBytes)
	assert.Equal(t, 2, c.Concurrency)
}

func TestNewAIJSExtractConfig_OptionsApplied(t *testing.T) {
	c := NewAIJSExtractConfig(
		WithAIJS_MaxTokens(40000),
		WithAIJS_ChunkBytes(128*1024),
		WithAIJS_OverlapBytes(0),
		WithAIJS_ContextBytes(60),
		WithAIJS_SkipBelowBytes(2048),
		WithAIJS_Concurrency(4),
	)
	assert.Equal(t, 40000, c.MaxTokens)
	assert.Equal(t, int64(128*1024), c.ChunkBytes)
	assert.Equal(t, 0, c.OverlapBytes)
	assert.Equal(t, 60, c.ContextBytes)
	assert.Equal(t, 2048, c.SkipBelowBytes)
	assert.Equal(t, 4, c.Concurrency)
}
