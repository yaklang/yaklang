package crep

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"net/http"
)

// TestMITMH2_SequentialRequestsOverSameConn drives several requests through one
// client-facing h2 connection, the way a browser does. A regression that only
// breaks connection/stream reuse looks fine on the first request and hangs from
// the second one on, so a single-request test cannot catch it.
func TestMITMH2_SequentialRequestsOverSameConn(t *testing.T) {
	cert := h2GenCert(t)
	var originHits int32
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&originHits, 1)
			fmt.Fprintf(w, "resp-%d", atomic.LoadInt32(&originHits))
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	target := "https://" + lis.Addr().String() + "/"

	for i := 1; i <= 4; i++ {
		start := time.Now()
		resp, err := client.Get(target)
		elapsed := time.Since(start)
		require.NoErrorf(t, err, "request %d failed after %v", i, elapsed)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoErrorf(t, err, "request %d body read failed after %v", i, elapsed)
		require.Equalf(t, 2, resp.ProtoMajor, "request %d must stay on h2 to the client", i)
		require.Equalf(t, fmt.Sprintf("resp-%d", i), string(body), "request %d returned the wrong body", i)
		t.Logf("request %d OK in %v (proto=%s)", i, elapsed, resp.Proto)
		require.Lessf(t, elapsed, 5*time.Second, "request %d took %v — reuse is stalling", i, elapsed)
	}
}

// TestMITMH2_ConcurrentStreamsOverSameConn fires overlapping requests down one
// client-facing h2 connection, which is what a browser actually does when it
// loads a page. Serial reuse can look healthy while concurrent streams deadlock.
func TestMITMH2_ConcurrentStreamsOverSameConn(t *testing.T) {
	cert := h2GenCert(t)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "ok:%s", r.URL.Path)
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	base := "https://" + lis.Addr().String()

	// Warm the connection so every concurrent request below shares one h2 conn.
	warm, err := client.Get(base + "/warm")
	require.NoError(t, err)
	io.Copy(io.Discard, warm.Body)
	warm.Body.Close()

	const parallel = 8
	type result struct {
		idx     int
		err     error
		body    string
		elapsed time.Duration
	}
	results := make(chan result, parallel)
	for i := 0; i < parallel; i++ {
		go func(i int) {
			start := time.Now()
			resp, err := client.Get(fmt.Sprintf("%s/p%d", base, i))
			if err != nil {
				results <- result{idx: i, err: err, elapsed: time.Since(start)}
				return
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			results <- result{idx: i, err: err, body: string(body), elapsed: time.Since(start)}
		}(i)
	}

	deadline := time.After(30 * time.Second)
	for done := 0; done < parallel; done++ {
		select {
		case r := <-results:
			require.NoErrorf(t, r.err, "concurrent request %d failed after %v", r.idx, r.elapsed)
			require.Equalf(t, fmt.Sprintf("ok:/p%d", r.idx), r.body, "concurrent request %d body mismatch", r.idx)
			t.Logf("concurrent request %d OK in %v", r.idx, r.elapsed)
		case <-deadline:
			t.Fatalf("only %d/%d concurrent requests completed — the rest are hung", done, parallel)
		}
	}
}
