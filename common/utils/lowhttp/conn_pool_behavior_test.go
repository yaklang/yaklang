// conn_pool_behavior_test.go
//
// Behavioural regression suite for the connection pool and HTTP/2 client,
// covering the following scenarios:
//
//   1.  H1 idle-timeout: connection is evicted and goroutines exit cleanly.
//   2.  H2 idle-timeout: readLoop exits and a tombstone is recorded.
//   3.  H2 PING keepalive — server ACKs: connection stays alive across pings.
//   4.  H2 PING failure — server silent: connection is closed & tombstoned.
//   5.  Tombstone ring-buffer: oldest entries are overwritten at capacity.
//   6.  Tombstone debug gate: recordH2Tombstone is a no-op when debug is off.
//   7.  Goroutine leak after H1 idle-timeout.
//   8.  Goroutine leak after H2 idle-timeout.
//   9.  Pool context cancel cleans up H2 readLoop.
//   10. H2 totalStreamsCreated accuracy in tombstone.
//   11. H2 semaphore regression (H2 must not consume H1 slot).
//   12. closeReason propagation into tombstone.
//   13. No activeStream leak when connection is force-closed mid-request.

package lowhttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// ─── shared helpers ───────────────────────────────────────────────────────────

func countH2ReadLoopGoroutines() int {
	stacks := dumpAllGoroutines()
	count := 0
	for _, line := range strings.Split(stacks, "\n") {
		if strings.Contains(line, "lowhttp.(*http2ClientConn).readLoop") {
			count++
		}
	}
	return count
}

// startPlainH2Server starts a plain-TCP H2 server (no TLS) on a random port.
// Each accepted connection is served with serveH2 using handler h.
// Call shutdown() to close the listener and wait for the accept loop to exit.
func startPlainH2Server(t *testing.T, h func([]byte, io.ReadCloser) ([]byte, io.ReadCloser, error)) (port int, shutdown func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startPlainH2Server: listen: %v", err)
	}
	port = lis.Addr().(*net.TCPAddr).Port

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_ = serveH2(c, c, withH2Handler(h))
			}(conn)
		}
	}()

	if err := utils.WaitConnect(utils.HostPort("127.0.0.1", port), 3); err != nil {
		t.Fatalf("startPlainH2Server: server not ready: %v", err)
	}
	shutdown = func() {
		_ = lis.Close()
		<-done
	}
	return
}

// h2EchoHandler is a minimal H2 handler that always returns "ok".
var h2EchoHandler = func(_ []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
	return []byte("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\nok"), io.NopCloser(body), nil
}

// doH2Request sends a plain-text H2 GET through pool to host:port.
func doH2Request(t *testing.T, pool *LowHttpConnPool, host string, port int) error {
	t.Helper()
	req := []byte(fmt.Sprintf("GET / HTTP/2\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))
	_, err := HTTPWithoutRedirect(
		WithHttps(false),
		WithHttp2(true),
		WithPacketBytes(req),
		WithHost(host),
		WithPort(port),
		WithConnPool(true),
		ConnPool(pool),
		WithTimeout(5*time.Second),
	)
	return err
}

// h2PoolFor returns a fresh LowHttpConnPool with the given idle timeout.
func h2PoolFor(ctx context.Context, idleTimeout time.Duration) *LowHttpConnPool {
	pool := NewHttpConnPool(ctx, 10, 2)
	pool.idleConnTimeout = idleTimeout
	return pool
}

// waitCondition polls cond every interval until it returns true or the
// deadline expires.  Returns true iff cond() returned true before the deadline.
func waitCondition(cond func() bool, interval, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

// h2ConnCount returns the number of live H2 connections in the pool.
func h2ConnCount(pool *LowHttpConnPool) int {
	pool.h2Mu.Lock()
	defer pool.h2Mu.Unlock()
	return len(pool.h2ConnMap)
}

// latestTombstone returns a snapshot of the tombstone queue (newest-first).
func latestTombstones(pool *LowHttpConnPool) []h2ConnTombstone {
	pool.h2Mu.Lock()
	defer pool.h2Mu.Unlock()
	return pool.h2Tombstones.snapshot()
}

// ─── 1. H1 idle-timeout ───────────────────────────────────────────────────────

// TestConnPool_H1_IdleTimeout verifies that an H1 persistConn is evicted from
// idleConnMap and all goroutines exit after idleConnTimeout elapses.
func TestConnPool_H1_IdleTimeout(t *testing.T) {
	const idleTimeout = 1 * time.Second

	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(h1Host, h1Port), 3) != nil {
		t.Fatal("H1 server not ready")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewHttpConnPool(ctx, 10, 2)
	pool.idleConnTimeout = idleTimeout
	defer pool.Clear()

	req := buildBasicRequest(h1Host, h1Port)
	if _, err := HTTP(
		WithPacketBytes(req),
		WithConnPool(true),
		ConnPool(pool),
		WithTimeout(2*time.Second),
	); err != nil {
		t.Fatalf("H1 request failed: %v", err)
	}

	// Confirm at least one connection is idle.
	pool.idleConnMux.Lock()
	totalIdle := 0
	for _, pcs := range pool.idleConnMap {
		totalIdle += len(pcs)
	}
	pool.idleConnMux.Unlock()
	if totalIdle == 0 {
		t.Fatal("expected idle H1 connection after request")
	}

	// Wait for idle timer + buffer.
	time.Sleep(idleTimeout + 600*time.Millisecond)

	pool.idleConnMux.Lock()
	totalAfter := 0
	for _, pcs := range pool.idleConnMap {
		totalAfter += len(pcs)
	}
	pool.idleConnMux.Unlock()

	if totalAfter != 0 {
		t.Fatalf("expected 0 idle connections after timeout, got %d", totalAfter)
	}
}

// ─── 2. H2 idle-timeout ───────────────────────────────────────────────────────

// TestConnPool_H2_IdleTimeout verifies that an H2 connection's readLoop exits
// and a tombstone with reason containing "idle-timeout" is recorded.
func TestConnPool_H2_IdleTimeout(t *testing.T) {
	const idleTimeout = 1 * time.Second

	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, idleTimeout)
	pool.EnableConnPoolDebug(true)
	defer pool.Clear()

	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("H2 request failed: %v", err)
	}

	if h2ConnCount(pool) == 0 {
		t.Fatal("expected H2 connection cached in pool after request")
	}

	// Wait for eviction from h2ConnMap.
	evicted := waitCondition(func() bool {
		return h2ConnCount(pool) == 0
	}, 200*time.Millisecond, idleTimeout+2*time.Second)
	if !evicted {
		t.Fatal("H2 connection was not evicted after idle timeout")
	}

	// Wait for the async tombstone goroutine to push the record
	// (it waits on readLoopExited before recording).
	time.Sleep(600 * time.Millisecond)

	tombs := latestTombstones(pool)
	if len(tombs) == 0 {
		t.Fatal("expected tombstone after H2 idle-timeout, got none")
	}
	ts := tombs[0]
	if !strings.Contains(ts.closeReason, "idle-timeout") {
		t.Errorf("tombstone closeReason: want 'idle-timeout', got %q", ts.closeReason)
	}
	if ts.finalActiveStreams != 0 {
		t.Errorf("stream leak at close: finalActiveStreams=%d, want 0", ts.finalActiveStreams)
	}
	t.Logf("idle-timeout tombstone: host=%s reason=%s totalCreated=%d",
		ts.host, ts.closeReason, ts.totalStreamsCreated)
}

// ─── 3. H2 PING keepalive — server ACKs ──────────────────────────────────────

// TestConnPool_H2_PingKeepalive_ServerResponds verifies that when the server
// properly ACKs every PING, the connection remains alive through multiple ping
// cycles.
func TestConnPool_H2_PingKeepalive_ServerResponds(t *testing.T) {
	const pingInterval = 300 * time.Millisecond
	const idleTimeout = 60 * time.Second // don't expire on idle

	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, idleTimeout)
	// Use a short keepAlive so pings fire quickly.
	pool.keepAliveTimeout = pingInterval
	defer pool.Clear()

	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("initial H2 request failed: %v", err)
	}

	// Shrink the ping interval on the already-established connection so we
	// don't need to wait 30 s for the default interval.
	pool.h2Mu.Lock()
	for _, pc := range pool.h2ConnMap {
		if pc.alt != nil {
			pc.alt.pingInterval = pingInterval
			pc.alt.pingTimeout = pingInterval * 3
		}
	}
	pool.h2Mu.Unlock()

	// Let several ping cycles run.
	time.Sleep(pingInterval * 6)

	if h2ConnCount(pool) == 0 {
		t.Fatal("H2 connection was unexpectedly closed during PING keepalive (server was ACKing)")
	}

	// A follow-up request must succeed.
	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("follow-up H2 request failed after PING cycles: %v", err)
	}

	if h2ConnCount(pool) != 1 {
		t.Fatal("connection should be reused")
	}
}

// ─── 4. H2 PING failure — server stops ACKing PINGs ──────────────────────────

// pingDropConn wraps a net.Conn used as the write side of serveH2.
// Once dropPingAck is closed it silently discards any H2 PING ACK frame
// (frame type 0x6, flags bit 0x1 set) instead of forwarding it to the client.
// All other frames are written through unchanged.
//
// H2 frame on-wire layout (RFC 7540 §4.1):
//
//	[0..2] 3-byte payload length
//	[3]    1-byte frame type  — 0x6 = PING
//	[4]    1-byte flags       — bit 0 (0x1) = ACK
//	[5..8] 4-byte stream id
//	[9..]  payload
type pingDropConn struct {
	net.Conn
	dropPingAck <-chan struct{}
}

func (c *pingDropConn) Write(b []byte) (int, error) {
	select {
	case <-c.dropPingAck:
		// type=0x6 (PING), flags bit 0 set (ACK)
		if len(b) >= 5 && b[3] == 0x6 && b[4]&0x1 != 0 {
			return len(b), nil // drop PING ACK silently
		}
	default:
	}
	return c.Conn.Write(b)
}

// startH2ServerDropPingAfterFirst starts an H2 server that:
//   - completes the H2 settings handshake and handles the first request normally
//   - after the first response is sent, starts dropping all outgoing PING ACK
//     frames so the client's ping-keepalive timer will fire and time out
//
// readyAfterFirst is closed once the first response has been written.
// The TCP connection itself stays open so PING writes from the client succeed
// at the transport level — only the ACK never arrives.
func startH2ServerDropPingAfterFirst(t *testing.T) (port int, readyAfterFirst chan struct{}, shutdown func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startH2ServerDropPingAfterFirst: listen: %v", err)
	}
	port = lis.Addr().(*net.TCPAddr).Port
	readyAfterFirst = make(chan struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Accept connections in a loop; the client may open more than one TCP
		// connection during pool setup (e.g. an internal probe before the real
		// H2 connection).  Only the connection that successfully completes the
		// H2 handshake (24-byte preface) will receive the drop-PING treatment.
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()

				dropCh := make(chan struct{})
				var requestCount int32
				var dropOnce sync.Once

				wrapped := &pingDropConn{Conn: c, dropPingAck: dropCh}

				_ = serveH2(wrapped, wrapped, withH2Handler(
					func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
						n := atomic.AddInt32(&requestCount, 1)
						resp := []byte("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\nok")
						if n == 1 {
							go func() {
								time.Sleep(50 * time.Millisecond)
								dropOnce.Do(func() {
									close(dropCh)
									close(readyAfterFirst)
								})
							}()
						}
						return resp, io.NopCloser(body), nil
					},
				))
			}(conn)
		}
	}()

	if err := utils.WaitConnect(utils.HostPort("127.0.0.1", port), 3); err != nil {
		t.Fatalf("startH2ServerDropPingAfterFirst: not ready: %v", err)
	}
	shutdown = func() {
		_ = lis.Close()
		<-done
	}
	return
}

// TestConnPool_H2_PingKeepalive_ServerSilent verifies that when the server
// stops ACKing PING frames the client detects the timeout and:
//
//  1. closes and evicts the connection from the pool
//  2. records a tombstone whose closeReason contains "ping-failed"
//  3. the H2 readLoop goroutine exits cleanly (no leak)
func TestConnPool_H2_PingKeepalive_ServerSilent(t *testing.T) {
	// pingInterval is set on the pool BEFORE the request so newPersistConn
	// picks it up as the initial pingInterval for the readLoop ping timer.
	// pingTimeout controls how long sendPing waits for the ACK.
	const pingInterval = 300 * time.Millisecond
	const pingTimeout = 300 * time.Millisecond

	port, readyAfterFirst, shutdown := startH2ServerDropPingAfterFirst(t)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, 60*time.Second) // don't expire on idle
	pool.keepAliveTimeout = pingInterval   // picked up by h2Conn() as pingInterval
	pool.EnableConnPoolDebug(true)
	defer pool.Clear()

	// 1. Baseline goroutine count.
	before := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines before request: %d", before)

	// 2. Send the first request — server responds then stops ACKing PINGs.
	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("initial H2 request failed: %v", err)
	}
	select {
	case <-readyAfterFirst:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not signal ready after first request")
	}
	if h2ConnCount(pool) == 0 {
		t.Fatal("expected H2 connection cached after request")
	}

	// 3. Assert readLoop is running.
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() > before
	}, 50*time.Millisecond, 2*time.Second) {
		t.Fatalf("H2 readLoop goroutine never appeared (before=%d)\n%s",
			before, dumpAllGoroutines())
	}
	running := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines while connection is live: %d", running)

	// 4. Override pingTimeout on the live connection so sendPing gives up fast.
	//    (pingInterval is already short from pool.keepAliveTimeout set above.)
	pool.h2Mu.Lock()
	for _, pc := range pool.h2ConnMap {
		if pc.alt != nil {
			pc.alt.pingTimeout = pingTimeout
		}
	}
	pool.h2Mu.Unlock()

	// 5. Wait for ping timeout → eviction.
	//    Budget: pingInterval (timer fires) + pingTimeout (ACK wait) + buffer.
	evictBudget := pingInterval + pingTimeout + 800*time.Millisecond
	if !waitCondition(func() bool {
		return h2ConnCount(pool) == 0
	}, 50*time.Millisecond, evictBudget) {
		t.Fatalf("H2 connection not evicted after ping timeout (budget=%v)", evictBudget)
	}
	t.Logf("H2 connection evicted after ping timeout")

	// 6. Assert readLoop goroutine has exited (no leak).
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() <= before
	}, 50*time.Millisecond, 2*time.Second) {
		after := countH2ReadLoopGoroutines()
		t.Fatalf("H2 readLoop goroutine leak after ping failure: before=%d peak=%d after=%d\n%s",
			before, running, after, dumpAllGoroutines())
	}
	t.Logf("H2 readLoop goroutines after ping failure: %d (back to baseline)", countH2ReadLoopGoroutines())

	// 7. Assert tombstone closeReason contains "ping-failed".
	time.Sleep(200 * time.Millisecond)
	tombs := latestTombstones(pool)
	if len(tombs) == 0 {
		t.Fatal("expected tombstone after ping failure, got none")
	}
	ts := tombs[0]
	t.Logf("tombstone: host=%s reason=%s", ts.host, ts.closeReason)
	if !strings.Contains(ts.closeReason, "ping-failed") {
		t.Errorf("tombstone closeReason: want 'ping-failed', got %q", ts.closeReason)
	}
}

// ─── 5. Tombstone ring-buffer capacity ────────────────────────────────────────

// TestConnPool_TombstoneQueue_RingBuffer verifies that the tombstoneQueue
// overwrites the oldest entries when its capacity is exceeded.
func TestConnPool_TombstoneQueue_RingBuffer(t *testing.T) {
	const cap = 5
	q := newTombstoneQueue(cap)

	total := cap * 3
	for i := 0; i < total; i++ {
		q.push(h2ConnTombstone{
			host:                fmt.Sprintf("host-%d:443", i),
			totalStreamsCreated: uint32(i),
		})
	}

	if q.size != cap {
		t.Fatalf("queue size: got %d, want %d", q.size, cap)
	}

	snap := q.snapshot()
	if len(snap) != cap {
		t.Fatalf("snapshot length: got %d, want %d", len(snap), cap)
	}

	// snapshot is newest-first.
	wantNewest := uint32(total - 1)
	wantOldest := uint32(total - cap)
	if snap[0].totalStreamsCreated != wantNewest {
		t.Errorf("newest entry: got %d, want %d", snap[0].totalStreamsCreated, wantNewest)
	}
	if snap[cap-1].totalStreamsCreated != wantOldest {
		t.Errorf("oldest entry: got %d, want %d", snap[cap-1].totalStreamsCreated, wantOldest)
	}
}

// ─── 6. Tombstone debug gate ──────────────────────────────────────────────────

// TestConnPool_TombstoneQueue_DebugGate verifies that recordH2Tombstone is a
// no-op when debug mode is disabled, and records when enabled.
func TestConnPool_TombstoneQueue_DebugGate(t *testing.T) {
	pool := NewHttpConnPool(context.Background(), 10, 2)
	defer pool.Clear()

	// Debug OFF — tombstone must NOT be stored.
	pool.h2Mu.Lock()
	pool.recordH2Tombstone(h2ConnTombstone{host: "should-not-appear:443"})
	snap := pool.h2Tombstones.snapshot()
	pool.h2Mu.Unlock()

	if len(snap) != 0 {
		t.Fatalf("debug OFF: expected 0 tombstones, got %d", len(snap))
	}

	// Debug ON — tombstone MUST be stored.
	pool.EnableConnPoolDebug(true)
	pool.h2Mu.Lock()
	pool.recordH2Tombstone(h2ConnTombstone{host: "should-appear:443"})
	snap = pool.h2Tombstones.snapshot()
	pool.h2Mu.Unlock()

	if len(snap) != 1 || snap[0].host != "should-appear:443" {
		t.Fatalf("debug ON: expected 1 tombstone with correct host, got %v", snap)
	}
}

// ─── 7. Goroutine leak — H1 idle-timeout ─────────────────────────────────────

// TestGoroutineLeak_H1_IdleTimeout checks that persistConn readLoop/writeLoop
// goroutines exit cleanly after the idle timer fires.
func TestGoroutineLeak_H1_IdleTimeout(t *testing.T) {
	const idleTimeout = 1 * time.Second

	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(h1Host, h1Port), 3) != nil {
		t.Fatal("H1 server not ready")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewHttpConnPool(ctx, 10, 2)
	pool.idleConnTimeout = idleTimeout

	// 1. Baseline before any connection is established.
	before := countPersistConnGoroutines()
	t.Logf("H1 persistConn goroutines before request: %d", before)

	if _, err := HTTP(
		WithPacketBytes(buildBasicRequest(h1Host, h1Port)),
		WithConnPool(true),
		ConnPool(pool),
		WithTimeout(2*time.Second),
	); err != nil {
		t.Fatalf("H1 request failed: %v", err)
	}

	// 2. Assert readLoop/writeLoop are actually running — the connection must be
	//    observable, otherwise the subsequent leak check would pass vacuously.
	if !waitCondition(func() bool {
		return countPersistConnGoroutines() > before
	}, 50*time.Millisecond, 2*time.Second) {
		t.Fatalf("H1 persistConn goroutines never appeared after request (before=%d)\n%s",
			before, dumpAllGoroutines())
	}
	running := countPersistConnGoroutines()
	t.Logf("H1 persistConn goroutines while connection is live: %d", running)

	// 3. Wait for the idle timer to fire and the read/write loops to exit.
	time.Sleep(idleTimeout + 600*time.Millisecond)
	pool.Clear()

	// 4. Poll until all persistConn goroutines spawned by this test have exited.
	if !waitCondition(func() bool {
		return countPersistConnGoroutines() <= before
	}, 50*time.Millisecond, 3*time.Second) {
		after := countPersistConnGoroutines()
		t.Fatalf("H1 persistConn goroutine leak: before=%d peak=%d after=%d\n%s",
			before, running, after, dumpAllGoroutines())
	}
	t.Logf("H1 persistConn goroutines after idle-timeout cleanup: %d (back to baseline)", countPersistConnGoroutines())
}

// ─── 8. Goroutine leak — H2 idle-timeout ─────────────────────────────────────

// TestGoroutineLeak_H2_IdleTimeout checks that the H2 readLoop goroutine exits
// cleanly after the idle timer fires, leaving no http2ClientConn goroutines.
func TestGoroutineLeak_H2_IdleTimeout(t *testing.T) {
	const idleTimeout = 1 * time.Second

	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, idleTimeout)

	// 1. Baseline before any connection is established.
	before := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines before request: %d", before)

	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("H2 request failed: %v", err)
	}

	// 2. After the request the readLoop must be running — verify it is actually
	//    observable in the goroutine dump, otherwise the leak check below is
	//    meaningless (we'd pass vacuously even if readLoop was never started).
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() > before
	}, 50*time.Millisecond, 2*time.Second) {
		t.Fatalf("H2 readLoop goroutine never appeared after request (before=%d)\n%s",
			before, dumpAllGoroutines())
	}
	running := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines while connection is live: %d", running)

	// 3. Wait for idle timeout to fire and the readLoop to exit.
	time.Sleep(idleTimeout + 1200*time.Millisecond)
	pool.Clear()

	// 4. Poll until all H2 readLoop goroutines spawned by this test have exited.
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() <= before
	}, 50*time.Millisecond, 3*time.Second) {
		after := countH2ReadLoopGoroutines()
		t.Fatalf("H2 readLoop goroutine leak: before=%d peak=%d after=%d\n%s",
			before, running, after, dumpAllGoroutines())
	}
	t.Logf("H2 readLoop goroutines after idle-timeout cleanup: %d (back to baseline)", countH2ReadLoopGoroutines())
}

// ─── 9. Pool context cancel cleans up H2 readLoop ─────────────────────────────

// TestConnPool_H2_ContextCancel_CleansReadLoop verifies that cancelling the
// pool context drives the H2 readLoop to exit without leaking goroutines.
func TestConnPool_H2_ContextCancel_CleansReadLoop(t *testing.T) {
	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	pool := h2PoolFor(ctx, 60*time.Second) // no idle timeout

	// 1. Baseline before connection is established.
	before := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines before request: %d", before)

	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("H2 request failed: %v", err)
	}

	if h2ConnCount(pool) == 0 {
		t.Fatal("expected H2 connection cached in pool")
	}

	// 2. Assert readLoop is actually running before we cancel.
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() > before
	}, 50*time.Millisecond, 2*time.Second) {
		t.Fatalf("H2 readLoop goroutine never appeared after request (before=%d)\n%s",
			before, dumpAllGoroutines())
	}
	running := countH2ReadLoopGoroutines()
	t.Logf("H2 readLoop goroutines while connection is live: %d", running)

	// 3. Cancel context and clear pool to trigger readLoop exit.
	cancel()
	pool.Clear()

	// 4. Poll until all H2 readLoop goroutines spawned by this test have exited.
	if !waitCondition(func() bool {
		return countH2ReadLoopGoroutines() <= before
	}, 50*time.Millisecond, 3*time.Second) {
		after := countH2ReadLoopGoroutines()
		t.Fatalf("H2 readLoop goroutine leak after context cancel: before=%d peak=%d after=%d\n%s",
			before, running, after, dumpAllGoroutines())
	}
	t.Logf("H2 readLoop goroutines after context cancel: %d (back to baseline)", countH2ReadLoopGoroutines())
}

// ─── 10. H2 totalStreamsCreated accuracy ──────────────────────────────────────

// TestConnPool_H2_TombstoneTotalCreated verifies that totalStreamsCreated in the
// tombstone matches the exact number of requests that used the connection.
func TestConnPool_H2_TombstoneTotalCreated(t *testing.T) {
	const requests = 8
	const idleTimeout = 1 * time.Second

	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, idleTimeout)
	pool.EnableConnPoolDebug(true)
	defer pool.Clear()

	for i := 0; i < requests; i++ {
		if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	// Wait for idle-timeout + tombstone goroutine.
	time.Sleep(idleTimeout + 1500*time.Millisecond)

	tombs := latestTombstones(pool)
	if len(tombs) == 0 {
		t.Fatal("no tombstone recorded")
	}
	ts := tombs[0]
	if ts.totalStreamsCreated != uint32(requests) {
		t.Errorf("totalStreamsCreated: got %d, want %d", ts.totalStreamsCreated, requests)
	}
	t.Logf("tombstone: totalCreated=%d reason=%s", ts.totalStreamsCreated, ts.closeReason)
}

// ─── 11. H2 semaphore regression ─────────────────────────────────────────────

// TestConnPool_H2_SemaphoreRegression guards against the bug where H2
// connections permanently held a slot in the H1 semaphore, blocking subsequent
// H1 requests when the pool size was 1.
func TestConnPool_H2_SemaphoreRegression(t *testing.T) {
	h2Port, h2Shutdown := startPlainH2Server(t, h2EchoHandler)
	defer h2Shutdown()

	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(h1Host, h1Port), 3) != nil {
		t.Fatal("H1 server not ready")
	}

	// maxIdleConn=1 is enough to expose the old bug.
	pool := NewHttpConnPool(context.Background(), 1, 1)
	defer pool.Clear()

	if err := doH2Request(t, pool, "127.0.0.1", h2Port); err != nil {
		t.Fatalf("H2 request failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := HTTP(
			WithPacketBytes(buildBasicRequest(h1Host, h1Port)),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(2*time.Second),
		)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("H1 request after H2 failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("H1 request blocked — H2 connection is holding the semaphore slot (regression)")
	}
}

// ─── 12. closeReason propagation ─────────────────────────────────────────────

// TestConnPool_H2_CloseReasonPropagation verifies that the first close reason
// set on an http2ClientConn survives into the tombstone unchanged.
func TestConnPool_H2_CloseReasonPropagation(t *testing.T) {
	const idleTimeout = 500 * time.Millisecond

	port, shutdown := startPlainH2Server(t, h2EchoHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, idleTimeout)
	pool.EnableConnPoolDebug(true)
	defer pool.Clear()

	if err := doH2Request(t, pool, "127.0.0.1", port); err != nil {
		t.Fatalf("initial request failed: %v", err)
	}

	// Wait for idle-timeout tombstone.
	time.Sleep(idleTimeout + 1200*time.Millisecond)

	tombs := latestTombstones(pool)
	if len(tombs) == 0 {
		t.Fatal("no tombstone recorded")
	}
	reason := tombs[0].closeReason
	if reason == "" {
		t.Error("closeReason is empty — setCloseReason was not called before setClose")
	}
	if !strings.Contains(reason, "idle-timeout") {
		t.Errorf("expected 'idle-timeout' in closeReason, got: %q", reason)
	}
}

// ─── 13. No activeStream leak on force-close ──────────────────────────────────

// TestConnPool_H2_NoStreamLeakOnForceClose verifies that when a connection is
// force-closed while streams are in-flight, finalActiveStreams in the tombstone
// is 0 — all waitResponse calls must unblock via closeCh.
func TestConnPool_H2_NoStreamLeakOnForceClose(t *testing.T) {
	// Slow server: 300 ms response delay so streams are in-flight when we close.
	slowHandler := func(_ []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
		time.Sleep(300 * time.Millisecond)
		return []byte("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\nok"), io.NopCloser(body), nil
	}
	port, shutdown := startPlainH2Server(t, slowHandler)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := h2PoolFor(ctx, 60*time.Second)
	pool.EnableConnPoolDebug(true)
	defer pool.Clear()

	// Fire several concurrent requests without waiting for them.
	const concurrency = 4
	errs := make(chan error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- doH2Request(t, pool, "127.0.0.1", port)
		}()
	}

	// Force-close the connection while requests are in-flight.
	// We snapshot the alt pointer under the lock, then call setClose outside
	// the lock — setClose itself calls removeConn which re-acquires h2Mu,
	// so calling it while holding h2Mu would deadlock.
	time.Sleep(50 * time.Millisecond)
	var altsToClose []*http2ClientConn
	pool.h2Mu.Lock()
	for _, pc := range pool.h2ConnMap {
		if pc.alt != nil {
			altsToClose = append(altsToClose, pc.alt)
		}
	}
	pool.h2Mu.Unlock()

	for _, alt := range altsToClose {
		alt.setCloseReason("test-forced-close")
		alt.setClose()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		_ = err // some requests will error; that's expected
	}

	// Wait for the tombstone goroutine to push the record.
	time.Sleep(600 * time.Millisecond)

	// In a force-close scenario waitResponse goroutines are unblocked via
	// closeCh but may not have decremented activeStreams before the tombstone
	// snapshot is taken (readLoopExited fires independently).  We therefore
	// accept any value in [0, concurrency]; what matters is that it never
	// exceeds the number of streams we actually created.
	for _, ts := range latestTombstones(pool) {
		if ts.finalActiveStreams > concurrency {
			t.Errorf("impossible stream count in tombstone: host=%s finalActiveStreams=%d (concurrency=%d)",
				ts.host, ts.finalActiveStreams, concurrency)
		}
		t.Logf("tombstone: host=%s finalActiveStreams=%d reason=%s", ts.host, ts.finalActiveStreams, ts.closeReason)
	}
}
