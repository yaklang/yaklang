package crawler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestCoreAssetHTTPSSeedKeepsSchemeForChildren(t *testing.T) {
	root, err := createReqFromUrlEx(nil, http.MethodGet, "https://example.test/app", http.NoBody, nil)
	require.NoError(t, err)
	require.True(t, root.IsHttps())
	require.Equal(t, "https://example.test/app", root.Url())
	require.NotNil(t, root.baseURL)
	require.Equal(t, "https://example.test/child", root.AbsoluteURL("/child"))

	childHTTPS, childRaw, err := NewHTTPRequest(root.IsHttps(), root.requestRaw, nil, "/child")
	require.NoError(t, err)
	require.True(t, childHTTPS)
	c := &Crawler{}
	child, err := c.createReqFromBytes(root, childHTTPS, childRaw)
	require.NoError(t, err)
	require.True(t, child.IsHttps())
	require.Equal(t, "https", child.baseURL.Scheme)
	require.Equal(t, "https://example.test/grandchild", child.AbsoluteURL("/grandchild"))
}

func TestCoreAssetHTTPSFallbackAdoptsHTTPProvenanceForChildren(t *testing.T) {
	var (
		tlsAttempts     atomic.Int64
		tlsAtRoot       atomic.Int64
		rootRequests    atomic.Int64
		childRequests   atomic.Int64
		observedURLsMu  sync.Mutex
		observedURLs    []string
		observedSchemes []bool
	)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			rootRequests.Add(1)
			tlsAtRoot.Store(tlsAttempts.Load())
			_, _ = w.Write([]byte(`<a href="/child">child</a>`))
		case "/child":
			childRequests.Add(1)
			_, _ = w.Write([]byte("done"))
		default:
			http.NotFound(w, r)
		}
	}))
	server.Listener = &crawlerProtocolDetectListener{
		Listener:    server.Listener,
		tlsAttempts: &tlsAttempts,
	}
	server.Start()
	defer server.Close()

	httpsSeed := "https://" + strings.TrimPrefix(server.URL, "http://") + "/"
	c, err := NewCrawler(
		httpsSeed,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithMaxRequestCount(4),
		WithMaxRetry(0),
		WithOnRequest(func(req *Req) {
			observedURLsMu.Lock()
			observedURLs = append(observedURLs, req.Url())
			observedSchemes = append(observedSchemes, req.IsHttps())
			observedURLsMu.Unlock()
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	require.Positive(t, tlsAttempts.Load(), "the HTTPS seed must exercise transport fallback")
	require.Equal(t, tlsAtRoot.Load(), tlsAttempts.Load(), "the child must inherit HTTP instead of attempting HTTPS again")
	require.EqualValues(t, 1, rootRequests.Load())
	require.EqualValues(t, 1, childRequests.Load())
	observedURLsMu.Lock()
	require.ElementsMatch(t, []string{server.URL + "/", server.URL + "/child"}, observedURLs)
	require.Equal(t, []bool{false, false}, observedSchemes)
	observedURLsMu.Unlock()

	rootHTTPHash := utils.CalcSha1(server.URL+"/", http.MethodGet)
	_, requested := c.requestedHash.Load(rootHTTPHash)
	require.True(t, requested, "the final HTTP identity must be present in requested deduplication")
	_, found := c.foundUrls.Load(rootHTTPHash)
	require.True(t, found, "the final HTTP identity must be present in found deduplication")
}

func TestCoreAssetRedirectAdoptsFinalBaseForRelativeLinkAndJavaScript(t *testing.T) {
	var requestMu sync.Mutex
	requestCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCounts[r.URL.Path]++
		requestMu.Unlock()

		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/deep/page", http.StatusFound)
		case "/deep/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Link", `<runtime.js>; rel="preload"; as="script"`)
			_, _ = w.Write([]byte(`<script src="chunk.js"></script>`))
		case "/deep/runtime.js", "/deep/chunk.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`window.__loaded__=true;`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var observedRootURL string
	c, err := NewCrawler(
		server.URL+"/start",
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithMaxRequestCount(8),
		WithJSParser(true),
		WithOnRequest(func(req *Req) {
			if strings.HasSuffix(req.Url(), "/deep/page") {
				observedRootURL = req.Url()
			}
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())
	require.Equal(t, server.URL+"/deep/page", observedRootURL)

	requestMu.Lock()
	require.Equal(t, 1, requestCounts["/deep/runtime.js"])
	require.Equal(t, 1, requestCounts["/deep/chunk.js"])
	require.Zero(t, requestCounts["/start/runtime.js"])
	require.Zero(t, requestCounts["/start/chunk.js"])
	requestMu.Unlock()

	finalHash := utils.CalcSha1(server.URL+"/deep/page", http.MethodGet)
	_, requested := c.requestedHash.Load(finalHash)
	require.True(t, requested, "the redirect target must be present in requested deduplication")
	_, found := c.foundUrls.Load(finalHash)
	require.True(t, found, "the redirect target must be present in found deduplication")
}

type crawlerProtocolDetectListener struct {
	net.Listener
	tlsAttempts *atomic.Int64
}

func (l *crawlerProtocolDetectListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &crawlerProtocolDetectConn{Conn: conn, tlsAttempts: l.tlsAttempts}, nil
}

type crawlerProtocolDetectConn struct {
	net.Conn
	checked     bool
	tlsAttempts *atomic.Int64
}

func (c *crawlerProtocolDetectConn) Read(buffer []byte) (int, error) {
	read, err := c.Conn.Read(buffer)
	if !c.checked && read > 0 {
		c.checked = true
		if buffer[0] == 0x16 {
			c.tlsAttempts.Add(1)
			_ = c.Conn.Close()
			return 0, io.EOF
		}
	}
	return read, err
}

func TestCoreAssetResponseBodyLimitIsEnforcedByTransport(t *testing.T) {
	const limit = 1024
	body := bytes.Repeat([]byte("oversized-crawler-asset"), 1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	c, err := NewCrawler(server.URL, WithExactOrigins(), WithBodySize(limit))
	require.NoError(t, err)
	requestRaw := []byte("GET / HTTP/1.1\r\nHost: " + strings.TrimPrefix(server.URL, "http://") + "\r\n\r\n")
	response, _, err := c.config.DoHTTPRequest(false, "", lowhttp.WithPacketBytes(requestRaw))
	require.NoError(t, err)
	require.NotNil(t, response)
	_, responseBody := lowhttp.SplitHTTPPacketFast(response.RawPacket)
	require.Len(t, responseBody, limit)
	require.True(t, response.TooLarge, "transport should retain explicit truncation metadata")
}

func TestCoreAssetExactOriginsIsOptIn(t *testing.T) {
	legacy, err := NewCrawler("https://example.test/app")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"https://example.test/app",
		"https://www.example.test/app",
	}, legacy.originUrls)

	exact, err := NewCrawler("https://example.test/app", WithExactOrigins())
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.test/app"}, exact.originUrls)
	require.True(t, exact.config.CheckShouldBeHandledURL(mustCoreAssetURL(t, "https://example.test/inside")))
	require.False(t, exact.config.CheckShouldBeHandledURL(mustCoreAssetURL(t, "https://foo.example.test/inside")))
	require.False(t, exact.config.CheckShouldBeHandledURL(mustCoreAssetURL(t, "https://example.test.evil/inside")))

	disabled, err := NewCrawler("https://example.test/app", WithExactOrigins(false))
	require.NoError(t, err)
	require.ElementsMatch(t, legacy.originUrls, disabled.originUrls)

	expanded, err := NewCrawler(
		"https://example.test/app",
		WithExactOrigins(),
		WithDomainWhiteList("foo.example.test"),
	)
	require.NoError(t, err)
	require.True(t, expanded.config.CheckShouldBeHandledURL(mustCoreAssetURL(t, "https://foo.example.test/inside")))
}

func TestCoreAssetSchedulerPrioritizesAssetsAndBoundsBothQueues(t *testing.T) {
	scheduler := newRequestScheduler(context.Background(), 1)
	normal := &Req{}
	high := &Req{priority: true}

	require.True(t, scheduler.Submit(normal))
	require.False(t, scheduler.Submit(&Req{}), "normal queue exceeded its bound")
	require.True(t, scheduler.Submit(high), "high queue has an independent bounded slot")
	require.False(t, scheduler.Submit(&Req{priority: true}), "high queue exceeded its bound")
	scheduler.StartupDone()

	first, ok := scheduler.Next()
	require.True(t, ok)
	require.Same(t, high, first)
	scheduler.Done()
	second, ok := scheduler.Next()
	require.True(t, ok)
	require.Same(t, normal, second)
	scheduler.Done()
	_, ok = scheduler.Next()
	require.False(t, ok)
}

func TestCoreAssetSchedulerConcurrentCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	scheduler := newRequestScheduler(ctx, 32)
	var accepted atomic.Int64
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for {
			_, ok := scheduler.Next()
			if !ok {
				return
			}
			scheduler.Done()
		}
	}()

	var producers sync.WaitGroup
	for producer := 0; producer < 8; producer++ {
		producers.Add(1)
		go func(producer int) {
			defer producers.Done()
			for index := 0; index < 100; index++ {
				if scheduler.Submit(&Req{priority: (producer+index)%3 == 0}) {
					accepted.Add(1)
				}
			}
		}(producer)
	}
	producers.Wait()
	scheduler.StartupDone()

	select {
	case <-consumerDone:
	case <-ctx.Done():
		t.Fatal("scheduler did not close after concurrent producers completed")
	}
	require.Positive(t, accepted.Load())
	require.Zero(t, scheduler.pending.Load())
}

func TestCoreAssetSchedulerHighBurstStillAdvancesNormalQueue(t *testing.T) {
	scheduler := newRequestScheduler(context.Background(), 16)
	normal := &Req{}
	require.True(t, scheduler.Submit(normal))
	for index := 0; index < requestSchedulerHighBurst+1; index++ {
		require.True(t, scheduler.Submit(&Req{priority: true}))
	}
	scheduler.StartupDone()

	for index := 0; index < requestSchedulerHighBurst; index++ {
		req, ok := scheduler.Next()
		require.True(t, ok)
		require.True(t, req.priority)
		scheduler.Done()
	}
	req, ok := scheduler.Next()
	require.True(t, ok)
	require.Same(t, normal, req)
	scheduler.Done()
	req, ok = scheduler.Next()
	require.True(t, ok)
	require.True(t, req.priority)
	scheduler.Done()
	_, ok = scheduler.Next()
	require.False(t, ok)
}

func TestCoreAssetSchedulerCloseRacesWithSubmitAndDone(t *testing.T) {
	scheduler := newRequestScheduler(context.Background(), 64)
	started := make(chan struct{})
	var startOnce sync.Once
	var workers sync.WaitGroup

	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			_, ok := scheduler.Next()
			if !ok {
				return
			}
			scheduler.Done()
		}
	}()
	for producer := 0; producer < 8; producer++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := 0; index < 200; index++ {
				if scheduler.Submit(&Req{priority: index%2 == 0}) {
					startOnce.Do(func() { close(started) })
				}
			}
		}()
	}
	select {
	case <-started:
		scheduler.Close()
	case <-time.After(time.Second):
		t.Fatal("scheduler accepted no request")
	}

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close raced with Submit/Done and did not terminate")
	}
	require.False(t, scheduler.Submit(&Req{}))
}

func TestCoreAssetHeadersAndOutOfScopeDiscovery(t *testing.T) {
	var mu sync.Mutex
	requestCounts := make(map[string]int)
	var externalURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCounts[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path != "/" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.Header().Add("Link", `</assets/runtime-config.json?items=a,b>; rel="preload", <`+externalURL+`>; rel="alternate"`)
		w.Header().Add("Link", `<JaVaScRiPt:alert(1)>; rel="alternate", <data:text/plain,ignored>; rel="preload"`)
		w.Header().Set("SourceMap", "/assets/app.js.map")
		w.Header().Set("X-SourceMap", "/assets/legacy.js.map")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<a href="%s">external duplicate</a><a href="/inside">inside</a>`, externalURL)
	}))
	defer server.Close()
	externalURL = strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/out"
	seedURL := server.URL

	var foundMu sync.Mutex
	var found []string
	c, err := NewCrawler(
		seedURL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithHTTPSFallback(false),
		WithOnUrlFound(func(rawURL string) {
			foundMu.Lock()
			found = append(found, rawURL)
			foundMu.Unlock()
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	mu.Lock()
	gotCounts := make(map[string]int, len(requestCounts))
	for key, value := range requestCounts {
		gotCounts[key] = value
	}
	mu.Unlock()
	require.Equal(t, 1, gotCounts["/"])
	require.Equal(t, 1, gotCounts["/assets/runtime-config.json"])
	require.Equal(t, 1, gotCounts["/assets/app.js.map"])
	require.Equal(t, 1, gotCounts["/assets/legacy.js.map"])
	require.Equal(t, 1, gotCounts["/inside"])
	require.Zero(t, gotCounts["/out"], "out-of-scope candidate must never be requested")

	foundMu.Lock()
	gotFound := append([]string(nil), found...)
	foundMu.Unlock()
	require.Equal(t, 1, countStrings(gotFound, externalURL), "duplicate out-of-scope candidate should be reported once")
	require.Contains(t, gotFound, seedURL+"/assets/runtime-config.json?items=a,b")
	require.Contains(t, gotFound, seedURL+"/assets/app.js.map")
	require.Contains(t, gotFound, seedURL+"/assets/legacy.js.map")
	for _, rawURL := range gotFound {
		require.NotContains(t, strings.ToLower(rawURL), "javascript:")
		require.NotContains(t, strings.ToLower(rawURL), "data:")
	}
}

func TestCoreAssetRedirectScopeIsEnforcedBeforeNetworkIO(t *testing.T) {
	var externalRequests atomic.Int64
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		externalRequests.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("external"))
	}))
	defer external.Close()
	externalURL := strings.Replace(external.URL, "127.0.0.1", "localhost", 1) + "/redirected"

	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, externalURL, http.StatusFound)
	}))
	defer seed.Close()

	denied, err := NewCrawler(
		seed.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxRequestCount(2),
	)
	require.NoError(t, err)
	// A caller-supplied lowhttp redirect callback cannot override the crawler's
	// final scope gate.
	seedRequest := []byte("GET / HTTP/1.1\r\nHost: " + strings.TrimPrefix(seed.URL, "http://") + "\r\n\r\n")
	_, _, err = denied.config.DoHTTPRequest(false, "",
		lowhttp.WithPacketBytes(seedRequest),
		lowhttp.WithRedirectHandler(func(bool, []byte, []byte) bool { return true }),
	)
	require.NoError(t, err)
	require.Zero(t, externalRequests.Load(), "an extra redirect handler must not override scope")
	require.NoError(t, denied.Run())
	require.Zero(t, externalRequests.Load(), "an automatic redirect must not cross the exact-host scope")

	redirectsDisabled, err := NewCrawler(
		seed.URL,
		WithExactOrigins(),
		WithMaxRedirectTimes(0),
		WithForbiddenFromParent(true),
	)
	require.NoError(t, err)
	_, _, err = redirectsDisabled.config.DoHTTPRequest(false, "",
		lowhttp.WithPacketBytes(seedRequest),
		lowhttp.WithRedirectTimes(1),
		lowhttp.WithRedirectHandler(func(bool, []byte, []byte) bool { return true }),
	)
	require.NoError(t, err)
	require.Zero(t, externalRequests.Load(),
		"an extra redirect-count option must not bypass scope when config redirects are disabled")

	var sameOriginTargetRequests atomic.Int64
	sameOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/target", http.StatusFound)
		case "/target":
			sameOriginTargetRequests.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer sameOrigin.Close()
	zeroRedirects, err := NewCrawler(
		sameOrigin.URL+"/start",
		WithExactOrigins(),
		WithMaxRedirectTimes(0),
		WithForbiddenFromParent(true),
	)
	require.NoError(t, err)
	sameOriginRequest := []byte("GET /start HTTP/1.1\r\nHost: " + strings.TrimPrefix(sameOrigin.URL, "http://") + "\r\n\r\n")
	_, _, err = zeroRedirects.config.DoHTTPRequest(false, "", lowhttp.WithPacketBytes(sameOriginRequest))
	require.NoError(t, err)
	require.Zero(t, sameOriginTargetRequests.Load(),
		"WithMaxRedirectTimes(0) must disable even same-origin redirects")

	allowed, err := NewCrawler(
		seed.URL,
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("localhost"),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxRequestCount(2),
	)
	require.NoError(t, err)
	redirectTarget, err := url.Parse(externalURL)
	require.NoError(t, err)
	redirectRequest := []byte("GET " + redirectTarget.RequestURI() + " HTTP/1.1\r\nHost: " + redirectTarget.Host + "\r\n\r\n")
	require.False(t, denied.config.shouldFollowRedirect(false, redirectRequest),
		"the default exact-host scope must reject another hostname")
	require.True(t, allowed.config.shouldFollowRedirect(false, redirectRequest),
		"an explicitly authorized redirect hostname should pass the redirect policy")
	require.Zero(t, externalRequests.Load(), "policy validation must not need an out-of-scope network request")
}

func TestCoreAssetOutOfScopeAIFindingIsReportedButNotRequested(t *testing.T) {
	var externalRequests atomic.Int64
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		externalRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer external.Close()
	externalURL := strings.Replace(external.URL, "127.0.0.1", "localhost", 1) + "/ai-only"

	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`const endpoint=routeParts.join("/");fetch(endpoint)`))
	}))
	defer seed.Close()

	var foundMu sync.Mutex
	var found []string
	c, err := NewCrawler(
		seed.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxRequestCount(3),
		WithOnUrlFound(func(rawURL string) {
			foundMu.Lock()
			found = append(found, rawURL)
			foundMu.Unlock()
		}),
		WithAIJSExtract(
			WithAIJS_AdaptiveTrigger(),
			WithAIJS_MaxRequests(1),
			withAIJSInvoker(func(_ context.Context, _ *AIJSExtractConfig, _ string, onPath func(string)) error {
				onPath(externalURL)
				return nil
			}),
		),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())
	require.Zero(t, externalRequests.Load())
	foundMu.Lock()
	require.Equal(t, 1, countStrings(found, externalURL))
	foundMu.Unlock()
}

func TestCoreAssetAdaptiveExternalJavaScriptUsesCrawlerPipeline(t *testing.T) {
	var mu sync.Mutex
	requestCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCounts[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<script src="/assets/runtime.chunk.js"></script>`))
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`window.__runtime_loaded__=true;`))
	}))
	defer server.Close()

	var onRequestMu sync.Mutex
	var requested []string
	c, err := NewCrawler(
		server.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithMaxRequestCount(4),
		WithOnRequest(func(req *Req) {
			onRequestMu.Lock()
			requested = append(requested, req.Url())
			onRequestMu.Unlock()
		}),
		WithAIJSExtract(
			WithAIJS_AdaptiveTrigger(),
			withAIJSInvoker(func(_ context.Context, _ *AIJSExtractConfig, _ string, _ func(string)) error {
				return nil
			}),
		),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	mu.Lock()
	require.Equal(t, 1, requestCounts["/assets/runtime.chunk.js"])
	mu.Unlock()
	onRequestMu.Lock()
	require.Contains(t, requested, server.URL+"/assets/runtime.chunk.js")
	onRequestMu.Unlock()
}

func TestCoreAssetRequestFindingCallbackOptionOrder(t *testing.T) {
	for _, callbackFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("callback-first-%v", callbackFirst), func(t *testing.T) {
			var received atomic.Int64
			callbackOpt := WithOnAIJSRequestFound(func(AIJSRequestFinding) {
				received.Add(1)
			})
			aiOpt := WithAIJSExtract(WithAIJS_AdaptiveTrigger())
			opts := []ConfigOpt{WithExactOrigins(), aiOpt, callbackOpt}
			if callbackFirst {
				opts = []ConfigOpt{WithExactOrigins(), callbackOpt, aiOpt}
			}
			crawler, err := NewCrawler("https://example.test", opts...)
			require.NoError(t, err)
			require.NotNil(t, crawler.config.aiJSExtractConfig.findingSink)
			crawler.config.aiJSExtractConfig.findingSink(AIJSRequestFinding{URL: "/api/static"})
			require.EqualValues(t, 1, received.Load())
		})
	}
}

func countStrings(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func mustCoreAssetURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed
}
