package crep

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// TestMITMH2_RealOriginSequential proxies several requests to a real public h2
// origin. Local mock servers answer instantly and always send their SETTINGS
// frame first, so they cannot reproduce stalls that depend on real handshake
// timing, CDNs, or connection reuse across a WAN link.
//
// Set MITM_H2_REAL_TARGET to override the origin; skipped without network.
func TestMITMH2_RealOriginSequential(t *testing.T) {
	// Opt-in only: needs egress to a real origin, so it must not run in CI.
	target := os.Getenv("MITM_H2_REAL_TARGET")
	if target == "" {
		t.Skip("set MITM_H2_REAL_TARGET=<url> to run this diagnostic against a real origin")
	}

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

	client := h2BrowserClientThroughProxy(t, addr, 20*time.Second, nil)

	for i := 1; i <= 5; i++ {
		start := time.Now()
		done := make(chan struct {
			resp *http.Response
			err  error
		}, 1)
		go func() {
			resp, err := client.Get(target)
			done <- struct {
				resp *http.Response
				err  error
			}{resp, err}
		}()

		select {
		case r := <-done:
			elapsed := time.Since(start)
			require.NoErrorf(t, r.err, "request %d failed after %v", i, elapsed)
			n, _ := io.Copy(io.Discard, r.resp.Body)
			r.resp.Body.Close()
			t.Logf("request %d OK in %v (proto=%s, status=%d, body=%d bytes)",
				i, elapsed, r.resp.Proto, r.resp.StatusCode, n)
		case <-time.After(25 * time.Second):
			// Dump every goroutine so the stall's location is in the output
			// instead of having to be guessed at.
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Logf("=== GOROUTINE DUMP AT STALL (request %d) ===\n%s", i, buf[:n])
			t.Fatalf("request %d hung for more than 25s", i)
		}
	}
}
