package lowhttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
)

type legacyProxyCloseSpy struct {
	net.Conn
	closes *atomic.Int32
}

func (c *legacyProxyCloseSpy) Close() error {
	c.closes.Add(1)
	return c.Conn.Close()
}

func assertNoLegacyProxyPoolEntries(t *testing.T, pool *LowHttpConnPool) {
	t.Helper()
	pool.idleConnMux.Lock()
	idle := len(pool.idleConnMap)
	pool.idleConnMux.Unlock()
	require.Zero(t, idle, "a legacy forward-proxy connection entered the H1 pool")
	require.Zero(t, len(pool.connSem), "legacy fallback retained an H1 pool slot")
}

func TestLegacyProxyFallbackNeverUsesConnPool(t *testing.T) {
	for _, forceLegacy := range []bool{false, true} {
		for _, withPool := range []bool{false, true} {
			name := "normal_proxy"
			if forceLegacy {
				name = "forced_legacy"
			}
			if withPool {
				name += "_with_pool"
			} else {
				name += "_direct"
			}
			t.Run(name, func(t *testing.T) {
				var connects, forwarded, opened, closed atomic.Int32
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodConnect {
						connects.Add(1)
						w.WriteHeader(http.StatusMethodNotAllowed)
						return
					}
					if r.Method != http.MethodGet || r.RequestURI != "http://target.invalid/path" || r.Host != "target.invalid" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					forwarded.Add(1)
					_, _ = io.WriteString(w, "legacy-ok")
				}))
				server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
					if state == http.StateNew {
						opened.Add(1)
					}
				}
				server.Start()
				defer server.Close()

				pool := NewHttpConnPool(context.Background(), 2, 2)
				defer pool.Clear()
				for i := 0; i < 2; i++ {
					rsp, err := HTTPWithoutRedirect(
						WithPacketBytes([]byte("GET /path HTTP/1.1\r\nHost: target.invalid\r\n\r\n")),
						WithHost("target.invalid"), WithPort(80), WithProxy(server.URL),
						WithForceLegacyProxy(forceLegacy), WithConnPool(withPool), ConnPool(pool),
						WithEnableSystemProxyFromEnv(false), WithDisableSession(true),
						WithTimeout(2*time.Second), WithConnectTimeout(time.Second),
						WithDialer(func(timeout time.Duration, addr string) (net.Conn, error) {
							conn, err := net.DialTimeout("tcp", addr, timeout)
							if err != nil {
								return nil, err
							}
							return &legacyProxyCloseSpy{Conn: conn, closes: &closed}, nil
						}),
					)
					require.NoError(t, err)
					require.Equal(t, "legacy-ok", string(GetHTTPPacketBody(rsp.RawPacket)))
					assertNoLegacyProxyPoolEntries(t, pool)
				}
				require.Equal(t, int32(2), forwarded.Load())
				require.GreaterOrEqual(t, opened.Load(), int32(2), "forward-proxy connections must not be reused")
				require.GreaterOrEqual(t, closed.Load(), int32(2), "each forwarded connection must be closed")
				if forceLegacy {
					require.Zero(t, connects.Load())
				} else {
					require.Equal(t, int32(2), connects.Load(), "normal CONNECT must fail before fallback")
				}
			})
		}
	}
}

func TestLegacyProxyFallbackReadErrorDoesNotEnterPool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	pool := NewHttpConnPool(context.Background(), 2, 2)
	defer pool.Clear()

	_, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET /drop HTTP/1.1\r\nHost: target.invalid\r\n\r\n")),
		WithHost("target.invalid"), WithPort(80), WithProxy(server.URL),
		WithForceLegacyProxy(true), WithConnPool(true), ConnPool(pool),
		WithEnableSystemProxyFromEnv(false), WithDisableSession(true),
		WithTimeout(time.Second), WithConnectTimeout(time.Second),
	)
	require.Error(t, err)
	assertNoLegacyProxyPoolEntries(t, pool)
}

func TestLegacyProxyFallbackBuildsPacketBeforeDial(t *testing.T) {
	var opened atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	pool := NewHttpConnPool(context.Background(), 1, 1)
	defer pool.Clear()

	tr := &transportRequest{
		option:     &LowhttpExecConfig{Proxy: []string{server.URL}, WithConnPool: true},
		packet:     []byte("GET /path HTTP/1.1\r\n\r\n"), // no Host: cannot construct an absolute URL
		dialOpts:   []netx.DialXOption{netx.DialX_WithForceProxy(true)},
		cacheKey:   &connectKey{addr: "target.invalid:80", scheme: H1},
		connPool:   pool,
		usePool:    true,
		traceInfo:  newLowhttpTraceInfo(),
		originAddr: "target.invalid:80",
		timeout:    time.Second,
	}
	_, err := newH1Transport(pool).RoundTrip(context.Background(), tr)
	require.ErrorContains(t, err, "invalid http request for legacy proxy request")
	require.Zero(t, opened.Load(), "invalid request must not open a proxy connection")
	assertNoLegacyProxyPoolEntries(t, pool)
}
