package lowhttp

import (
	"bufio"
	"container/list"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// ── H2ConnPool: standalone H2 connection pool ───────────────────────────────

// H2ConnPool manages HTTP/2 connections independently from the H1 connection
// pool.  A single H2 TCP connection multiplexes all streams for a given host,
// so the pool stores at most one *h2ConnEntry per connectKey hash and throttles
// concurrency inside http2ClientConn.newStream via SETTINGS_MAX_CONCURRENT_STREAMS.
//
// Lock order: h2Mu → http2ClientConn.mu (never the reverse).
type H2ConnPool struct {
	ctx              context.Context
	idleTimeout      time.Duration
	keepAliveTimeout time.Duration
	maxIdleConn      int

	mu          sync.Mutex
	connMap     map[string]*h2ConnEntry // per-host cached H2 connection
	idle        map[*h2ConnEntry]*list.Element
	idleLRU     *list.List
	dials       map[string]*h2DialCall
	generation  uint64
	tombstones  *tombstoneQueue
	debugEnabled int32 // atomic; mirrors LowHttpConnPool.debugEnabled
}

// h2ConnEntry is the H2 equivalent of persistConn: it wraps a single TCP/TLS
// connection that speaks HTTP/2 and owns the http2ClientConn state machine.
type h2ConnEntry struct {
	conn     net.Conn       // underlying TCP/TLS connection
	cacheKey *connectKey    // pool cache key (scheme == H2)
	alt      *http2ClientConn // H2 protocol state; nil after close

	pool *H2ConnPool // back-reference for eviction
}

// NewH2ConnPool creates a standalone H2 connection pool.
func NewH2ConnPool(ctx context.Context, maxIdleConn int, idleTimeout, keepAliveTimeout time.Duration) *H2ConnPool {
	return &H2ConnPool{
		ctx:              ctx,
		idleTimeout:      idleTimeout,
		keepAliveTimeout: keepAliveTimeout,
		maxIdleConn:      maxIdleConn,
		connMap:          make(map[string]*h2ConnEntry),
		tombstones:       newTombstoneQueue(defaultTombstoneQueueSize),
	}
}

func (p *H2ConnPool) contextDone() bool {
	if p == nil || p.ctx == nil {
		return true
	}
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// connUsable returns true when the connection can accept new streams.
func (p *H2ConnPool) connUsable(e *h2ConnEntry) bool {
	if e == nil || e.alt == nil {
		return false
	}
	e.alt.mu.Lock()
	defer e.alt.mu.Unlock()
	return !e.alt.readGoAway && !e.alt.closed && !e.alt.full
}

// GetOrCreate retrieves an existing usable H2 connection for the given key,
// or dials a new one.  Concurrent dials for the same host are coalesced.
func (p *H2ConnPool) GetOrCreate(ctx context.Context, key *connectKey, opts []netx.DialXOption) (*h2ConnEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := requestContextError(ctx); err != nil {
		return nil, err
	}
	hash := key.hash()

retryH2Dial:
	if err := requestContextError(ctx); err != nil {
		return nil, err
	}
	if p.contextDone() {
		return nil, utils.Error("lowhttp: h2 pool context done")
	}

	// Fast path: reuse an existing, healthy H2 connection.
	p.mu.Lock()
	if e, ok := p.connMap[hash]; ok {
		if p.connUsable(e) {
			p.mu.Unlock()
			if err := requestContextError(ctx); err != nil {
				return nil, err
			}
			return e, nil
		}
		// Existing connection is no longer usable; evict it.
		delete(p.connMap, hash)
		if elem := p.idle[e]; elem != nil {
			p.idleLRU.Remove(elem)
			delete(p.idle, e)
		}
	}
	if call := p.dials[hash]; call != nil {
		p.mu.Unlock()
		select {
		case <-call.done:
			goto retryH2Dial
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		}
	}
	if p.dials == nil {
		p.dials = make(map[string]*h2DialCall)
	}
	dialCtx, cancelDial := context.WithCancel(ctx)
	call := &h2DialCall{done: make(chan struct{}), cancel: cancelDial}
	p.dials[hash] = call
	generation := p.generation
	p.mu.Unlock()

	stopPoolCancel := context.AfterFunc(p.ctx, cancelDial)
	defer func() {
		stopPoolCancel()
		cancelDial()
		p.mu.Lock()
		delete(p.dials, hash)
		close(call.done)
		p.mu.Unlock()
	}()

	entry, err := p.dialH2Conn(dialCtx, key, opts)
	if err != nil {
		if ctxErr := requestContextError(ctx); ctxErr != nil {
			return nil, ctxErr
		}
		// Another goroutine may have concurrently established a usable connection.
		p.mu.Lock()
		if e, ok := p.connMap[hash]; ok {
			if p.connUsable(e) {
				p.mu.Unlock()
				if ctxErr := requestContextError(ctx); ctxErr != nil {
					return nil, ctxErr
				}
				return e, nil
			}
		}
		p.mu.Unlock()
		return nil, err
	}

	// Downgrade: ALPN negotiation or preface failure changed key.scheme to H1.
	// The caller (exec.go) detects the scheme change and switches to H1 path.
	if key.scheme != H2 {
		if err := requestContextError(ctx); err != nil {
			entry.conn.Close()
			return nil, err
		}
		return entry, nil
	}

	// Store the new H2 connection. Handle the race where another goroutine
	// already stored a usable conn while we were dialing.
	p.mu.Lock()
	if existing, ok := p.connMap[hash]; ok {
		if p.connUsable(existing) {
			p.mu.Unlock()
			entry.conn.Close()
			if err := requestContextError(ctx); err != nil {
				return nil, err
			}
			return existing, nil
		}
	}
	if err := requestContextError(ctx); err != nil {
		p.mu.Unlock()
		entry.conn.Close()
		return nil, err
	}
	if p.generation != generation || p.contextDone() || entry.alt.isClosed() {
		p.mu.Unlock()
		entry.conn.Close()
		return nil, errH2ConnClosed
	}
	p.connMap[hash] = entry
	p.mu.Unlock()
	p.markIdle(entry)
	return entry, nil
}

// dialH2Conn dials a new TCP/TLS connection and initializes the H2 state machine.
// If ALPN fails to negotiate h2, key.scheme is changed to H1 and the entry is
// returned without an alt (downgrade signal).
func (p *H2ConnPool) dialH2Conn(ctx context.Context, key *connectKey, opts []netx.DialXOption) (*h2ConnEntry, error) {
	needProxy := len(key.proxy) > 0
	opts = append(opts, netx.DialX_WithKeepAlive(p.keepAliveTimeout))
	newConn, err := dialXWithContext(ctx, key.addr, opts...)
	if err != nil {
		return nil, err
	}

	entry := &h2ConnEntry{
		conn:     newConn,
		cacheKey: key,
		pool:     p,
	}
	_ = needProxy // proxy is encoded in key.proxy; nothing extra to do here

	if key.https { // ALPN negotiation
		switch conn := newConn.(type) {
		case *tls.Conn:
			key.scheme = conn.ConnectionState().NegotiatedProtocol
		case *utls.UConn:
			key.scheme = conn.ConnectionState().NegotiatedProtocol
		case *gmtls.Conn:
			key.scheme = conn.ConnectionState().NegotiatedProtocol
		}
		if key.scheme != H2 { // downgrade: ALPN did not select h2
			key.scheme = H1
			return entry, nil
		}
	}

	// Initialize H2 state machine
	p.initH2Conn(entry)

	entry.alt.writeCtx = ctx
	err = entry.alt.preface()
	entry.alt.writeCtx = nil
	if err == nil {
		go entry.alt.readLoop()
		entry.alt.watchServerPreface()
		return entry, nil
	}

	// Client preface failure: tear down and downgrade to H1
	if entry.alt != nil {
		entry.alt.setClose()
	}
	entry.alt = nil
	if err := requestContextError(ctx); err != nil {
		return nil, err
	}
	key.scheme = H1
	if entry.conn != nil {
		entry.conn.Close()
	}
	newH1Conn, err := dialXWithContext(ctx, key.addr, append(opts, netx.DialX_WithTLSNextProto(H1))...)
	if err != nil {
		return nil, err
	}
	entry.conn = newH1Conn
	return entry, nil
}

// initH2Conn creates the http2ClientConn and wires it to the entry.
func (p *H2ConnPool) initH2Conn(entry *h2ConnEntry) {
	var profile *http2Profile
	if entry.cacheKey != nil && entry.cacheKey.http2Fingerprint != "" {
		resolved, err := getHTTP2Profile(entry.cacheKey.http2Fingerprint)
		if err != nil {
			log.Warnf("lowhttp: %v, falling back to default HTTP/2 framing", err)
		} else {
			profile = resolved
		}
	}

	newH2Conn := &http2ClientConn{
		conn:                   entry.conn,
		ctx:                    p.ctx,
		mu:                     new(sync.Mutex),
		streams:                make(map[uint32]*http2ClientStream),
		currentStreamID:        1,
		idleTimeout:            p.idleTimeout,
		pingInterval:           p.keepAliveTimeout,
		pingTimeout:            15 * time.Second,
		pendingPings:           make(map[[8]byte]chan struct{}),
		maxFrameSize:           defaultMaxFrameSize,
		initialWindowSize:      65535,
		headerListMaxSize:      ^uint32(0),
		sendWindow:             65535,
		receiveUpdateThreshold: 32 << 10,
		maxStreamsCount:        defaultMaxConcurrentStreamSize,
		frWriteMutex:           new(sync.Mutex),
		hDec:                   hpack.NewDecoder(4096, nil),
		clientPrefaceOk:        utils.NewAtomicBool(),
		closeCh:                make(chan struct{}),
		readLoopExited:         make(chan struct{}),
		serverPrefaceCh:        make(chan struct{}, 1),
		http2Profile:           profile,
		pc:                     entry, // back-reference for eviction
		http2StreamPool: &sync.Pool{
			New: func() interface{} {
				return new(http2ClientStream)
			},
		},
	}

	newH2Conn.hDec.SetAllowedMaxDynamicTableSize(profile.settingValue(http2.SettingHeaderTableSize, 4096))
	newH2Conn.streamsCond = sync.NewCond(newH2Conn.mu)
	newH2Conn.bw = bufio.NewWriterSize(&h2DeadlineWriter{conn: newH2Conn}, 4096)
	newH2Conn.fr = http2.NewFramer(newH2Conn.bw, bufio.NewReader(entry.conn))
	newH2Conn.fr.SetMaxReadFrameSize(defaultMaxFrameSize)

	newH2Conn.mu.Lock()
	if newH2Conn.idleTimeout > 0 {
		newH2Conn.idleTimer = time.AfterFunc(newH2Conn.idleTimeout, func() {
			newH2Conn.mu.Lock()
			if newH2Conn.activeStreams != 0 || newH2Conn.closed {
				newH2Conn.mu.Unlock()
				return
			}
			newH2Conn.closed = true
			newH2Conn.mu.Unlock()
			newH2Conn.setCloseReason(fmt.Sprintf("idle-timeout: no activity for %v", newH2Conn.idleTimeout))
			newH2Conn.setClose()
		})
	}
	newH2Conn.mu.Unlock()

	entry.alt = newH2Conn
}

// ── idle management ──────────────────────────────────────────────────────────

func (p *H2ConnPool) markActive(entry *h2ConnEntry) {
	p.mu.Lock()
	if e := p.idle[entry]; e != nil {
		p.idleLRU.Remove(e)
		delete(p.idle, entry)
	}
	p.mu.Unlock()
}

func (p *H2ConnPool) markIdle(entry *h2ConnEntry) {
	if p.maxIdleConn <= 0 {
		return
	}
	var closeConns []*http2ClientConn
	p.mu.Lock()
	c := entry.alt
	c.mu.Lock()
	idle := !c.closed && c.activeStreams == 0 && p.connMap[entry.cacheKey.hash()] == entry
	c.mu.Unlock()
	if idle {
		if p.idle == nil {
			p.idle = make(map[*h2ConnEntry]*list.Element)
			p.idleLRU = list.New()
		}
		if e := p.idle[entry]; e != nil {
			p.idleLRU.MoveToBack(e)
		} else {
			p.idle[entry] = p.idleLRU.PushBack(entry)
		}
	}
	for len(p.idle) > p.maxIdleConn {
		e := p.idleLRU.Front()
		old := e.Value.(*h2ConnEntry)
		p.idleLRU.Remove(e)
		delete(p.idle, old)
		old.alt.mu.Lock()
		if old.alt.activeStreams == 0 && !old.alt.closed {
			old.alt.closed = true
			closeConns = append(closeConns, old.alt)
		}
		old.alt.mu.Unlock()
	}
	p.mu.Unlock()
	for _, c := range closeConns {
		c.setCloseReason("idle H2 connection limit")
		c.setClose()
	}
}

// removeEntry evicts an entry from the pool's maps. Called by http2ClientConn.setClose
// via the pc back-reference when the connection transitions to closed.
func (p *H2ConnPool) removeEntry(entry *h2ConnEntry) {
	if entry == nil {
		return
	}
	hash := entry.cacheKey.hash()
	p.mu.Lock()
	if e := p.idle[entry]; e != nil {
		p.idleLRU.Remove(e)
		delete(p.idle, entry)
	}
	_, evicted := p.connMap[hash]
	if evicted && p.connMap[hash] == entry {
		delete(p.connMap, hash)
	} else {
		evicted = false
	}
	p.mu.Unlock()

	// Record tombstone only after the readLoop goroutine has fully exited.
	if evicted && entry.alt != nil && atomic.LoadInt32(&p.debugEnabled) != 0 {
		alt := entry.alt
		go func() {
			<-alt.readLoopExited
			alt.mu.Lock()
			tombstone := h2ConnTombstone{
				host:                entry.cacheKey.addr,
				closedAt:            time.Now(),
				finalActiveStreams:  alt.activeStreams,
				totalStreamsCreated: atomic.LoadUint32(&alt.currentStreamID) / 2,
				maxStreams:          alt.maxStreamsCount,
				closeReason:         alt.closeReason,
			}
			alt.mu.Unlock()
			p.mu.Lock()
			p.recordTombstone(tombstone)
			p.mu.Unlock()
		}()
	}
}

func (p *H2ConnPool) recordTombstone(t h2ConnTombstone) {
	if atomic.LoadInt32(&p.debugEnabled) == 0 {
		return
	}
	p.tombstones.push(t)
}

// Clear closes all H2 connections and resets the pool.
func (p *H2ConnPool) Clear() {
	p.mu.Lock()
	entries := make([]*h2ConnEntry, 0, len(p.connMap))
	for _, e := range p.connMap {
		entries = append(entries, e)
	}
	p.connMap = make(map[string]*h2ConnEntry)
	p.idle = nil
	p.idleLRU = nil
	p.generation++
	for _, call := range p.dials {
		call.cancel()
	}
	p.mu.Unlock()
	for _, e := range entries {
		e.alt.setClose()
	}
}

// SetDebugEnabled mirrors the parent pool's debug flag so tombstones are recorded.
func (p *H2ConnPool) SetDebugEnabled(on bool) {
	if on {
		atomic.StoreInt32(&p.debugEnabled, 1)
	} else {
		atomic.StoreInt32(&p.debugEnabled, 0)
	}
}

// Snapshot returns a snapshot of live H2 connections and recent tombstones for debugging.
func (p *H2ConnPool) Snapshot() (live map[string]*h2ConnEntry, tombstones []h2ConnTombstone) {
	p.mu.Lock()
	live = make(map[string]*h2ConnEntry, len(p.connMap))
	for k, v := range p.connMap {
		live[k] = v
	}
	tombstones = p.tombstones.snapshot()
	p.mu.Unlock()
	return
}

// ── net.Conn implementation for h2ConnEntry ──────────────────────────────────
// h2ConnEntry implements net.Conn so it can be stored in the same `conn` variable
// as persistConn and bare net.Conn in exec.go, allowing shared error-handling
// and proxy-fallback paths.

func (e *h2ConnEntry) Read(b []byte) (int, error)  { return e.conn.Read(b) }
func (e *h2ConnEntry) Write(b []byte) (int, error) { return e.conn.Write(b) }
func (e *h2ConnEntry) Close() error                { return e.conn.Close() }
func (e *h2ConnEntry) LocalAddr() net.Addr         { return e.conn.LocalAddr() }
func (e *h2ConnEntry) RemoteAddr() net.Addr        { return e.conn.RemoteAddr() }
func (e *h2ConnEntry) SetDeadline(t time.Time) error {
	return e.conn.SetDeadline(t)
}
func (e *h2ConnEntry) SetReadDeadline(t time.Time) error {
	return e.conn.SetReadDeadline(t)
}
func (e *h2ConnEntry) SetWriteDeadline(t time.Time) error {
	return e.conn.SetWriteDeadline(t)
}

// IsH2 returns true when the entry's ALPN negotiation succeeded with h2.
func (e *h2ConnEntry) IsH2() bool {
	return e.cacheKey != nil && e.cacheKey.scheme == H2
}

// h2DialCall coalesces concurrent dials for the same origin.
type h2DialCall struct {
	done   chan struct{}
	cancel context.CancelFunc
}
