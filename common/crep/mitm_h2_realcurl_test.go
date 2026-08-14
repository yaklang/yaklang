package crep

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// TestMITMH2_RealCurlRepeated drives the real curl binary through the MITM at a
// real origin, repeatedly. Go's http client is far more forgiving than curl
// about connection reuse and framing, so a stall that only curl sees cannot be
// reproduced with an in-process client.
//
// MITM_H2_CURL_TARGET overrides the origin.
func TestMITMH2_RealCurlRepeated(t *testing.T) {
	// Opt-in only: this drives a real curl binary against a real origin, so it
	// needs egress and is not suitable for CI. Set MITM_H2_CURL_TARGET to run
	// it against a specific endpoint when diagnosing a report from the field.
	target := os.Getenv("MITM_H2_CURL_TARGET")
	if target == "" {
		t.Skip("set MITM_H2_CURL_TARGET=<url> to run this diagnostic against a real origin")
	}
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
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
	time.Sleep(200 * time.Millisecond)

	run := func(i int, extraArgs ...string) {
		args := append([]string{
			"-sS", "-o", "/dev/null",
			"-x", "http://" + addr,
			"-k",
			"--max-time", "15",
			"-w", "http_version=%{http_version} code=%{http_code} total=%{time_total}",
		}, extraArgs...)
		args = append(args, target)

		start := time.Now()
		cmd := exec.Command(curlPath, args...)
		outCh := make(chan struct {
			out []byte
			err error
		}, 1)
		go func() {
			out, err := cmd.CombinedOutput()
			outCh <- struct {
				out []byte
				err error
			}{out, err}
		}()

		select {
		case r := <-outCh:
			elapsed := time.Since(start)
			t.Logf("curl #%d (%v) [%v]: %s", i, strings.Join(extraArgs, " "), elapsed, strings.TrimSpace(string(r.out)))
			require.NoErrorf(t, r.err, "curl #%d failed after %v: %s", i, elapsed, string(r.out))
			if elapsed > 14*time.Second {
				dumpGoroutines(t, fmt.Sprintf("curl #%d slow (%v)", i, elapsed))
				t.Fatalf("curl #%d took %v", i, elapsed)
			}
		case <-time.After(20 * time.Second):
			// curl's own --max-time should have fired by now; if we are still
			// here the proxy is wedged, so capture where.
			dumpGoroutines(t, fmt.Sprintf("curl #%d hung", i))
			_ = cmd.Process.Kill()
			t.Fatalf("curl #%d hung — stall reproduced", i)
		}
	}

	rounds := 4
	if v := os.Getenv("MITM_H2_CURL_ROUNDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rounds = n
		}
	}

	t.Run("default-http1-client", func(t *testing.T) {
		for i := 1; i <= rounds; i++ {
			run(i)
		}
	})

	t.Run("http2-client", func(t *testing.T) {
		for i := 1; i <= rounds; i++ {
			run(i, "--http2")
		}
	})

	// Keep-alive on one client connection: several requests share a single
	// client-side connection while the upstream connection is also pooled.
	t.Run("keepalive-same-connection", func(t *testing.T) {
		urls := make([]string, 0, rounds)
		for i := 0; i < rounds; i++ {
			urls = append(urls, target)
		}
		args := append([]string{
			"-sS", "-o", "/dev/null",
			"-x", "http://" + addr,
			"-k",
			"--max-time", "30",
			"-w", "http_version=%{http_version} code=%{http_code} total=%{time_total}\\n",
		}, urls...)

		start := time.Now()
		cmd := exec.Command(curlPath, args...)
		outCh := make(chan struct {
			out []byte
			err error
		}, 1)
		go func() {
			out, err := cmd.CombinedOutput()
			outCh <- struct {
				out []byte
				err error
			}{out, err}
		}()
		select {
		case r := <-outCh:
			t.Logf("curl keep-alive x%d [%v]:\n%s", rounds, time.Since(start), strings.TrimSpace(string(r.out)))
			require.NoErrorf(t, r.err, "keep-alive run failed: %s", string(r.out))
		case <-time.After(40 * time.Second):
			dumpGoroutines(t, "curl keep-alive hung")
			_ = cmd.Process.Kill()
			t.Fatal("keep-alive run hung — stall reproduced")
		}
	})
}

// dumpGoroutines writes every goroutine stack into the test log so a stall
// shows its location instead of having to be guessed at.
func dumpGoroutines(t *testing.T, reason string) {
	t.Helper()
	buf := make([]byte, 1<<21)
	n := runtime.Stack(buf, true)
	t.Logf("=== GOROUTINE DUMP (%s) ===\n%s", reason, buf[:n])
}
