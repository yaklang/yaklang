package lowhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnPoolH1AcquiresConnectionAtCapacity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		alreadyIdle bool
		sameOrigin  bool
	}{
		{name: "reclaim idle connection", alreadyIdle: true},
		{name: "reuse connection returned while waiting", sameOrigin: true},
		{name: "reclaim connection returned while waiting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 2)
			releaseFirst := make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseFirst) })
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.RemoteAddr
				if r.URL.Path == "/first" {
					<-releaseFirst
				}
				_, _ = io.WriteString(w, "ok")
			})
			firstOrigin := httptest.NewServer(handler)
			defer firstOrigin.Close()
			secondOrigin := firstOrigin
			if !tc.sameOrigin {
				secondOrigin = httptest.NewServer(handler)
				defer secondOrigin.Close()
			}
			defer release()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool := NewHttpConnPool(ctx, 1, 1)
			defer pool.Clear()

			send := func(origin, path string) <-chan error {
				result := make(chan error, 1)
				go func() {
					host := strings.TrimPrefix(origin, "http://")
					_, err := HTTPWithoutRedirect(
						WithPacketBytes([]byte(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", path, host))),
						WithContext(ctx), WithConnPool(true), ConnPool(pool),
						WithConnectTimeout(2*time.Second),
					)
					result <- err
				}()
				return result
			}

			first := send(firstOrigin.URL, "/first")
			var firstPeer string
			select {
			case firstPeer = <-requests:
			case <-ctx.Done():
				t.Fatal("the first request did not reach the origin")
			}
			if tc.alreadyIdle {
				release()
				require.NoError(t, <-first)
				require.Eventually(t, func() bool {
					pool.idleConnMux.Lock()
					defer pool.idleConnMux.Unlock()
					return len(pool.idleConnMap) == 1 && len(pool.connSem) == 1
				}, time.Second, time.Millisecond)
			}

			started := time.Now()
			second := send(secondOrigin.URL, "/second")
			if !tc.alreadyIdle {
				require.Eventually(t, func() bool {
					pool.idleConnMux.Lock()
					defer pool.idleConnMux.Unlock()
					return pool.h1IdleReady != nil
				}, time.Second, time.Millisecond, "the second request should wait while the slot is in use")
				release()
				require.NoError(t, <-first)
			}
			require.NoError(t, <-second)
			t.Logf("second request completed in %s", time.Since(started))
			if tc.sameOrigin {
				require.Equal(t, firstPeer, <-requests, "the waiter should reuse the returned connection")
			}
		})
	}
}

func TestConnPoolH1SlotWaitRespectsConnectTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool := NewHttpConnPool(ctx, 1, 1)
	defer pool.Clear()
	key := &connectKey{scheme: H1, addr: "127.0.0.1:1"}
	_, err := pool.acquireH1Conn(ctx, key, 0)
	require.NoError(t, err)
	defer pool.releaseConnSlot()

	_, err = HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1\r\n\r\n")),
		WithContext(ctx), WithConnPool(true), ConnPool(pool),
		WithConnectTimeout(40*time.Millisecond),
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "waiting for HTTP/1.1 connection slot")
	require.Len(t, pool.connSem, 1)
}
