package crep

// Regression tests for the MITM HTTP/2 origin-probe stall and the h1->h2
// response header sanitization, using a browser-grade h2 client
// (golang.org/x/net/http2 Transport — the same class of stack Chromium
// ships), instead of yaklang's own h2 client.
//
// Background: with HTTP/2 support enabled, the MITM used to synchronously
// probe the origin's h2 capability on the client TLS-handshake path. A slow,
// bot-mitigated or unreachable origin stalled the probe (up to its 10s
// timeout), so the client could not even finish its handshake and no traffic
// was captured — browser proxy checks failed. The probe now runs in the
// background and unknown origins are served over HTTP/1.1 (Burp semantics).

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
)

// h2BrowserClientThroughProxy builds an x/net/http2 client that tunnels via
// CONNECT through the MITM proxy and negotiates TLS with ALPN h2, like a real
// browser. onHandshake (optional) is invoked once the TLS handshake with the
// MITM has completed.
func h2BrowserClientThroughProxy(t *testing.T, proxyAddr string, timeout time.Duration, onHandshake func()) *http.Client {
	t.Helper()
	tr := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			var d net.Dialer
			raw, err := d.DialContext(ctx, "tcp", proxyAddr)
			if err != nil {
				return nil, err
			}
			if _, err := fmt.Fprintf(raw, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr); err != nil {
				raw.Close()
				return nil, err
			}
			br := bufio.NewReader(raw)
			resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
			if err != nil {
				raw.Close()
				return nil, fmt.Errorf("read CONNECT response: %w", err)
			}
			if resp.StatusCode != 200 {
				raw.Close()
				return nil, fmt.Errorf("CONNECT failed: %s", resp.Status)
			}
			host, _, _ := net.SplitHostPort(addr)
			tc := tls.Client(&h2BufferedConn{Conn: raw, r: br}, &tls.Config{
				ServerName:         host,
				InsecureSkipVerify: true,
				NextProtos:         []string{"h2"},
			})
			if err := tc.HandshakeContext(ctx); err != nil {
				raw.Close()
				return nil, fmt.Errorf("tls handshake through proxy: %w", err)
			}
			if onHandshake != nil {
				onHandshake()
			}
			return tc, nil
		},
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

type h2BufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *h2BufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// delayedFirstReadConn delays the first read (i.e. the origin-side processing
// of the TLS ClientHello) to simulate a slow/bot-mitigated origin.
type delayedFirstReadConn struct {
	net.Conn
	once  sync.Once
	delay time.Duration
}

func (c *delayedFirstReadConn) Read(p []byte) (int, error) {
	c.once.Do(func() { time.Sleep(c.delay) })
	return c.Conn.Read(p)
}

type delayFirstConnListener struct {
	net.Listener
	delay time.Duration
	seq   int32
}

func (l *delayFirstConnListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if atomic.AddInt32(&l.seq, 1) == 1 {
		// only the first connection is slow: the (now background) h2 probe
		// eats it; the real request dials a fresh, fast connection
		return &delayedFirstReadConn{Conn: c, delay: l.delay}, nil
	}
	return c, nil
}

// newSlowH2Origin starts an h2 TLS origin whose FIRST connection pays `delay`
// before the TLS handshake completes; later connections are fast.
func newSlowH2Origin(t *testing.T, delay time.Duration) string {
	t.Helper()
	cert := h2GenCert(t)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "ok")
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2"},
		},
	}
	require.NoError(t, http2.ConfigureServer(srv, &http2.Server{}))
	rawLis, err := tls.Listen("tcp", "127.0.0.1:0", srv.TLSConfig)
	require.NoError(t, err)
	lis := &delayFirstConnListener{Listener: rawLis, delay: delay}
	go srv.Serve(lis)
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		srv.Shutdown(ctx)
	})
	return rawLis.Addr().String()
}

// TestMITMH2_OriginProbeDoesNotStallClientHandshake proves the origin h2
// probe no longer sits on the client-handshake critical path: even with a
// 4s-slow origin, the client<->MITM TLS handshake completes immediately and
// the request succeeds. Before the fix the client stalled for the whole
// probe duration (up to the 10s probe timeout) and nothing was captured.
func TestMITMH2_OriginProbeDoesNotStallClientHandshake(t *testing.T) {
	originAddr := newSlowH2Origin(t, 4*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mServer, err := NewMITMServer(MITM_SetHTTP2(true), MITM_SetDisableSystemProxy(true))
	require.NoError(t, err)
	ready := make(chan struct{})
	go func() {
		_ = mServer.ServeWithListenedCallback(ctx, addr, func() { close(ready) })
	}()
	<-ready
	time.Sleep(100 * time.Millisecond)

	var handshakeAt atomic.Int64 // ms since start
	start := time.Now()
	client := h2BrowserClientThroughProxy(t, addr, 10*time.Second, func() {
		handshakeAt.Store(time.Since(start).Milliseconds())
	})
	resp, err := client.Get("https://" + originAddr + "/")
	require.NoError(t, err, "request through MITM(h2) with slow origin must succeed")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, "ok", string(body))

	handshakeElapsed := time.Duration(handshakeAt.Load()) * time.Millisecond
	require.Less(t, handshakeElapsed, 2*time.Second,
		"client<->MITM handshake must not wait for the background origin probe")
}

// newRawH1Origin starts a raw TLS origin that only speaks HTTP/1.1 and
// answers every request with a canned response carrying connection-specific
// headers (Connection: keep-alive, Content-Length) — the kind of response
// that must never leak into an h2 stream.
func newRawH1Origin(t *testing.T, body string) string {
	t.Helper()
	cert := h2GenCert(t)
	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"http/1.1"},
	})
	require.NoError(t, err)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				br := bufio.NewReader(conn)
				// read request headers
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if line == "\r\n" {
						break
					}
				}
				fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n%s", len(body), body)
			}()
		}
	}()
	t.Cleanup(func() { lis.Close() })
	return lis.Addr().String()
}

// TestMITMH2_H1UpstreamResponseSanitized sends an h2 request through the MITM
// to an h1-only origin. The upstream h1 response (with Connection and
// Content-Length headers) is translated to h2; a strict h2 client must accept
// it. Before the header sanitization fix, connection-specific h1 headers were
// encoded into the h2 response and strict clients reset the stream
// (RFC 7540 Section 8.1.2.2).
func TestMITMH2_H1UpstreamResponseSanitized(t *testing.T) {
	token := utils.RandStringBytes(16)
	originAddr := newRawH1Origin(t, token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mServer, err := NewMITMServer(MITM_SetHTTP2(true), MITM_SetDisableSystemProxy(true))
	require.NoError(t, err)
	ready := make(chan struct{})
	go func() {
		_ = mServer.ServeWithListenedCallback(ctx, addr, func() { close(ready) })
	}()
	<-ready
	time.Sleep(100 * time.Millisecond)

	client := h2BrowserClientThroughProxy(t, addr, 10*time.Second, nil)
	resp, err := client.Get("https://" + originAddr + "/")
	require.NoError(t, err, "strict h2 client must accept the translated h1 response")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, token, string(body))
	require.False(t, strings.Contains(strings.ToLower(fmt.Sprint(resp.Header)), "keep-alive"),
		"connection-specific headers must not reach the h2 client")
}
