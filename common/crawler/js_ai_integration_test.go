package crawler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fetchLocalTarget tries to GET a helper URL from the developer's local
// fixture server. Returns (body, true) when reachable; ("", false) otherwise.
func fetchLocalTarget(url string) (string, bool) {
	cli := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", false
	}
	resp, err := cli.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// TestRunAIJSExtract_WebpackFixture_SliceSize is an integration test that
// drives RunAIJSExtract with a real webpack-style bundle served by the
// developer's local helper (http://127.0.0.1:8787). Because it depends on a
// service that is not always running, it SKIPS cleanly when unreachable.
//
// Purpose: protect the regression where AI request payloads were only ~6KB
// each because "--- end ---" was acting as an every-occurrence flush trigger.
// With the new "separator-as-boundary" mode plus ChunkBytes=250KB, slice
// payloads must comfortably fill the chunk budget.
func TestRunAIJSExtract_WebpackFixture_SliceSize(t *testing.T) {
	const base = "http://127.0.0.1:8787"
	urls := []string{
		base + "/misc/response/webpack-ssa-ir-test.html",
		base + "/static/js/spa/pre-main.js",
		base + "/static/js/spa/main.js",
	}

	var combined strings.Builder
	anyOk := false
	for _, u := range urls {
		body, ok := fetchLocalTarget(u)
		if !ok {
			continue
		}
		anyOk = true
		combined.WriteString(body)
		combined.WriteString("\n//---fixture-end---\n")
	}
	if !anyOk {
		t.Skip("local fixture at 127.0.0.1:8787 unreachable; skipping")
		return
	}
	if combined.Len() < 100*1024 {
		t.Skipf("local fixture total bytes=%d < 100KB, not enough to assert slice size", combined.Len())
		return
	}
	t.Logf("fetched total bytes=%d from %d urls", combined.Len(), len(urls))

	originalFn := invokeLiteForgeForPathsFunc
	defer func() { invokeLiteForgeForPathsFunc = originalFn }()

	var (
		mu       sync.Mutex
		payloads []int
	)
	invokeLiteForgeForPathsFunc = func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, len(payload))
		return nil
	}

	chunkBytes := int64(250 * 1024)
	cfg := NewAIJSExtractConfig(
		WithAIJS_ChunkBytes(chunkBytes),
		WithAIJS_MaxTokens(80*1024),
		WithAIJS_OverlapBytes(2048),
		WithAIJS_SkipBelowBytes(1024),
		// This test exercises the reducer slicing path; explicitly disable
		// the direct-feed fast path so a sub-200KB fixture still goes
		// through extractURLLikeCandidates + aireducer rather than being
		// shipped in a single AI call.
		WithAIJS_SmallInputBytes(0),
		WithAIJS_SmallInputTokens(0),
		WithAIJS_Concurrency(1),
	)

	err := RunAIJSExtract(context.Background(), combined.String(), cfg, func(p string) {})
	assert.NoError(t, err)

	mu.Lock()
	sizes := append([]int(nil), payloads...)
	mu.Unlock()

	assert.NotEmpty(t, sizes)
	t.Logf("slices=%d sizes_bytes=%v", len(sizes), sizes)

	var maxSize, sum int
	for _, s := range sizes {
		if s > maxSize {
			maxSize = s
		}
		sum += s
	}
	avg := 0
	if len(sizes) > 0 {
		avg = sum / len(sizes)
	}
	t.Logf("max_slice=%d avg_slice=%d", maxSize, avg)

	// The regression produced ~6KB per slice. Even allowing for the final
	// tail chunk being small, the LARGEST slice must be far above that.
	assert.Greater(t, maxSize, 60*1024, "max slice must exceed 60KB - regression guard against per-separator flushing")

	// Sanity upper bound: no slice should blow past chunkBytes + generous headroom.
	assert.LessOrEqual(t, maxSize, int(chunkBytes)+16*1024)
}
