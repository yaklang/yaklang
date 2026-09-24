package lowhttp

// Protocol, callback, and connection ownership regression checks.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// F03: ALPN already acquired a perfectly valid H1 socket. Do not throw it away
// and dial a second socket just to cross an internal Transport boundary.
func TestPR5180_ALPNFallbackReusesNegotiatedSocket(t *testing.T) {
	var opened atomic.Int32
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	s.StartTLS()
	t.Cleanup(s.Close)
	cfg := pr5180Config(t, s.Listener.Addr().String(), "POST", "/handoff", "HTTP/2", "once", pr5180Pool(t, 2))
	cfg.Https = true
	cfg.WithConnPool = false
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil || rsp.Http2 {
		t.Fatalf("ALPN fallback failed: rsp=%v err=%v", rsp, err)
	}
	if got := opened.Load(); got != 1 {
		t.Fatalf("ALPN fallback opened %d TCP connections; want ownership handoff of the first socket", got)
	}
}

// This guard makes a superficially attractive but incorrect fix to F01 fail.
func TestPR5180_RequestBeforeServerSettings(t *testing.T) {
	p := pr5180NewPeer(t, pr5180SettingsAfterRequest)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/preface-order", "HTTP/2", "request-first", pr5180Pool(t, 2))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cfg.Ctx = ctx
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil || !rsp.Http2 {
		t.Fatalf("request gated on server SETTINGS: rsp=%v err=%v", rsp, err)
	}
	if p.h2.Load() != 1 || p.h1.Load() != 0 {
		t.Fatalf("unexpected replay: H2=%d H1=%d", p.h2.Load(), p.h1.Load())
	}
}

// LastStreamID >= this stream ID is not an assurance of non-processing.
func TestPR5180_UnsafeFailureDoesNotReplay(t *testing.T) {
	scenarios := []struct {
		name string
		mode pr5180PeerMode
	}{
		{"tcp_loss", pr5180DropAfterRequest},
		{"goaway_accepts_stream", pr5180GoAwayAcceptedNoResponse},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			for _, method := range []string{"POST", "PATCH"} {
				t.Run(method, func(t *testing.T) {
					p := pr5180NewPeer(t, scenario.mode)
					cfg := pr5180Config(t, p.ln.Addr().String(), method, "/possibly-committed", "HTTP/2", "once", pr5180Pool(t, 2))
					_, err := HTTPWithoutRetry(cfg)
					if err == nil {
						t.Fatal("missing error for uncertain outcome")
					}
					if p.h2.Load() != 1 || p.h1.Load() != 0 {
						t.Fatalf("unsafe replay after %s: H2=%d H1=%d err=%v", scenario.name, p.h2.Load(), p.h1.Load(), err)
					}
				})
			}
		})
	}
}

func TestPR5180_RefusedPostThenSuccess(t *testing.T) {
	p := pr5180NewPeer(t, pr5180RefuseFirstStream)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/explicitly-rejected", "HTTP/2", "safe-replay", pr5180Pool(t, 2))
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil {
		t.Fatalf("safe POST replay was disabled: %v", err)
	}
	if p.h2.Load() != 2 || p.h1.Load() != 0 || p.conns.Load() != 1 {
		t.Fatalf("REFUSED_STREAM must retry the stream, not kill the connection: H2=%d H1=%d conn=%d", p.h2.Load(), p.h1.Load(), p.conns.Load())
	}
}

func TestPR5180_ClearActivePostAndRecover(t *testing.T) {
	p := pr5180NewPeer(t, pr5180HoldFirstStream)
	pool := pr5180Pool(t, 2)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/held", "HTTP/2", "once", pool)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg.Ctx = ctx
	done := make(chan error, 1)
	go func() { _, err := HTTPWithoutRetry(cfg); done <- err }()
	pr5180WaitSeen(t, p)
	pool.Clear()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Clear must abort the active request with an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active stream survived Clear")
	}
	if p.h2.Load() != 1 || p.h1.Load() != 0 {
		t.Fatalf("Clear replayed active POST: H2=%d H1=%d", p.h2.Load(), p.h1.Load())
	}
	next := pr5180Config(t, p.ln.Addr().String(), "GET", "/recovery", "HTTP/2", "", pool)
	if rsp, err := HTTPWithoutRetry(next); err != nil || rsp == nil {
		t.Fatalf("pool failed to recover: %v", err)
	}
	if p.conns.Load() != 2 || p.h2.Load() != 2 {
		t.Fatalf("unexpected recovery counts: connections=%d streams=%d", p.conns.Load(), p.h2.Load())
	}
}

// The server lowers its concurrency limit only after a successful warmup.
// Waiting for two SETTINGS ACKs ensures the client has processed the zero limit;
// no sleep is used to guess when the settings reached the state machine.
func TestPR5180_ZeroConcurrentLimitCancellation(t *testing.T) {
	p := pr5180NewPeer(t, pr5180ZeroLimitAfterFirst)
	pool := pr5180Pool(t, 2)
	warm := pr5180Config(t, p.ln.Addr().String(), "GET", "/warm", "HTTP/2", "", pool)
	if _, err := HTTPWithoutRetry(warm); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-p.settingsAcks:
		case <-time.After(2 * time.Second):
			t.Fatal("zero SETTINGS limit was not acknowledged")
		}
	}
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/must-not-send", "HTTP/2", "not-sent", pool)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cfg.Ctx = ctx
	started := time.Now()
	_, err := HTTPWithoutRetry(cfg)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting stream did not honor context: %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("slot waiter cancellation was not bounded")
	}
	if p.h2.Load() != 1 || p.h1.Load() != 0 || p.conns.Load() != 1 {
		t.Fatalf("zero limit was bypassed: H2=%d H1=%d connections=%d", p.h2.Load(), p.h1.Load(), p.conns.Load())
	}
}

type pr5180StreamCapture struct {
	body   []byte
	header []byte
	err    error
}

// The body is binary, non-text, and crosses ordinary reader buffer boundaries.
// Content-Length prevents an H1 chunked wire-body callback from changing the
// intended representation of this particular test.
func TestPR5180_BufferModesAcrossProtocols(t *testing.T) {
	payload := bytes.Repeat([]byte{0x00, 0xff, 0x80, '\r', '\n', 'X', 0x13, 0x7f}, 4096)
	modes := []struct {
		name, proto string
		pooled      bool
	}{
		{"h1_direct", "HTTP/1.1", false}, {"h1_pool", "HTTP/1.1", true}, {"h2_pool", "HTTP/2", true},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, noBuffer := range []bool{false, true} {
				t.Run(fmt.Sprintf("no_buffer_%v", noBuffer), func(t *testing.T) {
					s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/octet-stream")
						w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
						_, _ = w.Write(payload)
					}))
					s.EnableHTTP2 = true
					s.StartTLS()
					t.Cleanup(s.Close)
					var pool *LowHttpConnPool
					if mode.pooled {
						pool = pr5180Pool(t, 2)
					}
					cfg := pr5180Config(t, s.Listener.Addr().String(), "GET", "/binary", mode.proto, "", pool)
					cfg.Https = true
					cfg.NoBodyBuffer = noBuffer
					var callbacks atomic.Int32
					capture := make(chan pr5180StreamCapture, 2)
					cfg.BodyStreamReaderHandler = func(header []byte, r io.ReadCloser) {
						defer r.Close()
						callbacks.Add(1)
						body, err := io.ReadAll(io.LimitReader(r, int64(len(payload)+1)))
						select {
						case capture <- pr5180StreamCapture{body: body, header: bytes.Clone(header), err: err}:
						default:
						}
					}
					rsp, err := HTTPWithoutRetry(cfg)
					if err != nil || rsp == nil {
						t.Fatalf("request failed: %v", err)
					}
					if callbacks.Load() != 1 {
						t.Fatalf("callback count=%d; want exactly one", callbacks.Load())
					}
					select {
					case got := <-capture:
						if got.err != nil || !bytes.Equal(got.body, payload) || len(got.header) == 0 {
							t.Fatalf("stream capture differs: body=%d expected=%d header=%d err=%v", len(got.body), len(payload), len(got.header), got.err)
						}
					case <-time.After(time.Second):
						t.Fatal("request returned before callback completion")
					}
					body := GetHTTPPacketBody(rsp.RawPacket)
					if noBuffer && len(body) != 0 {
						t.Fatalf("NoBodyBuffer returned %d body bytes", len(body))
					}
					if !noBuffer && !bytes.Equal(body, payload) {
						t.Fatalf("buffered body changed: got=%d want=%d", len(body), len(payload))
					}
					if rsp.Http2 != strings.HasPrefix(mode.proto, "HTTP/2") {
						t.Fatalf("actual protocol flag mismatch: %v", rsp.Http2)
					}
				})
			}
		})
	}
}

func TestPR5180_ErrorCallbackExactlyOnce(t *testing.T) {
	for _, proto := range []string{"HTTP/1.1", "HTTP/2"} {
		t.Run(proto, func(t *testing.T) {
			p := pr5180NewPeer(t, pr5180DropAfterRequest)
			addr := p.ln.Addr().String()
			if proto == "HTTP/1.1" {
				addr = pr5180OneShot(t, func(c net.Conn) {
					// Consume the entire request before closing without a response.
					request, err := http.ReadRequest(bufio.NewReader(c))
					if err == nil {
						_, _ = io.Copy(io.Discard, request.Body)
						_ = request.Body.Close()
					}
				})
			}
			var pool *LowHttpConnPool
			if proto == "HTTP/2" {
				pool = pr5180Pool(t, 2)
			}
			cfg := pr5180Config(t, addr, "POST", "/error-callback", proto, "body", pool)
			var calls atomic.Int32
			cfg.BodyStreamReaderHandler = func(_ []byte, r io.ReadCloser) {
				defer r.Close()
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r)
			}
			_, err := HTTPWithoutRetry(cfg)
			if err == nil {
				t.Fatal("expected a transport failure")
			}
			if calls.Load() != 1 {
				t.Fatalf("error callback invoked %d times", calls.Load())
			}
		})
	}
}

// H2 responses must use the same session cookie update order as H1 responses.
func TestPR5180_H2CookieRotationAndDeletion(t *testing.T) {
	got := make(chan string, 3)
	var requests atomic.Int32
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("pr5180_token")
		value := ""
		if cookie != nil {
			value = cookie.Value
		}
		got <- value
		switch requests.Add(1) {
		case 1:
			http.SetCookie(w, &http.Cookie{Name: "pr5180_token", Value: "new", Path: "/"})
		case 2:
			http.SetCookie(w, &http.Cookie{Name: "pr5180_token", Value: "", Path: "/", MaxAge: -1})
		}
		_, _ = io.WriteString(w, "ok")
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	pool := pr5180Pool(t, 2)
	session := fmt.Sprintf("pr5180-%s-%d", t.Name(), time.Now().UnixNano())
	t.Cleanup(func() { RemoveCookiejar(session) })
	for i := 0; i < 3; i++ {
		cfg := pr5180Config(t, s.Listener.Addr().String(), "GET", "/session", "HTTP/2", "", pool)
		cfg.Https = true
		cfg.DisableSession = false
		cfg.Session = session
		if i == 0 {
			cfg.Packet = bytes.Replace(cfg.Packet, []byte("Content-Length:"), []byte("Cookie: pr5180_token=old\r\nContent-Length:"), 1)
		}
		rsp, err := HTTPWithoutRetry(cfg)
		if err != nil || rsp == nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	for i, want := range []string{"old", "new", ""} {
		select {
		case value := <-got:
			if value != want {
				t.Fatalf("cookie at request %d=%q, want %q", i, value, want)
			}
		case <-time.After(time.Second):
			t.Fatal("missing recorded cookie")
		}
	}
}
