package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoreURLDiscoveryBudgetLimitsHTMLBeforeCallbackAndScheduling(t *testing.T) {
	var requestMu sync.Mutex
	requestCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCounts[r.URL.Path]++
		requestMu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<a href="/one">one</a><a href="/two">two</a><a href="/three">three</a><a href="/four">four</a>`))
			return
		}
		_, _ = w.Write([]byte("done"))
	}))
	defer server.Close()

	var foundMu sync.Mutex
	var found []string
	c, err := NewCrawler(
		server.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithMaxRequestCount(10),
		WithMaxUrlCount(2),
		WithOnUrlFound(func(rawURL string) {
			foundMu.Lock()
			found = append(found, rawURL)
			foundMu.Unlock()
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	foundMu.Lock()
	require.Equal(t, []string{server.URL + "/one", server.URL + "/two"}, found)
	foundMu.Unlock()
	requestMu.Lock()
	require.Equal(t, 1, requestCounts["/"])
	require.Equal(t, 1, requestCounts["/one"])
	require.Equal(t, 1, requestCounts["/two"])
	require.Zero(t, requestCounts["/three"])
	require.Zero(t, requestCounts["/four"])
	requestMu.Unlock()
	require.EqualValues(t, 2, c.linkCounter)
	require.Equal(t, 2, countCoreURLBudgetEntries(c.reportedUrls))
}

func TestCoreURLDiscoveryBudgetCountsResponseHeaderCandidates(t *testing.T) {
	var requestMu sync.Mutex
	requestCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCounts[r.URL.Path]++
		requestMu.Unlock()
		if r.URL.Path == "/" {
			w.Header().Set("SourceMap", "/assets/first.map")
			w.Header().Set("X-SourceMap", "/assets/second.map")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var foundMu sync.Mutex
	var found []string
	c, err := NewCrawler(
		server.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxRequestCount(10),
		WithMaxUrlCount(1),
		WithOnUrlFound(func(rawURL string) {
			foundMu.Lock()
			found = append(found, rawURL)
			foundMu.Unlock()
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	foundMu.Lock()
	require.Equal(t, []string{server.URL + "/assets/first.map"}, found)
	foundMu.Unlock()
	requestMu.Lock()
	require.Equal(t, 1, requestCounts["/"])
	require.Equal(t, 1, requestCounts["/assets/first.map"])
	require.Zero(t, requestCounts["/assets/second.map"])
	requestMu.Unlock()
	require.EqualValues(t, 1, c.linkCounter)
	require.Equal(t, 1, countCoreURLBudgetEntries(c.reportedUrls))
}

func TestCoreURLDiscoveryBudgetCountsOutOfScopeAIFindings(t *testing.T) {
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`const endpoint=routeParts.join("/");fetch(endpoint)`))
	}))
	defer seed.Close()

	var aiCalls atomic.Int64
	var requested atomic.Int64
	var foundMu sync.Mutex
	var found []string
	c, err := NewCrawler(
		seed.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxRequestCount(10),
		WithMaxUrlCount(1),
		WithOnRequest(func(*Req) {
			requested.Add(1)
		}),
		WithOnUrlFound(func(rawURL string) {
			foundMu.Lock()
			found = append(found, rawURL)
			foundMu.Unlock()
		}),
		WithAIJSExtract(
			WithAIJS_AdaptiveTrigger(),
			WithAIJS_MaxRequests(1),
			withAIJSInvoker(func(_ context.Context, _ *AIJSExtractConfig, _ string, onPath func(string)) error {
				aiCalls.Add(1)
				onPath("https://outside.invalid/one")
				onPath("https://outside.invalid/two")
				return nil
			}),
		),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())
	require.EqualValues(t, 1, aiCalls.Load())
	require.EqualValues(t, 1, requested.Load(), "out-of-scope AI discoveries must never be scheduled")
	foundMu.Lock()
	require.Equal(t, []string{"https://outside.invalid/one"}, found)
	foundMu.Unlock()
	require.EqualValues(t, 1, c.linkCounter)
	require.Equal(t, 1, countCoreURLBudgetEntries(c.reportedUrls))
}

func TestCoreURLDiscoveryBudgetIsConcurrentUniqueAndSeedFree(t *testing.T) {
	const (
		limit      = 17
		candidates = 64
		repeats    = 8
	)

	var callbackCount atomic.Int64
	c, err := NewCrawler(
		"https://seed.example.test/root",
		WithExactOrigins(),
		WithMaxUrlCount(limit),
		WithOnUrlFound(func(string) {
			callbackCount.Add(1)
		}),
	)
	require.NoError(t, err)

	var admitted atomic.Int64
	var wg sync.WaitGroup
	for repeat := 0; repeat < repeats; repeat++ {
		for candidate := 0; candidate < candidates; candidate++ {
			candidateURL := fmt.Sprintf("https://seed.example.test/discovered/%d", candidate)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if c.reportDiscoveredURL(candidateURL) {
					admitted.Add(1)
				}
				// Fragment variants and repeated seed sightings must not consume
				// another slot or grow the retained URL set.
				_ = c.reportDiscoveredURL(candidateURL + "#fragment")
				_ = c.reportDiscoveredURL("https://seed.example.test/root")
			}()
		}
	}
	wg.Wait()

	require.EqualValues(t, limit, admitted.Load())
	require.EqualValues(t, limit, callbackCount.Load())
	require.EqualValues(t, limit, c.linkCounter)
	require.Equal(t, limit, countCoreURLBudgetEntries(c.reportedUrls))
}

func TestCoreURLDiscoveryBudgetNonPositiveIsUnlimited(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			c, err := NewCrawler(
				"https://seed.example.test/",
				WithExactOrigins(),
				WithMaxUrlCount(limit),
			)
			require.NoError(t, err)
			for index := 0; index < 128; index++ {
				require.True(t, c.reportDiscoveredURL(fmt.Sprintf("https://seed.example.test/%d", index)))
			}
			require.EqualValues(t, 128, c.linkCounter)
			require.Equal(t, 128, countCoreURLBudgetEntries(c.reportedUrls))
		})
	}
}

func TestCoreURLSchedulerCapacityPreservesLateValidDiscovery(t *testing.T) {
	var lateRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/" {
			// These candidates are reported and queued first, then rejected by
			// preReq because images are excluded. Historically they could fill the
			// small request-count-sized queue and permanently drop /late-valid even
			// though the rejected entries consume no request budget.
			for index := 0; index < 8; index++ {
				_, _ = fmt.Fprintf(w, `<a href="/discard/%d.png">discard</a>`, index)
			}
			_, _ = w.Write([]byte(`<a href="/late-valid">late valid</a>`))
			return
		}
		if r.URL.Path == "/late-valid" {
			lateRequests.Add(1)
		}
		_, _ = w.Write([]byte("done"))
	}))
	defer server.Close()

	c, err := NewCrawler(
		server.URL,
		WithExactOrigins(),
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithMaxRequestCount(2),
		WithMaxUrlCount(9),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())
	require.EqualValues(t, 1, lateRequests.Load(), "a transient queue fill must not lose a later valid discovery")
	require.EqualValues(t, 2, c.requestCounter, "preReq-rejected candidates must not consume the request budget")
}

func TestCoreURLSchedulerIsUnlimitedWhenMaxURLsIsNonPositive(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			c, err := NewCrawler(
				"https://seed.example.test/",
				WithExactOrigins(),
				WithMaxUrlCount(limit),
			)
			require.NoError(t, err)
			c.initScheduler()
			defer c.scheduler.Close()
			require.Zero(t, c.scheduler.queueCapacity)
			for index := 0; index < 128; index++ {
				require.True(t, c.scheduler.Submit(&Req{}))
			}
		})
	}
}

func countCoreURLBudgetEntries(values *sync.Map) int {
	if values == nil {
		return 0
	}
	count := 0
	values.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
