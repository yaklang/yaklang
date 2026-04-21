package lowhttp

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// ── Tombstone ─────────────────────────────────────────────────────────────────

// h2ConnTombstone is a lightweight record of an H2 connection that was recently
// closed and evicted from h2ConnMap.  It lets the debug printer show a brief
// post-mortem even after the live entry has been removed.
type h2ConnTombstone struct {
	host                string    // remote address (e.g. "api.example.com:443")
	closedAt            time.Time // wall-clock time the readLoop goroutine fully exited
	finalActiveStreams  int       // activeStreams at close time; non-zero indicates stream leak
	totalStreamsCreated uint32    // currentStreamID/2 — total streams ever issued
	maxStreams          uint32    // SETTINGS_MAX_CONCURRENT_STREAMS received from server
	closeReason         string    // first-writer-wins explanation of why the conn was closed
}

// ── tombstoneQueue ────────────────────────────────────────────────────────────

// tombstoneQueue is a fixed-capacity ring-buffer that stores the most recent
// H2 connection close events.  Oldest entries are silently overwritten when
// the buffer is full.  All methods are NOT goroutine-safe; callers must hold
// an appropriate lock (h2Mu in LowHttpConnPool).
type tombstoneQueue struct {
	buf  []h2ConnTombstone
	head int // index of the oldest entry
	tail int // index where the next entry will be written
	size int // number of valid entries currently stored
	cap  int // fixed capacity of buf
}

// defaultTombstoneQueueSize is the capacity used when none is specified.
const defaultTombstoneQueueSize = 100

// newTombstoneQueue allocates a queue with the given capacity.
func newTombstoneQueue(capacity int) *tombstoneQueue {
	if capacity <= 0 {
		capacity = defaultTombstoneQueueSize
	}
	return &tombstoneQueue{
		buf: make([]h2ConnTombstone, capacity),
		cap: capacity,
	}
}

// push adds t to the queue.  If the queue is full the oldest entry is
// overwritten (ring-buffer semantics).
func (q *tombstoneQueue) push(t h2ConnTombstone) {
	q.buf[q.tail] = t
	q.tail = (q.tail + 1) % q.cap
	if q.size < q.cap {
		q.size++
	} else {
		// Overwrite oldest: advance head to keep ring consistent.
		q.head = (q.head + 1) % q.cap
	}
}

// snapshot returns a slice of all entries ordered newest-first.
// The returned slice is a copy and safe to use after releasing the lock.
func (q *tombstoneQueue) snapshot() []h2ConnTombstone {
	if q.size == 0 {
		return nil
	}
	out := make([]h2ConnTombstone, q.size)
	for i := 0; i < q.size; i++ {
		// Walk backwards from tail to get newest-first ordering.
		idx := (q.tail - 1 - i + q.cap) % q.cap
		out[i] = q.buf[idx]
	}
	return out
}

// ── LowHttpConnPool debug methods ─────────────────────────────────────────────

// recordH2Tombstone pushes t into the bounded tombstone queue.
// It is a no-op when debug mode is disabled, so there is zero overhead in
// production.  Must be called while holding l.h2Mu.
func (l *LowHttpConnPool) recordH2Tombstone(t h2ConnTombstone) {
	if atomic.LoadInt32(&l.debugEnabled) == 0 {
		return
	}
	l.h2Tombstones.push(t)
}

// EnableConnPoolDebug turns the periodic connection-pool status printer on
// (true) or off (false).  When turned on, the first subsequent call to
// getIdleConn will spawn a background goroutine that logs pool state every 5
// seconds until the pool's context is cancelled.
func (l *LowHttpConnPool) EnableConnPoolDebug(on bool) {
	if on {
		atomic.StoreInt32(&l.debugEnabled, 1)
	} else {
		atomic.StoreInt32(&l.debugEnabled, 0)
	}
}

// startDebugPrinter is called on every getIdleConn invocation.  At most one
// printer goroutine is ever started (guarded by debugOnce).  The goroutine
// exits automatically when the pool's context is cancelled.
func (l *LowHttpConnPool) startDebugPrinter() {
	if atomic.LoadInt32(&l.debugEnabled) == 0 {
		return // debug not enabled, fast-exit without consuming the once
	}
	l.debugOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			log.Infof("[connpool-debug] printer started (interval=5s)")
			for {
				select {
				case <-l.ctx.Done():
					log.Infof("[connpool-debug] printer stopped (pool context cancelled)")
					return
				case <-ticker.C:
					if atomic.LoadInt32(&l.debugEnabled) == 0 {
						// Debug was disabled at runtime; keep goroutine alive so
						// it resumes if re-enabled without a new once.Do call.
						continue
					}
					l.debugState()
				}
			}
		}()
	})
}

// debugState snapshots the pool and emits a structured log entry describing
// the current H1 and H2 connection inventory.
func (l *LowHttpConnPool) debugState() {
	var sb strings.Builder
	sb.WriteString("\n╔══════════════════ ConnPool Debug ══════════════════╗\n")

	// ── H1 semaphore ──────────────────────────────────────────────────────────
	semCap := l.maxIdleConn
	semUsed := 0
	if l.connSem != nil {
		semUsed = len(l.connSem)
	}
	sb.WriteString(fmt.Sprintf("║ H1 semaphore : %d / %d slots used\n", semUsed, semCap))

	// ── H1 idle connections ───────────────────────────────────────────────────
	l.idleConnMux.Lock()
	h1Total := 0
	for host, pcs := range l.idleConnMap {
		h1Total += len(pcs)
		for _, pc := range pcs {
			sb.WriteString(fmt.Sprintf("║  H1 %-38s  idle=%v  closed=%v\n",
				host,
				time.Since(pc.idleAt).Round(time.Millisecond),
				pc.closed,
			))
		}
	}
	l.idleConnMux.Unlock()
	if h1Total == 0 {
		sb.WriteString("║  H1  (no idle connections)\n")
	}

	// ── H2 live connections ───────────────────────────────────────────────────
	l.h2Mu.Lock()
	h2Snapshot := make(map[string]*persistConn, len(l.h2ConnMap))
	for k, v := range l.h2ConnMap {
		h2Snapshot[k] = v
	}
	// Snapshot tombstones (newest first) under the same lock so the two
	// sections are consistent with each other.
	tombstones := l.h2Tombstones.snapshot()
	l.h2Mu.Unlock()

	if len(h2Snapshot) == 0 {
		sb.WriteString("║  H2  (no live connections)\n")
	}
	for host, pc := range h2Snapshot {
		if pc.alt == nil {
			sb.WriteString(fmt.Sprintf("║  H2 %-38s  alt=nil\n", host))
			continue
		}
		alt := pc.alt
		alt.mu.Lock()
		active := alt.activeStreams
		maxS := alt.maxStreamsCount
		totalCreated := alt.currentStreamID / 2
		closed := alt.closed
		goAway := alt.readGoAway
		full := alt.full
		streamIDs := make([]uint32, 0, len(alt.streams))
		for id := range alt.streams {
			streamIDs = append(streamIDs, id)
		}
		alt.mu.Unlock()

		// readLoopRunning is accessed atomically (no lock needed).
		readLoopAlive := atomic.LoadInt32(&alt.readLoopRunning) == 1

		status := "OK"
		if closed {
			status = "CLOSED"
		} else if goAway {
			status = "GOAWAY"
		} else if full {
			status = "FULL(id-exhausted)"
		}
		goroutineStr := "readLoop=alive"
		if !readLoopAlive {
			goroutineStr = "readLoop=DEAD⚠"
		}
		// active:       slots reserved via newStream (decremented in waitResponse)
		// totalCreated: cumulative streams ever sent on this connection
		sb.WriteString(fmt.Sprintf(
			"║  H2 %-38s  active=%d  maxStreams=%d  totalCreated=%d  status=%-18s  %s  streamIDs=%v\n",
			host, active, int(maxS), totalCreated, status, goroutineStr, streamIDs,
		))
	}

	// ── Recent H2 closures (tombstones) ───────────────────────────────────────
	if len(tombstones) > 0 {
		sb.WriteString("║─ Recent H2 closures ──────────────────────────────────\n")
		for _, t := range tombstones {
			// closedAt is recorded after readLoop goroutine fully exits.
			ago := time.Since(t.closedAt).Round(time.Millisecond)
			reason := t.closeReason
			if reason == "" {
				reason = "unknown"
			}
			// Flag stream leak: activeStreams should be 0 on clean shutdown.
			// A non-zero value means streams were abandoned without waitResponse.
			leakStr := ""
			if t.finalActiveStreams > 0 {
				leakStr = fmt.Sprintf("  ⚠ streamLeak=%d", t.finalActiveStreams)
			}
			sb.WriteString(fmt.Sprintf(
				"║  ✗ H2 %-36s  exited %-10s ago  maxStreams=%d  totalCreated=%d%s  reason=%s\n",
				t.host, ago, int(t.maxStreams), t.totalStreamsCreated, leakStr, reason,
			))
		}
	}

	sb.WriteString("╚════════════════════════════════════════════════════╝")
	log.Infof("%s", sb.String())
}
