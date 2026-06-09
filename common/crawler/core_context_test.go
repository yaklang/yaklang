package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCrawler_RunWithCanceledOptionContextReturns(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c, err := NewCrawler(ts.URL, WithForbiddenFromParent(true), WithContext(ctx))
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- c.Run()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("crawler did not return after context cancellation")
	}
}

func TestStartCrawlerWithOptionContextCancelClosesOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := StartCrawler(ts.URL, WithForbiddenFromParent(true), WithContext(ctx))
	require.NoError(t, err)
	cancel()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("crawler output channel did not close after context cancellation")
		}
	}
}

func TestCrawler_RunCompletesAfterDiscoveredRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<a href="/next">next</a>`))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	var mu sync.Mutex
	seen := map[string]bool{}
	c, err := NewCrawler(
		ts.URL,
		WithForbiddenFromParent(true),
		WithOnRequest(func(req *Req) {
			mu.Lock()
			seen[req.Request().URL.Path] = true
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- c.Run()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("crawler did not naturally finish after discovered requests")
	}

	mu.Lock()
	defer mu.Unlock()
	require.True(t, seen["/"])
	require.True(t, seen["/next"])
}

func TestCrawler_RunNaturalCompletionDoesNotLeaveSchedulerWatcher(t *testing.T) {
	before := runtime.NumGoroutine()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c, err := NewCrawler(ts.URL, WithForbiddenFromParent(true))
	require.NoError(t, err)
	require.NoError(t, c.Run())

	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before+4
	}, time.Second, 20*time.Millisecond)
}
