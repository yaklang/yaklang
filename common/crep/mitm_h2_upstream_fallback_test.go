package crep

// Regression test: when the origin negotiates h2 in ALPN (so the probe marks
// it h2-capable) but then kills actual h2 connections right after the client
// preface — the behavior of fingerprinting-protected endpoints such as
// browser vendors' own APIs — the MITM must downgrade the upstream to
// HTTP/1.1 (Burp semantics) instead of retrying h2 forever.

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
)

// h2KillerOrigin negotiates h2 via ALPN but immediately closes any connection
// that sends the HTTP/2 client preface; plain HTTP/1.1 requests are answered
// normally. It emulates endpoints that fingerprint-kill non-browser h2 stacks.
type h2KillerOrigin struct {
	addr        string
	h2Kills     int32
	h1Requests  int32
	probeChecks int32
}

func newH2KillerOrigin(t *testing.T, body string) *h2KillerOrigin {
	t.Helper()
	cert := h2GenCert(t)
	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	})
	require.NoError(t, err)
	o := &h2KillerOrigin{addr: lis.Addr().String()}
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go o.handle(conn, body)
		}
	}()
	t.Cleanup(func() { lis.Close() })
	return o
}

func (o *h2KillerOrigin) handle(conn net.Conn, body string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	peek, err := br.Peek(3)
	if err != nil {
		return
	}
	if string(peek) == "PRI" {
		// h2 client preface: kill the connection like a fingerprinting WAF
		atomic.AddInt32(&o.h2Kills, 1)
		return
	}
	// plain HTTP/1.x request: read headers, then answer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" {
			break
		}
	}
	atomic.AddInt32(&o.h1Requests, 1)
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
}

// tarpitH2Origin negotiates h2 in ALPN but then never sends any frame (and
// never closes) — the fingerprinting-WAF tarpit behavior seen in production.
// HTTP/1.1 requests are answered normally.
type tarpitH2Origin struct {
	addr    string
	h2Conns int32
	h1Conns int32
}

func newTarpitH2Origin(t *testing.T, body string) *tarpitH2Origin {
	t.Helper()
	cert := h2GenCert(t)
	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	})
	require.NoError(t, err)
	o := &tarpitH2Origin{addr: lis.Addr().String()}
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				tc := conn.(*tls.Conn)
				_ = tc.Handshake()
				if tc.ConnectionState().NegotiatedProtocol == "h2" {
					atomic.AddInt32(&o.h2Conns, 1)
					_, _ = io.Copy(io.Discard, conn) // tarpit: never respond
					return
				}
				atomic.AddInt32(&o.h1Conns, 1)
				br := bufio.NewReader(conn)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if line == "\r\n" {
						break
					}
				}
				fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			}()
		}
	}()
	t.Cleanup(func() { lis.Close() })
	return o
}

// TestMITMH2_UpstreamTarpitFallsBackToH1 covers origins that negotiate h2 in
// ALPN but then go silent (no SETTINGS frame). The probe now requires the
// server's SETTINGS preface, so such origins are cached as h1 and requests
// never attempt h2 at all. The bounded server-preface wait at conn setup and
// the request-level fallback remain as defense in depth.
func TestMITMH2_UpstreamTarpitFallsBackToH1(t *testing.T) {
	token := utils.RandStringBytes(16)
	origin := newTarpitH2Origin(t, token)

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

	client := h2BrowserClientThroughProxy(t, addr, 15*time.Second, nil)
	for i := 1; i <= 3; i++ {
		start := time.Now()
		resp, err := client.Get("https://" + origin.addr + "/")
		elapsed := time.Since(start)
		require.NoError(t, err, "request %d must succeed via h1", i)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, token, string(body))
		t.Logf("request %d OK in %v (h2 conns: %d, h1 conns: %d)",
			i, elapsed, atomic.LoadInt32(&origin.h2Conns), atomic.LoadInt32(&origin.h1Conns))
		require.Less(t, elapsed, 8*time.Second, "request %d must not hang on the h2 tarpit", i)
	}
	require.Equal(t, int32(3), atomic.LoadInt32(&origin.h1Conns), "all requests must be served over HTTP/1.1")
}

// TestMITMH2_UpstreamFallbackToH1 sends requests through the MITM (h2
// enabled) to an origin that advertises h2 but kills h2 connections. The
// first h2 upstream attempt must fail fast and fall back to HTTP/1.1; later
// requests must go straight to HTTP/1.1 via the downgraded cache.
func TestMITMH2_UpstreamFallbackToH1(t *testing.T) {
	token := utils.RandStringBytes(16)
	origin := newH2KillerOrigin(t, token)

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

	client := h2BrowserClientThroughProxy(t, addr, 15*time.Second, nil)
	for i := 1; i <= 3; i++ {
		start := time.Now()
		resp, err := client.Get("https://" + origin.addr + "/")
		elapsed := time.Since(start)
		require.NoError(t, err, "request %d must succeed via h1 fallback", i)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, token, string(body))
		t.Logf("request %d OK in %v (h2 kills: %d, h1 requests: %d)",
			i, elapsed, atomic.LoadInt32(&origin.h2Kills), atomic.LoadInt32(&origin.h1Requests))
		require.Less(t, elapsed, 8*time.Second, "request %d must not stall on h2 retries", i)
	}
	require.GreaterOrEqual(t, atomic.LoadInt32(&origin.h1Requests), int32(3),
		"all requests must end up served over HTTP/1.1")
}

// TestMITMH2_DisplayKeepsClientFacingProtocol verifies that when the client
// speaks h2 to the MITM but the upstream is served over HTTP/1.1, the
// recorded/displayed request packet still carries the "HTTP/2" version marker
// (the client-facing protocol), while the wire request to the origin is h1.
func TestMITMH2_DisplayKeepsClientFacingProtocol(t *testing.T) {
	token := utils.RandStringBytes(16)
	originAddr := newRawH1Origin(t, token)

	mirrorCh := make(chan int, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mServer, err := NewMITMServer(
		MITM_SetHTTP2(true),
		MITM_SetDisableSystemProxy(true),
		MITM_SetHTTPResponseMirrorInstance(func(isHttps bool, req, rsp []byte, remoteAddr string, response *http.Response) {
			if response != nil && response.Request != nil {
				mirrorCh <- response.Request.ProtoMajor
			}
		}),
	)
	require.NoError(t, err)
	ready := make(chan struct{})
	go func() {
		_ = mServer.ServeWithListenedCallback(ctx, addr, func() { close(ready) })
	}()
	<-ready
	time.Sleep(100 * time.Millisecond)

	client := h2BrowserClientThroughProxy(t, addr, 15*time.Second, nil)
	resp, err := client.Get("https://" + originAddr + "/")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, token, string(body))

	select {
	case protoMajor := <-mirrorCh:
		require.Equal(t, 2, protoMajor,
			"recorded request must keep the client-facing h2 protocol (displayed as HTTP/2)")
	case <-time.After(5 * time.Second):
		t.Fatal("mirror callback did not fire")
	}
}

// TestMITMH2_FirstRequestUsesH2WhenOriginSupportsIt verifies that even the
// FIRST request to a healthy h2 origin is proxied over h2: the background
// probe is given a brief bounded moment (h2FirstRequestProbeWait) instead of
// unconditionally serving the first request over HTTP/1.1.
func TestMITMH2_FirstRequestUsesH2WhenOriginSupportsIt(t *testing.T) {
	cert := h2GenCert(t)
	var h2Hits int32
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ProtoMajor == 2 {
				atomic.AddInt32(&h2Hits, 1)
			}
			fmt.Fprint(w, "ok")
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2"},
		},
	}
	require.NoError(t, http2.ConfigureServer(srv, &http2.Server{}))
	lis, err := tls.Listen("tcp", "127.0.0.1:0", srv.TLSConfig)
	require.NoError(t, err)
	go srv.Serve(lis)
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		srv.Shutdown(ctx)
	})

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

	client := h2BrowserClientThroughProxy(t, addr, 15*time.Second, nil)
	start := time.Now()
	resp, err := client.Get("https://" + lis.Addr().String() + "/")
	elapsed := time.Since(start)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, "ok", string(body))
	t.Logf("first request OK in %v (h2 origin hits: %d)", elapsed, atomic.LoadInt32(&h2Hits))
	require.Equal(t, int32(1), atomic.LoadInt32(&h2Hits),
		"the first request to a healthy h2 origin must be proxied over h2")
	require.Less(t, elapsed, 8*time.Second)
}
