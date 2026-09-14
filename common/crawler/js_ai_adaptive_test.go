package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMUSTPASS_AIJSExternalAssetBudget(t *testing.T) {
	var (
		mu     sync.Mutex
		counts = make(map[string]int)
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			var page strings.Builder
			page.WriteString("<!doctype html><html><body>")
			for index := 0; index < 3; index++ {
				page.WriteString(`<script src="/assets/duplicate.js"></script>`)
			}
			for index := 0; index < 16; index++ {
				page.WriteString(`<script src="/assets/distinct-`)
				page.WriteRune(rune('a' + index))
				page.WriteString(`.js"></script>`)
			}
			page.WriteString("</body></html>")
			_, _ = w.Write([]byte(page.String()))
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(`window.__assetLoaded=true;`))
	}))
	defer server.Close()

	crawler, err := NewCrawler(
		server.URL,
		WithForbiddenFromParent(true),
		WithMaxDepth(2),
		WithMaxRequestCount(3),
		WithConcurrent(4),
		WithAIJSExtract(
			WithAIJS_AdaptiveTrigger(),
			withAIJSInvoker(func(_ context.Context, _ *AIJSExtractConfig, _ string, _ func(string)) error {
				return nil
			}),
		),
	)
	require.NoError(t, err)
	require.NoError(t, crawler.Run())

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for _, count := range counts {
		total += count
	}
	require.LessOrEqual(t, total, 3, "direct script downloads bypassed maxRequestCount: %#v", counts)
	require.Equal(t, 1, counts["/assets/duplicate.js"], "duplicate script tags must share one in-flight request")
}

func TestNewAIJSExtractConfig_AdaptiveDefaultsDoNotChangeLegacy(t *testing.T) {
	legacy := NewAIJSExtractConfig()
	require.False(t, legacy.AdaptiveTrigger)
	require.Zero(t, legacy.MaxAIRequests)
	require.Zero(t, legacy.MaxCandidateWindows)
	require.Zero(t, legacy.MaxCandidateBytes)
	require.Zero(t, legacy.CallTimeout)

	adaptive := NewAIJSExtractConfig(WithAIJS_AdaptiveTrigger())
	require.True(t, adaptive.AdaptiveTrigger)
	require.Equal(t, 8, adaptive.MaxAIRequests)
	require.Equal(t, 256, adaptive.MaxCandidateWindows)
	require.Equal(t, 512*1024, adaptive.MaxCandidateBytes)
	require.Equal(t, 4*time.Second, adaptive.CallTimeout)

	restored := NewAIJSExtractConfig(WithAIJS_AdaptiveTrigger(), WithAIJS_AdaptiveTrigger(false))
	require.False(t, restored.AdaptiveTrigger)
	require.Zero(t, restored.MaxAIRequests)
	require.Zero(t, restored.MaxCandidateWindows)
	require.Zero(t, restored.MaxCandidateBytes)
	require.Zero(t, restored.CallTimeout)

	overridden := NewAIJSExtractConfig(
		WithAIJS_MaxRequests(2),
		WithAIJS_MaxCandidateWindows(12),
		WithAIJS_MaxCandidateBytes(32*1024),
		WithAIJS_CallTimeoutSeconds(1),
		WithAIJS_AdaptiveTrigger(),
	)
	require.Equal(t, 2, overridden.MaxAIRequests)
	require.Equal(t, 12, overridden.MaxCandidateWindows)
	require.Equal(t, 32*1024, overridden.MaxCandidateBytes)
	require.Equal(t, time.Second, overridden.CallTimeout)
}

func TestExtractURLLikeCandidatesBounded_PreservesTailEvidence(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 2000; i++ {
		source.WriteString(`fetch("/noise/front/path.json");`)
	}
	source.WriteString(strings.Repeat("x", 1024*1024))
	source.WriteString(`;const HIDDEN_TAIL_ROUTE=String.fromCharCode(47,97,112,105,47,116,97,105,108);fetch(HIDDEN_TAIL_ROUTE);`)

	blocks := extractAdaptiveURLLikeCandidatesBounded(source.String(), 160, 8, 16*1024)
	joined := strings.Join(blocks, "")
	require.NotEmpty(t, blocks)
	require.LessOrEqual(t, len(joined), 16*1024)
	require.Contains(t, joined, "HIDDEN_TAIL_ROUTE",
		"front-loaded minified noise must not starve evidence at the bundle tail")
}

func TestRawCandidateHits_MixedCaseResourceExtensions(t *testing.T) {
	hits := rawCandidateHits(`const a="/assets/ROUTES.JSON";const b="chunks/Runtime.MIN.JS";`)
	require.Contains(t, hits, "/assets/ROUTES.JSON")
	require.Contains(t, hits, "chunks/Runtime.MIN.JS")
}

func TestRawCandidateHits_CommentBoundaries(t *testing.T) {
	hits := rawCandidateHits("// fetch('/fake/line.json')\n" +
		"/* const x='/fake/block.json'; */\n" +
		`const live="/api/live.json";const absolute="https://example.test/live";`)
	require.NotContains(t, hits, "/fake/line.json")
	require.NotContains(t, hits, "/fake/block.json")
	require.Contains(t, hits, "/api/live.json")
	require.Contains(t, hits, "https://example.test/live")
}

func TestRawCandidateHitsBounded_CommentNoiseCannotStarveLiveTail(t *testing.T) {
	source := "/*" + strings.Repeat(` fetch('/noise/front.json');`, 512) +
		` */const live="/api/live/tail.json";`
	hits := rawCandidateHitsBounded(source, 1)
	require.Equal(t, []string{"/api/live/tail.json"}, hits)
}

func TestBuildRequestContextBlock_RedactsEveryCredentialSurface(t *testing.T) {
	raw := []byte("GET /app?token=request-secret&session_id=request-session-secret&signature=request-signature-secret&credential=request-credential-secret&access_key=request-access-secret&auth=request-auth-secret&x=visible HTTP/1.1\r\n" +
		"Host: example.test\r\n" +
		"Authorization: Bearer auth-secret\r\n" +
		"Cookie: sid=cookie-secret\r\n" +
		"X-Custom-Secret: custom-secret\r\n" +
		"Referer: https://example.test/from?api_key=referer-secret&oauth_code=oauth-secret&keep=yes\r\n" +
		"User-Agent: crawler-unit\r\n\r\n")
	cfg := NewAIJSExtractConfig(WithAIJS_BaseRequest(true, raw))
	cfg.assetSourceURL = "https://example.test/app.js?password=asset-secret&jwt=asset-jwt-secret&build=42"

	block := buildRequestContextBlock(cfg)
	for _, secret := range []string{
		"request-secret", "auth-secret", "cookie-secret", "custom-secret",
		"referer-secret", "oauth-secret", "asset-secret", "asset-jwt-secret",
		"request-session-secret", "request-signature-secret", "request-credential-secret",
		"request-access-secret", "request-auth-secret",
	} {
		require.NotContains(t, block, secret)
	}
	require.Contains(t, block, "x=visible")
	require.Contains(t, block, "keep=yes")
	require.Contains(t, block, "build=42")
	require.Contains(t, block, "User-Agent: crawler-unit")
}
