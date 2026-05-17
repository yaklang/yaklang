package aicommon

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrStreamIdleTimeout is returned by StreamIdleTimeoutReader.Read when the
// underlying source has not produced any bytes within either the configured
// time-to-first-byte (TTFB) or inter-byte idle threshold.
//
// 关键词: StreamIdleTimeoutReader, ErrStreamIdleTimeout, AI 流假活兜底
var ErrStreamIdleTimeout = errors.New("stream idle timeout: no bytes received within threshold")

// IsStreamIdleTimeout reports whether err (possibly wrapped) is the sentinel
// ErrStreamIdleTimeout. Callers can use this to special-case the idle-timeout
// path without introducing new error types.
//
// 关键词: IsStreamIdleTimeout, 错误判定
func IsStreamIdleTimeout(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrStreamIdleTimeout)
}

// StreamTimingSnapshot exposes the timing/byte stats tracked by
// StreamIdleTimeoutReader. It is always populated even when no timeout
// threshold is configured (pure-observation mode).
//
// 关键词: StreamTimingSnapshot, TTFB, AI 流计时
type StreamTimingSnapshot struct {
	StartedAt   time.Time
	FirstByteAt time.Time
	DoneAt      time.Time
	TTFB        time.Duration
	Duration    time.Duration
	BytesRead   int64
	TimedOut    bool
}

// StreamIdleTimeoutReader wraps an io.Reader and (optionally) enforces a
// "stream-idle" timeout: it aborts the read if the underlying source has not
// produced any bytes within the time-to-first-byte (ttfb) threshold, or if no
// new bytes appear within the inter-byte idle threshold (idle) after the first
// byte.
//
// Behavior contract:
//   - Setting both ttfb <= 0 and idle <= 0 disables enforcement; the reader
//     becomes a transparent passthrough plus timing tracker. This is the mode
//     used for P0 observation runs (no behavior change, only stats + logs).
//   - When at least one threshold is > 0, a background pump goroutine is
//     started lazily on first Read. The pump reads from the underlying source
//     into its own buffer; Read selects between the next chunk and a timer.
//   - On timeout, Read returns (0, ErrStreamIdleTimeout). The sentinel is
//     sticky: all subsequent Read calls return the same error. The pump
//     goroutine is signalled via pumpStop, but if the underlying source is
//     truly hung (no read ever returns) the pump goroutine may leak until the
//     transport layer eventually drops the connection. That is an explicit
//     tradeoff: the alternative is blocking the entire ReAct loop forever.
//
// Concurrency: Read / Close are safe to call from a single consumer goroutine;
// Snapshot is safe to call concurrently from any goroutine. Read itself is
// not designed for concurrent use (matches the io.Reader contract).
//
// 关键词: StreamIdleTimeoutReader, TTFB / idle 双阈值, 流假活 fail-fast,
//
//	StreamIdleTimeoutReader pump goroutine, P0 观测模式
type StreamIdleTimeoutReader struct {
	underlying io.Reader

	ttfb time.Duration
	idle time.Duration

	// stats (always tracked, regardless of threshold configuration)
	startedAt   time.Time
	firstByteAt atomic.Int64
	doneAt      atomic.Int64
	bytesRead   atomic.Int64
	timedOut    atomic.Bool

	// pump state (only initialized when at least one threshold is > 0)
	pumpOnce sync.Once
	pumpCh   chan streamPumpChunk
	pumpStop chan struct{}
	stopOnce sync.Once

	// partial chunk handling: a single chunk may be larger than the caller's
	// p slice; hold the residual until the next Read drains it.
	partial []byte
	offset  int

	// sticky error: once any terminal condition fires (timeout, underlying
	// error), all subsequent Read calls return the same error.
	finalErr atomic.Pointer[error]
}

type streamPumpChunk struct {
	data []byte
	err  error
}

// NewStreamIdleTimeoutReader wraps the given reader with optional idle-timeout
// enforcement. Pass 0 (or negative) for either threshold to disable that
// check; if both are <= 0 the reader is a transparent passthrough plus timing
// tracker.
//
// 关键词: NewStreamIdleTimeoutReader, ttfb / idle 双阈值构造
func NewStreamIdleTimeoutReader(underlying io.Reader, ttfb, idle time.Duration) *StreamIdleTimeoutReader {
	r := &StreamIdleTimeoutReader{
		underlying: underlying,
		ttfb:       ttfb,
		idle:       idle,
		startedAt:  time.Now(),
	}
	if !r.timeoutsDisabled() {
		r.pumpCh = make(chan streamPumpChunk, 1)
		r.pumpStop = make(chan struct{})
	}
	return r
}

// timeoutsDisabled reports whether both TTFB and idle thresholds are off.
func (r *StreamIdleTimeoutReader) timeoutsDisabled() bool {
	return r.ttfb <= 0 && r.idle <= 0
}

// ensurePump starts the background pump goroutine on first Read. It is a
// no-op when timeouts are disabled.
func (r *StreamIdleTimeoutReader) ensurePump() {
	if r.timeoutsDisabled() {
		return
	}
	r.pumpOnce.Do(func() {
		go r.runPump()
	})
}

// runPump continuously reads chunks from the underlying source and delivers
// them through pumpCh. It exits when the underlying source returns any error
// (including EOF) or when Close is called.
//
// 关键词: runPump, 独立读 goroutine, 自有 buffer 防止 caller 缓冲被污染
func (r *StreamIdleTimeoutReader) runPump() {
	defer close(r.pumpCh)
	buf := make([]byte, 8192)
	for {
		n, err := r.underlying.Read(buf)
		var data []byte
		if n > 0 {
			data = make([]byte, n)
			copy(data, buf[:n])
		}
		select {
		case r.pumpCh <- streamPumpChunk{data: data, err: err}:
		case <-r.pumpStop:
			return
		}
		if err != nil {
			return
		}
	}
}

func (r *StreamIdleTimeoutReader) markFirstByte(n int) {
	if n <= 0 {
		return
	}
	now := time.Now().UnixNano()
	r.firstByteAt.CompareAndSwap(0, now)
	r.bytesRead.Add(int64(n))
}

func (r *StreamIdleTimeoutReader) markDone() {
	r.doneAt.CompareAndSwap(0, time.Now().UnixNano())
}

func (r *StreamIdleTimeoutReader) gotFirstByte() bool {
	return r.firstByteAt.Load() > 0
}

// Read implements io.Reader with optional TTFB / idle timeout enforcement.
func (r *StreamIdleTimeoutReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Pure-passthrough fast path: no goroutine, no channel, just track stats.
	if r.timeoutsDisabled() {
		n, err := r.underlying.Read(p)
		r.markFirstByte(n)
		if err != nil {
			r.markDone()
		}
		return n, err
	}

	// Sticky error path: once any terminal condition fires, return it.
	if errp := r.finalErr.Load(); errp != nil {
		return 0, *errp
	}

	r.ensurePump()

	// Drain any pending partial chunk before pulling the next one.
	if r.offset < len(r.partial) {
		n := copy(p, r.partial[r.offset:])
		r.offset += n
		if r.offset >= len(r.partial) {
			r.partial = nil
			r.offset = 0
		}
		return n, nil
	}

	timeout := r.idle
	if !r.gotFirstByte() {
		timeout = r.ttfb
	}

	var (
		ch streamPumpChunk
		ok bool
	)
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case ch, ok = <-r.pumpCh:
			if !ok {
				// channel closed by pump without delivering EOF separately
				r.markDone()
				eof := io.EOF
				r.finalErr.CompareAndSwap(nil, &eof)
				return 0, io.EOF
			}
		case <-timer.C:
			r.stopOnce.Do(func() { close(r.pumpStop) })
			r.markDone()
			r.timedOut.Store(true)
			timeoutErr := ErrStreamIdleTimeout
			r.finalErr.CompareAndSwap(nil, &timeoutErr)
			return 0, ErrStreamIdleTimeout
		}
	} else {
		ch, ok = <-r.pumpCh
		if !ok {
			r.markDone()
			eof := io.EOF
			r.finalErr.CompareAndSwap(nil, &eof)
			return 0, io.EOF
		}
	}

	// Pure-error chunk: pump returned err without data.
	if len(ch.data) == 0 && ch.err != nil {
		r.markDone()
		errVal := ch.err
		r.finalErr.CompareAndSwap(nil, &errVal)
		return 0, ch.err
	}

	r.markFirstByte(len(ch.data))

	n := copy(p, ch.data)
	if n < len(ch.data) {
		r.partial = ch.data
		r.offset = n
		return n, nil
	}

	// Full chunk delivered. Propagate trailing error (EOF after the last
	// bytes is the common case) immediately.
	if ch.err != nil {
		r.markDone()
		errVal := ch.err
		r.finalErr.CompareAndSwap(nil, &errVal)
		return n, ch.err
	}
	return n, nil
}

// Close signals the pump goroutine to exit. It does not close the underlying
// reader because StreamIdleTimeoutReader does not own it; callers retain that
// responsibility. Close is idempotent and safe to call concurrently.
//
// 关键词: StreamIdleTimeoutReader.Close, 释放 pump goroutine 不接管 underlying
func (r *StreamIdleTimeoutReader) Close() error {
	if r.timeoutsDisabled() {
		return nil
	}
	r.stopOnce.Do(func() { close(r.pumpStop) })
	return nil
}

// Snapshot returns the current timing snapshot. Safe to call concurrently
// from any goroutine; the underlying fields are atomic / immutable.
//
// 关键词: StreamIdleTimeoutReader.Snapshot, 计时观测
func (r *StreamIdleTimeoutReader) Snapshot() StreamTimingSnapshot {
	s := StreamTimingSnapshot{
		StartedAt: r.startedAt,
		BytesRead: r.bytesRead.Load(),
		TimedOut:  r.timedOut.Load(),
	}
	if fb := r.firstByteAt.Load(); fb > 0 {
		s.FirstByteAt = time.Unix(0, fb)
		s.TTFB = s.FirstByteAt.Sub(r.startedAt)
	}
	if d := r.doneAt.Load(); d > 0 {
		s.DoneAt = time.Unix(0, d)
		s.Duration = s.DoneAt.Sub(r.startedAt)
	} else {
		s.Duration = time.Since(r.startedAt)
	}
	return s
}
