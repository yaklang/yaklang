package lowhttp

// Regression peers for transport retry, downgrade, and response diagnostics.
// Every socket opened by these tests is restricted to the local loopback.

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

type pr5180PeerMode int

const (
	pr5180ReplyOK pr5180PeerMode = iota
	pr5180RefuseEveryStream
	pr5180WithholdSettings
	pr5180HoldFirstStream
	pr5180GoAwayRejectFirst
	pr5180CompleteThenGoAway
	pr5180InvalidResponsePseudoHeader
	pr5180SettingsAfterRequest
	pr5180DropAfterRequest
	pr5180GoAwayAcceptedNoResponse
	pr5180RefuseFirstStream
	pr5180ZeroLimitAfterFirst
)

type pr5180Seen struct {
	id uint32
	n  int32
}

type pr5180Peer struct {
	ln           net.Listener
	mode         pr5180PeerMode
	conns        atomic.Int32
	h2           atomic.Int32
	h1           atomic.Int32
	seen         chan pr5180Seen
	errors       chan error
	settingsAcks chan struct{}
	mu           sync.Mutex
	active       map[net.Conn]struct{}
	wg           sync.WaitGroup
	acceptDone   chan struct{}
}

func pr5180NewPeer(t *testing.T, mode pr5180PeerMode) *pr5180Peer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &pr5180Peer{
		ln: ln, mode: mode, seen: make(chan pr5180Seen, 64), errors: make(chan error, 8),
		active: make(map[net.Conn]struct{}), acceptDone: make(chan struct{}),
		settingsAcks: make(chan struct{}, 16),
	}
	go func() {
		defer close(p.acceptDone)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.conns.Add(1)
			p.mu.Lock()
			p.active[c] = struct{}{}
			p.mu.Unlock()
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				defer c.Close()
				defer func() { p.mu.Lock(); delete(p.active, c); p.mu.Unlock() }()
				p.serve(c)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-p.acceptDone // no further WaitGroup.Add calls after this point
		p.mu.Lock()
		for c := range p.active {
			_ = c.Close()
		}
		p.mu.Unlock()
		done := make(chan struct{})
		go func() { p.wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("PR5180 test peer failed to stop")
		}
		select {
		case err := <-p.errors:
			t.Errorf("PR5180 test fixture protocol error: %v", err)
		default:
		}
	})
	return p
}

func (p *pr5180Peer) fixtureError(err error) {
	select {
	case p.errors <- err:
	default:
	}
}

func (p *pr5180Peer) serve(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(c)
	preface, err := br.Peek(len(http2.ClientPreface))
	if err != nil {
		return
	}
	if string(preface) != http2.ClientPreface {
		r, err := http.ReadRequest(br)
		if err != nil {
			p.fixtureError(err)
			return
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 128<<10))
		_ = r.Body.Close()
		p.h1.Add(1)
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nfallback")
		return
	}
	_, _ = br.Discard(len(http2.ClientPreface))
	fr := http2.NewFramer(c, br)
	fr.SetMaxReadFrameSize(16384)
	if p.mode != pr5180WithholdSettings && p.mode != pr5180SettingsAfterRequest {
		if err := fr.WriteSettings(); err != nil {
			return
		}
	}
	completed := make(map[uint32]bool)
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			return
		} // EOF/deadline/reset are expected during cleanup
		var id uint32
		var end bool
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if f.IsAck() {
				select {
				case p.settingsAcks <- struct{}{}:
				default:
				}
			}
			if !f.IsAck() && p.mode != pr5180WithholdSettings && p.mode != pr5180SettingsAfterRequest {
				if err := fr.WriteSettingsAck(); err != nil {
					return
				}
			}
		case *http2.PingFrame:
			if !f.IsAck() && p.mode != pr5180WithholdSettings && p.mode != pr5180SettingsAfterRequest {
				if err := fr.WritePing(true, f.Data); err != nil {
					return
				}
			}
		case *http2.HeadersFrame:
			id, end = f.StreamID, f.StreamEnded()
		case *http2.DataFrame:
			id, end = f.StreamID, f.StreamEnded()
		}
		if !end || completed[id] {
			continue
		}
		completed[id] = true
		n := p.h2.Add(1)
		select {
		case p.seen <- pr5180Seen{id: id, n: n}:
		default:
		}
		switch p.mode {
		case pr5180WithholdSettings:
			// Record the complete request but deliberately send NO server preface.
			// The counter stands for a committed operation at this adversarial peer.
			continue
		case pr5180SettingsAfterRequest:
			// A client that waits for SETTINGS before HEADERS deadlocks here.
			if err := fr.WriteSettings(); err != nil {
				return
			}
			if err := fr.WriteSettingsAck(); err != nil {
				return
			}
		case pr5180DropAfterRequest:
			return
		case pr5180GoAwayAcceptedNoResponse:
			_ = fr.WriteGoAway(id, http2.ErrCodeNo, nil)
			return
		case pr5180RefuseFirstStream:
			if n == 1 {
				if err := fr.WriteRSTStream(id, http2.ErrCodeRefusedStream); err != nil {
					return
				}
				continue
			}
		case pr5180RefuseEveryStream:
			if err := fr.WriteRSTStream(id, http2.ErrCodeRefusedStream); err != nil {
				return
			}
			continue
		case pr5180HoldFirstStream:
			if n == 1 {
				continue
			}
		case pr5180GoAwayRejectFirst:
			if n == 1 {
				_ = fr.WriteGoAway(0, http2.ErrCodeNo, nil)
				return
			}
		case pr5180InvalidResponsePseudoHeader:
			// HPACK static index 2 is :method=GET, forbidden in a response.
			_ = fr.WriteHeaders(http2.HeadersFrameParam{StreamID: id, EndHeaders: true, EndStream: true, BlockFragment: []byte{0x82}})
			continue
		}
		// HPACK static index 8 is :status=200. No dynamic table is needed.
		if err := fr.WriteHeaders(http2.HeadersFrameParam{StreamID: id, EndHeaders: true, BlockFragment: []byte{0x88}}); err != nil {
			return
		}
		if err := fr.WriteData(id, true, []byte("ok")); err != nil {
			return
		}
		if p.mode == pr5180ZeroLimitAfterFirst && n == 1 {
			if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 0}); err != nil {
				return
			}
		}
		if p.mode == pr5180CompleteThenGoAway {
			_ = fr.WriteGoAway(id, http2.ErrCodeNo, nil)
			return
		}
	}
}

func pr5180Pool(t *testing.T, capacity int) *LowHttpConnPool {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := NewHttpConnPool(ctx, capacity, capacity)
	t.Cleanup(func() { p.Clear(); cancel() })
	return p
}

func pr5180Config(t *testing.T, addr, method, path, proto, body string, pool *LowHttpConnPool) *LowhttpExecConfig {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	packet := fmt.Sprintf("%s %s %s\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s", method, path, proto, addr, len(body), body)
	return &LowhttpExecConfig{
		Host: host, Port: port, Packet: []byte(packet),
		Http2: strings.HasPrefix(proto, "HTTP/2"),
		Ctx:   context.Background(), ConnectTimeout: time.Second, Timeout: 8 * time.Second,
		Dialer: func(timeout time.Duration, addr string) (net.Conn, error) {
			dialHost, _, err := net.SplitHostPort(addr)
			if err != nil || net.ParseIP(dialHost) == nil || !net.ParseIP(dialHost).IsLoopback() {
				return nil, fmt.Errorf("test blocked non-loopback dial to %q", addr)
			}
			return net.DialTimeout("tcp", addr, timeout)
		},
		OverrideEnableSystemProxyFromEnv: true, EnableSystemProxyFromEnv: false,
		DisableSession: true, SaveHTTPFlow: false,
		WithConnPool: pool != nil, ConnPool: pool,
	}
}

func pr5180WaitSeen(t *testing.T, p *pr5180Peer) pr5180Seen {
	t.Helper()
	select {
	case s := <-p.seen:
		return s
	case err := <-p.errors:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("peer did not receive a complete H2 request")
	}
	return pr5180Seen{}
}

// F02: The budget belongs to the entire logical request, not each nested layer.
func TestPR5180_H2RetryBudget(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			p := pr5180NewPeer(t, pr5180RefuseEveryStream)
			cfg := pr5180Config(t, p.ln.Addr().String(), method, "/budget", "HTTP/2", "x", pr5180Pool(t, 2))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg.Ctx = ctx
			_, err := HTTPWithoutRetry(cfg)
			var streamErr http2.StreamError
			if !errors.As(err, &streamErr) || streamErr.Code != http2.ErrCodeRefusedStream {
				t.Fatalf("want bounded REFUSED_STREAM failure, got %v", err)
			}
			if got := p.h2.Load(); got != 4 {
				t.Fatalf("logical request made %d stream attempts; reviewed default budget is initial+3=4, not 4*4", got)
			}
		})
	}
}

// F01: Silence does not establish that a POST/PATCH was not processed.
func TestPR5180_PrefaceTimeoutDoesNotReplayUnsafe(t *testing.T) {
	for _, method := range []string{"POST", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			p := pr5180NewPeer(t, pr5180WithholdSettings)
			cfg := pr5180Config(t, p.ln.Addr().String(), method, "/commit", "HTTP/2", "synthetic-operation", pr5180Pool(t, 2))
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			cfg.Ctx = ctx
			_, err := HTTPWithoutRetry(cfg)
			if got := p.h2.Load(); got != 1 {
				t.Fatalf("fixture expected one complete H2 request, got %d; err=%v", got, err)
			}
			if got := p.h1.Load(); got != 0 {
				t.Fatalf("unsafe %s replayed %d time(s) through H1 after H2 already received its body; err=%v", method, got, err)
			}
			if err == nil {
				t.Fatal("missing uncertain-outcome error after server preface timeout")
			}
		})
	}
}

func TestPR5180_UnsafePrefaceTimeoutUsesWireMethod(t *testing.T) {
	p := pr5180NewPeer(t, pr5180WithholdSettings)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/commit", "HTTP/2", "operation", pr5180Pool(t, 2))
	// A caller may provide an instance that does not describe its raw packet.
	// Replay safety must follow the method that H2 actually sends on the wire.
	cfg.NativeHTTPRequestInstance = &http.Request{Method: http.MethodGet, Header: make(http.Header)}
	_, err := HTTPWithoutRetry(cfg)
	if err == nil || p.h2.Load() != 1 || p.h1.Load() != 0 {
		t.Fatalf("POST was replayed using the native request's GET method: H2=%d H1=%d err=%v", p.h2.Load(), p.h1.Load(), err)
	}
}

func TestPR5180_IdempotentPrefaceTimeoutCanDowngrade(t *testing.T) {
	p := pr5180NewPeer(t, pr5180WithholdSettings)
	cfg := pr5180Config(t, p.ln.Addr().String(), "GET", "/safe", "HTTP/2", "", pr5180Pool(t, 2))
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil || rsp.Http2 {
		t.Fatalf("idempotent request did not fall back after preface timeout: rsp=%v err=%v", rsp, err)
	}
	if gotH2, gotH1 := p.h2.Load(), p.h1.Load(); gotH2 != 1 || gotH1 != 1 {
		t.Fatalf("unexpected replay counts: H2=%d H1=%d", gotH2, gotH1)
	}
}

// F03: A previously acquired H1 pool slot must not gate a direct H2 downgrade.
func TestPR5180_DowngradeDoesNotWaitForH1PoolSlot(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hold" {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(s.Close)
	t.Cleanup(unblock)
	pool := pr5180Pool(t, 1)
	first := pr5180Config(t, s.Listener.Addr().String(), "GET", "/hold", "HTTP/1.1", "", pool)
	first.Https = true
	first.Timeout = 12 * time.Second
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	first.Ctx = firstCtx
	firstDone := make(chan error, 1)
	go func() { _, err := HTTPWithoutRetry(first); firstDone <- err }()
	defer func() {
		unblock()
		firstCancel()
		select {
		case <-firstDone:
		case <-time.After(2 * time.Second):
			t.Error("held H1 request did not exit")
		}
	}()
	select {
	case <-entered:
	case err := <-firstDone:
		t.Fatalf("H1 holder exited early: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("H1 holder never entered")
	}
	second := pr5180Config(t, s.Listener.Addr().String(), "GET", "/target", "HTTP/2", "", pool)
	second.Https = true
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	second.Ctx = ctx
	rsp, err := HTTPWithoutRetry(second)
	if err != nil {
		t.Fatalf("H2->H1 downgrade waited on unrelated H1 pool slot: %v", err)
	}
	if rsp == nil || rsp.Http2 || !bytes.HasSuffix(rsp.RawPacket, []byte("ok")) {
		t.Fatalf("unexpected downgrade response: %#v", rsp)
	}
	if !second.WithConnPool {
		t.Fatal("implementation mutated caller's option rather than effective per-attempt state")
	}
}

func pr5180OneShot(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var mu sync.Mutex
	var active net.Conn
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		mu.Lock()
		active = c
		mu.Unlock()
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		serve(c)
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		if active != nil {
			_ = active.Close()
		}
		mu.Unlock()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("one-shot fixture did not stop")
		}
	})
	return ln.Addr().String()
}

// F04: Auto-detected pipeline must retain both parsed responses, not just bytes.
func TestPR5180_AutoPipelineMetadata(t *testing.T) {
	peerErr := make(chan error, 1)
	addr := pr5180OneShot(t, func(c net.Conn) {
		br := bufio.NewReader(c)
		for i := 0; i < 2; i++ {
			r, err := http.ReadRequest(br)
			if err != nil {
				peerErr <- err
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
		_, err := io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\nAHTTP/1.1 201 Created\r\nContent-Length: 1\r\n\r\nB")
		peerErr <- err
	})
	cfg := pr5180Config(t, addr, "POST", "/one", "HTTP/1.1", "", nil)
	cfg.Packet = []byte(fmt.Sprintf("POST /one HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\nGET /two HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr))
	cfg.NoFixContentLength = false // must be inferred from the prepared packet
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-peerErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("peer did not receive both pipelined requests")
	}
	if rsp == nil {
		t.Fatal("nil pipeline response")
	}
	if !rsp.MultiResponse || len(rsp.MultiResponseInstances) != 2 {
		t.Fatalf("lost auto-derived pipeline state: MultiResponse=%v instances=%d raw=%q", rsp.MultiResponse, len(rsp.MultiResponseInstances), rsp.RawPacket)
	}
	if rsp.MultiResponseInstances[0].StatusCode != 200 || rsp.MultiResponseInstances[1].StatusCode != 201 {
		t.Fatal("incorrect pipeline response association")
	}
}

// F05: Connected-but-no-response is distinct from an unreachable port.
func TestPR5180_DirectReadErrorRetainsDiagnostics(t *testing.T) {
	addr := pr5180OneShot(t, func(c net.Conn) {
		r, err := http.ReadRequest(bufio.NewReader(c))
		if err == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
		// Close only after consuming the request, without returning any response.
	})
	cfg := pr5180Config(t, addr, "GET", "/eof", "HTTP/1.1", "", nil)
	rsp, err := HTTPWithoutRetry(cfg)
	if err == nil {
		t.Fatal("expected a response-read failure")
	}
	if rsp == nil {
		t.Fatalf("discarded diagnostic response on established TCP connection: %v", err)
	}
	if !rsp.PortIsOpen || rsp.RemoteAddr == "" || len(rsp.RawRequest) == 0 || rsp.TraceInfo == nil || rsp.TraceInfo.DialTraceInfo == nil {
		t.Fatalf("missing partial-result diagnostics: open=%v remote=%q request-bytes=%d err=%v", rsp.PortIsOpen, rsp.RemoteAddr, len(rsp.RawRequest), err)
	}
	t.Run("dial_failure_never_marks_port_open", func(t *testing.T) {
		cfg := pr5180Config(t, addr, "GET", "/no-dial", "HTTP/1.1", "", nil)
		cfg.Dialer = func(time.Duration, string) (net.Conn, error) {
			return nil, errors.New("local synthetic dial failure")
		}
		failed, err := HTTPWithoutRetry(cfg)
		if err == nil || (failed != nil && failed.PortIsOpen) {
			t.Fatalf("dial failure reported an open port: rsp=%v err=%v", failed, err)
		}
	})
}

func TestPR5180_H2ReadErrorRetainsDiagnostics(t *testing.T) {
	p := pr5180NewPeer(t, pr5180DropAfterRequest)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/eof", "HTTP/2", "operation", pr5180Pool(t, 2))
	rsp, err := HTTPWithoutRetry(cfg)
	if err == nil || rsp == nil {
		t.Fatalf("connected H2 failure lost its partial response: rsp=%v err=%v", rsp, err)
	}
	if !rsp.PortIsOpen || rsp.RemoteAddr == "" || len(rsp.RawRequest) == 0 || rsp.TraceInfo == nil || rsp.TraceInfo.DialTraceInfo == nil || p.h2.Load() != 1 {
		t.Fatalf("H2 diagnostics missing or request replayed: rsp=%v H2=%d err=%v", rsp, p.h2.Load(), err)
	}
}

func TestPR5180_ALPNFallbackWorks(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	t.Cleanup(s.Close)
	cfg := pr5180Config(t, s.Listener.Addr().String(), "GET", "/", "HTTP/2", "", pr5180Pool(t, 2))
	cfg.Https = true
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rsp == nil || rsp.Http2 || !rsp.Https {
		t.Fatalf("incorrect ALPN fallback flags: %#v", rsp)
	}
	if method, _, proto := GetHTTPPacketFirstLine(rsp.RawRequest); method != "GET" || proto != "HTTP/1.1" {
		t.Fatalf("RawRequest does not describe the H1 packet sent after fallback: %q", rsp.RawRequest)
	}
}

func TestPR5180_H2SequentialReuse(t *testing.T) {
	p := pr5180NewPeer(t, pr5180ReplyOK)
	pool := pr5180Pool(t, 2)
	for i := 0; i < 16; i++ {
		cfg := pr5180Config(t, p.ln.Addr().String(), "GET", "/reuse", "HTTP/2", "", pool)
		rsp, err := HTTPWithoutRetry(cfg)
		if err != nil || rsp == nil || !rsp.Http2 {
			t.Fatalf("request %d: rsp=%v err=%v", i, rsp, err)
		}
	}
	if p.conns.Load() != 1 || p.h2.Load() != 16 {
		t.Fatalf("reuse lost: conns=%d requests=%d", p.conns.Load(), p.h2.Load())
	}
}

func TestPR5180_CancelOneStreamKeepsSibling(t *testing.T) {
	p := pr5180NewPeer(t, pr5180HoldFirstStream)
	pool := pr5180Pool(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := pr5180Config(t, p.ln.Addr().String(), "GET", "/hold", "HTTP/2", "", pool)
	cfg.Ctx = ctx
	done := make(chan error, 1)
	go func() { _, err := HTTPWithoutRetry(cfg); done <- err }()
	pr5180WaitSeen(t, p)
	other := pr5180Config(t, p.ln.Addr().String(), "GET", "/other", "HTTP/2", "", pool)
	rsp, err := HTTPWithoutRetry(other)
	cancel()
	if err != nil || rsp == nil {
		t.Fatalf("sibling failed while first stream was held: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled request did not exit")
	}
	third := pr5180Config(t, p.ln.Addr().String(), "GET", "/after-cancel", "HTTP/2", "", pool)
	if _, err := HTTPWithoutRetry(third); err != nil {
		t.Fatal(err)
	}
	if p.conns.Load() != 1 {
		t.Fatalf("stream cancellation killed shared connection; conns=%d", p.conns.Load())
	}
}

func TestPR5180_GoAwayRejectedStreamCanRetry(t *testing.T) {
	p := pr5180NewPeer(t, pr5180GoAwayRejectFirst)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/rejected", "HTTP/2", "synthetic", pr5180Pool(t, 2))
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil {
		t.Fatalf("explicitly rejected POST should be replayable: %v", err)
	}
	if p.h2.Load() != 2 || p.h1.Load() != 0 {
		t.Fatalf("unexpected retry counts H2=%d H1=%d", p.h2.Load(), p.h1.Load())
	}
}

func TestPR5180_CompleteResponseWinsGoAway(t *testing.T) {
	p := pr5180NewPeer(t, pr5180CompleteThenGoAway)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/complete", "HTTP/2", "synthetic", pr5180Pool(t, 2))
	rsp, err := HTTPWithoutRetry(cfg)
	if err != nil || rsp == nil {
		t.Fatalf("completed response lost to connection shutdown: %v", err)
	}
	if p.h2.Load() != 1 {
		t.Fatalf("completed request replayed %d times", p.h2.Load())
	}
}

func TestPR5180_MalformedResponseDoesNotDowngrade(t *testing.T) {
	p := pr5180NewPeer(t, pr5180InvalidResponsePseudoHeader)
	cfg := pr5180Config(t, p.ln.Addr().String(), "POST", "/malformed", "HTTP/2", "synthetic", pr5180Pool(t, 2))
	_, err := HTTPWithoutRetry(cfg)
	if err == nil {
		t.Fatal("invalid response pseudo-header accepted")
	}
	if p.h2.Load() != 1 || p.h1.Load() != 0 {
		t.Fatalf("protocol violation triggered replay H2=%d H1=%d; err=%v", p.h2.Load(), p.h1.Load(), err)
	}
}
