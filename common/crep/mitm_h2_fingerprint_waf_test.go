package crep

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

// h2FingerprintWAF models an origin that advertises h2 and completes the h2
// connection preface — so any ALPN+SETTINGS probe concludes "h2 works" — but
// never answers an actual h2 request. Browser-fingerprinting endpoints behave
// exactly like this: real browsers get h2, non-browser h2 clients get stalled.
// Over HTTP/1.1 the same origin answers normally.
func h2FingerprintWAF(t *testing.T) (addr string, h2Requests *int32) {
	t.Helper()
	cert := h2GenCert(t)
	var h2ReqCount int32

	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { lis.Close() })

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				tc := conn.(*tls.Conn)
				if err := tc.Handshake(); err != nil {
					return
				}
				if tc.ConnectionState().NegotiatedProtocol != "h2" {
					// HTTP/1.1: behave like a healthy origin.
					br := bufio.NewReader(conn)
					for {
						req, err := http.ReadRequest(br)
						if err != nil {
							return
						}
						io.Copy(io.Discard, req.Body)
						fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok")
					}
				}

				// h2: delay the connection preface a little, then send it, so a
				// probe still concludes "h2 works" but finishes *after* a real
				// request has already failed — the ordering seen in the field.
				time.Sleep(300 * time.Millisecond)
				fr := http2.NewFramer(conn, conn)
				if err := fr.WriteSettings(); err != nil {
					return
				}
				// Consume the client connection preface before reading frames;
				// feeding it to the framer would look like a malformed frame.
				preface := make([]byte, len(http2.ClientPreface))
				if _, err := io.ReadFull(conn, preface); err != nil {
					t.Logf("origin: reading client preface failed: %v", err)
					return
				}
				// A real request (HEADERS) gets the connection dropped, the way
				// these endpoints reject non-browser h2 clients.
				for {
					f, err := fr.ReadFrame()
					if err != nil {
						t.Logf("origin: ReadFrame ended: %v", err)
						return
					}
					t.Logf("origin: got frame %T %v", f, f.Header())
					if _, ok := f.(*http2.HeadersFrame); ok {
						atomic.AddInt32(&h2ReqCount, 1)
						return
					}
				}
			}(conn)
		}
	}()
	return lis.Addr().String(), &h2ReqCount
}

// TestMITMH2_FingerprintWAFDoesNotStallLaterRequests reproduces the reported
// stall: the first request downgrades and succeeds, and every later request
// must keep using HTTP/1.1 instead of re-attempting the h2 path that only
// looks healthy to a probe.
func TestMITMH2_FingerprintWAFDoesNotStallLaterRequests(t *testing.T) {
	originAddr, h2Requests := h2FingerprintWAF(t)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
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

	target := "https://" + originAddr + "/bitbrowser/v1/apis/ipgeoMethod"
	var elapsedPerRequest []time.Duration
	var attemptsAfterFirst int32

	for i := 1; i <= 4; i++ {
		// Fresh HTTP/1.1 client each round, like a desktop client that opens a
		// new connection per check.
		client := h1ClientThroughProxy(t, addr, 120*time.Second)
		start := time.Now()
		done := make(chan error, 1)
		go func() {
			resp, err := client.Post(target, "application/json", nil)
			if err != nil {
				done <- err
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			done <- nil
		}()

		select {
		case err := <-done:
			elapsed := time.Since(start)
			require.NoErrorf(t, err, "request %d failed after %v", i, elapsed)
			t.Logf("request %d OK in %v (upstream h2 attempts so far: %d)",
				i, elapsed, atomic.LoadInt32(h2Requests))
			elapsedPerRequest = append(elapsedPerRequest, elapsed)
			if i == 1 {
				attemptsAfterFirst = atomic.LoadInt32(h2Requests)
			}
		case <-time.After(20 * time.Second):
			dumpGoroutines(t, fmt.Sprintf("request %d hung", i))
			t.Fatalf("request %d hung — stall reproduced", i)
		}
		client.CloseIdleConnections()
	}

	// The first request pays for the h2 attempt (plus its bounded reconnects)
	// before it learns better; every later one must go straight over HTTP/1.1.
	for i, elapsed := range elapsedPerRequest {
		if i == 0 {
			continue
		}
		require.Lessf(t, elapsed, 5*time.Second,
			"request %d took %v — it re-attempted the h2 path instead of remembering the downgrade", i+1, elapsed)
	}

	// The whole point: h2 attempts stop after the first request settles the
	// origin. They must not keep growing with the number of requests, and the
	// first request's own attempts must stay bounded by the reconnect cap.
	require.Equalf(t, attemptsAfterFirst, atomic.LoadInt32(h2Requests),
		"requests 2..n re-attempted h2: %d attempts after the first request, %d at the end",
		attemptsAfterFirst, atomic.LoadInt32(h2Requests))
	require.LessOrEqualf(t, attemptsAfterFirst, int32(4),
		"the first request retried h2 %d times — the reconnect cap is not holding", attemptsAfterFirst)
}
