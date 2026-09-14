package lowhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netx"
)

func h2ResourceServer(t *testing.T, handler http.Handler) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	connections := new(atomic.Int32)
	s := httptest.NewUnstartedServer(handler)
	s.EnableHTTP2 = true
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, connections
}
func h2ResourceRequest(s *httptest.Server, p *LowHttpConnPool, opts ...LowhttpOpt) (*LowhttpResponse, error) {
	addr := s.Listener.Addr().(*net.TCPAddr)
	options := []LowhttpOpt{WithPacketBytes([]byte(fmt.Sprintf("GET / HTTP/2\r\nHost: %s\r\n\r\n", addr))), WithHttps(true), WithHttp2(true), WithHost("127.0.0.1"), WithPort(addr.Port), WithConnPool(true), ConnPool(p), WithTimeout(3 * time.Second)}
	return HTTPWithoutRedirect(append(options, opts...)...)
}

func TestH2PoolReuseBeyondConcurrentStreamLimit(t *testing.T) {
	s, conns := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	for i := 0; i < 250; i++ {
		if _, err := h2ResourceRequest(s, p); err != nil {
			t.Fatal(err)
		}
	}
	if conns.Load() != 1 {
		t.Fatalf("250 sequential requests opened %d connections", conns.Load())
	}
}

func TestH2PoolCoalescesConcurrentDials(t *testing.T) {
	s, conns := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h2ResourceRequest(s, p, WithDialer(func(timeout time.Duration, addr string) (net.Conn, error) {
				time.Sleep(20 * time.Millisecond)
				return net.DialTimeout("tcp", addr, timeout)
			}))
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if conns.Load() != 1 {
		t.Fatalf("concurrent cold requests opened %d connections", conns.Load())
	}
	p.h2Mu.Lock()
	pending := len(p.h2Dials)
	p.h2Mu.Unlock()
	if pending != 0 {
		t.Fatal("completed dial remained in registry")
	}
}

func TestH2PoolBoundsIdleOrigins(t *testing.T) {
	p := NewHttpConnPool(context.Background(), 2, 1)
	defer p.Clear()
	for i := 0; i < 5; i++ {
		s, _ := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
		if _, err := h2ResourceRequest(s, p); err != nil {
			t.Fatal(err)
		}
	}
	p.h2Mu.Lock()
	cached, idle := len(p.h2ConnMap), len(p.h2Idle)
	p.h2Mu.Unlock()
	if cached > 2 || idle > 2 {
		t.Fatalf("idle pool grew beyond cap: cached=%d idle=%d", cached, idle)
	}
	p.Clear()
	p.h2Mu.Lock()
	cached, idle = len(p.h2ConnMap), len(p.h2Idle)
	p.h2Mu.Unlock()
	if cached != 0 || idle != 0 {
		t.Fatal("Clear retained idle connection references")
	}
}

func TestH2StreamingSlowConsumerDoesNotBlockOtherStreams(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	s, conns := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			io.Copy(w, strings.NewReader(strings.Repeat("x", 4<<20)))
			return
		}
		io.WriteString(w, "fast")
	}))
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := h2ResourceRequest(s, p, WithPacketBytes([]byte("GET /slow HTTP/2\r\nHost: "+s.Listener.Addr().String()+"\r\n\r\n")), WithContext(ctx), WithNoBodyBuffer(true), WithBodyStreamReaderHandler(func(_ []byte, r io.ReadCloser) { close(started); <-release; io.Copy(io.Discard, r); r.Close() }))
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream callback not started")
	}
	fast, err := h2ResourceRequest(s, p)
	if err != nil || !strings.HasSuffix(string(fast.RawPacket), "fast") {
		t.Fatalf("other stream blocked: %v", err)
	}
	cancel()
	unblock()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream callback or request leaked")
	}
	if conns.Load() != 1 {
		t.Fatal("slow consumer forced a separate connection")
	}
}

func TestH2BodyPipeBoundAndClose(t *testing.T) {
	r, w := newH2BodyPipe()
	if _, err := w.Write(make([]byte, defaultStreamReceiveWindowSize)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte{1}); !errors.Is(err, errH2BodyWindowExceeded) {
		t.Fatalf("unbounded buffer: %v", err)
	}
	n, err := r.Read(make([]byte, 16384))
	if err != nil || n != 16384 {
		t.Fatal(n, err)
	}
	if _, err := w.Write(make([]byte, n)); err != nil {
		t.Fatal(err)
	}
	r.Close()
	w.Close()
	if r.p.buf.Cap() != 0 {
		t.Fatal("closed reader retained unread buffer")
	}
	if _, err := r.Read(make([]byte, 1)); err == nil {
		t.Fatal("closed reader did not unblock")
	}
}

func TestH2MaxContentLengthBoundsBuffer(t *testing.T) {
	s, _ := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", 1<<20)) }))
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	response, err := h2ResourceRequest(s, p, WithMaxContentLength(1024))
	if err != nil {
		t.Fatal(err)
	}
	_, body := SplitHTTPHeadersAndBodyFromPacket(response.RawPacket)
	if !response.TooLarge || len(body) > 1024 {
		t.Fatalf("body limit ignored: large=%t bytes=%d", response.TooLarge, len(body))
	}
}

func TestH2PoolClearCancelsPendingDial(t *testing.T) {
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	client, peer := net.Pipe()
	defer peer.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	result := make(chan error, 1)
	go func() {
		_, err := p.getOrCreateH2Conn(context.Background(), &connectKey{scheme: H2, addr: "127.0.0.1:12345"},
			netx.DialX_WithDisableProxy(true), netx.DialX_WithDialer(func(time.Duration, string) (net.Conn, error) { close(started); <-release; return client, nil }))
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial did not start")
	}
	p.Clear()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Clear did not cancel dial: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Clear stranded dial waiter")
	}
	unblock()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := peer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("late dial transport was not closed: %v", err)
	}
	p.h2Mu.Lock()
	cached, pending := len(p.h2ConnMap), len(p.h2Dials)
	p.h2Mu.Unlock()
	if cached != 0 || pending != 0 {
		t.Fatalf("Clear retained state: cached=%d pending=%d", cached, pending)
	}
}

func TestH2PoolDialWaiterCancellationIsIndependent(t *testing.T) {
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	client, peer := net.Pipe()
	defer peer.Close()
	drained := make(chan struct{})
	go func() { io.Copy(io.Discard, peer); close(drained) }()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	result := make(chan error, 1)
	key := &connectKey{scheme: H2, addr: "127.0.0.1:12345"}
	go func() {
		_, err := p.getOrCreateH2Conn(context.Background(), key, netx.DialX_WithDisableProxy(true), netx.DialX_WithDialer(func(time.Duration, string) (net.Conn, error) { close(started); <-release; return client, nil }))
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := p.getOrCreateH2Conn(ctx, &connectKey{scheme: H2, addr: key.addr})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting request did not time out: %v", err)
	}
	unblock()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("waiter canceled shared dial: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first dial stranded")
	}
	p.Clear()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("transport leaked")
	}
}
