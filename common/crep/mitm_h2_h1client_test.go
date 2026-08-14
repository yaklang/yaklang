package crep

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
)

// h1ClientThroughProxy builds a plain HTTP/1.1 client that talks to the MITM
// through CONNECT — what curl and most scanners do by default. The client side
// never negotiates h2, while the upstream hop still does, which is the exact
// combination browsers never exercise.
func h1ClientThroughProxy(t *testing.T, proxyAddr string, timeout time.Duration) *http.Client {
	t.Helper()
	proxyURL, err := url.Parse("http://" + proxyAddr)
	require.NoError(t, err)
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"http/1.1"},
			},
			ForceAttemptHTTP2: false,
		},
	}
}

// TestMITMH2_H1ClientRepeatedRequestsToSameOrigin reproduces the curl/scanner
// case: an HTTP/1.1 client hitting the same h2 origin several times. The first
// request establishes the upstream h2 connection; every later one reuses it.
func TestMITMH2_H1ClientRepeatedRequestsToSameOrigin(t *testing.T) {
	cert := h2GenCert(t)
	var originHits int32
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&originHits, 1)
			fmt.Fprintf(w, "hit-%d-proto-%d", n, r.ProtoMajor)
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

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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

	target := "https://" + lis.Addr().String() + "/"

	for i := 1; i <= 4; i++ {
		// A fresh client per iteration mimics separate curl invocations: the
		// client-side connection is new every time, but the MITM's upstream
		// connection to the origin is pooled and reused.
		client := h1ClientThroughProxy(t, addr, 12*time.Second)
		start := time.Now()
		done := make(chan error, 1)
		var body []byte
		go func() {
			resp, err := client.Get(target)
			if err != nil {
				done <- err
				return
			}
			defer resp.Body.Close()
			body, err = io.ReadAll(resp.Body)
			done <- err
		}()

		select {
		case err := <-done:
			elapsed := time.Since(start)
			require.NoErrorf(t, err, "request %d failed after %v", i, elapsed)
			t.Logf("request %d OK in %v -> %s", i, elapsed, string(body))
			require.Lessf(t, elapsed, 5*time.Second, "request %d took %v — upstream reuse is stalling", i, elapsed)
		case <-time.After(15 * time.Second):
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Logf("=== GOROUTINE DUMP AT STALL (request %d) ===\n%s", i, buf[:n])
			t.Fatalf("request %d hung — reproduced the stall", i)
		}
		client.CloseIdleConnections()
	}
}
