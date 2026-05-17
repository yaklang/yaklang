package aicommon

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStreamIdleTimeoutReader_PassthroughMode ensures both thresholds = 0
// turns the wrapper into a pure passthrough, with timing stats still tracked.
// 关键词: StreamIdleTimeoutReader passthrough, 纯观测模式
func TestStreamIdleTimeoutReader_PassthroughMode(t *testing.T) {
	payload := strings.NewReader("hello world!")
	r := NewStreamIdleTimeoutReader(payload, 0, 0)
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll err=%v", err)
	}
	if string(got) != "hello world!" {
		t.Fatalf("payload mismatch: %q", got)
	}

	s := r.Snapshot()
	if s.BytesRead != int64(len("hello world!")) {
		t.Fatalf("bytes mismatch: got %d", s.BytesRead)
	}
	if s.FirstByteAt.IsZero() {
		t.Fatalf("expected firstByteAt to be set")
	}
	if s.DoneAt.IsZero() {
		t.Fatalf("expected doneAt to be set")
	}
	if s.TimedOut {
		t.Fatalf("did not expect TimedOut in passthrough mode")
	}
}

// TestStreamIdleTimeoutReader_TTFBTimeout proves that a source which never
// sends the first byte is aborted within the ttfb threshold.
// 关键词: TTFB 超时, 首字节假活
func TestStreamIdleTimeoutReader_TTFBTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	// pw is never written to and never closed within the test budget; the
	// reader must independently abort via TTFB timeout.
	defer pw.Close()

	r := NewStreamIdleTimeoutReader(pr, 80*time.Millisecond, 0)
	defer r.Close()

	start := time.Now()
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	elapsed := time.Since(start)

	if n != 0 {
		t.Fatalf("expected n=0, got n=%d", n)
	}
	if !IsStreamIdleTimeout(err) {
		t.Fatalf("expected ErrStreamIdleTimeout, got %v", err)
	}
	if elapsed < 50*time.Millisecond || elapsed > 1*time.Second {
		t.Fatalf("ttfb timeout fired outside reasonable window: %v", elapsed)
	}

	// Sticky: subsequent Reads should keep returning the timeout error.
	n2, err2 := r.Read(buf)
	if n2 != 0 || !IsStreamIdleTimeout(err2) {
		t.Fatalf("sticky timeout broken: n=%d err=%v", n2, err2)
	}

	s := r.Snapshot()
	if !s.TimedOut {
		t.Fatalf("expected TimedOut=true")
	}
}

// TestStreamIdleTimeoutReader_IdleTimeout proves that a source which sends
// some bytes and then stalls is aborted within the idle threshold.
// 关键词: 字节间空闲超时, 流中途假活
func TestStreamIdleTimeoutReader_IdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	r := NewStreamIdleTimeoutReader(pr, 0, 80*time.Millisecond)
	defer r.Close()

	go func() {
		pw.Write([]byte("first"))
	}()

	buf := make([]byte, 16)
	// First Read should return the initial chunk.
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("unexpected err on first read: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected first chunk to arrive")
	}

	// Drain any residual partial bytes within the same chunk before exposing
	// the wrapper to the actual idle gap.
	for r.offset < len(r.partial) {
		_, err := r.Read(buf)
		if err != nil {
			t.Fatalf("residual drain err: %v", err)
		}
	}

	// Source now silent. Second Read must hit idle timeout.
	start := time.Now()
	n2, err2 := r.Read(buf)
	elapsed := time.Since(start)
	if n2 != 0 {
		t.Fatalf("expected n=0 on idle, got %d", n2)
	}
	if !IsStreamIdleTimeout(err2) {
		t.Fatalf("expected ErrStreamIdleTimeout, got %v", err2)
	}
	if elapsed < 50*time.Millisecond || elapsed > 1*time.Second {
		t.Fatalf("idle timeout fired outside reasonable window: %v", elapsed)
	}
}

// TestStreamIdleTimeoutReader_NormalCompletion verifies that with sane
// thresholds and a fast source, the wrapper returns the full payload without
// triggering any timeout.
// 关键词: 正常完成路径, 无误杀
func TestStreamIdleTimeoutReader_NormalCompletion(t *testing.T) {
	payload := bytes.Repeat([]byte("ab"), 1024)
	r := NewStreamIdleTimeoutReader(bytes.NewReader(payload), 500*time.Millisecond, 500*time.Millisecond)
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll err=%v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch")
	}
	s := r.Snapshot()
	if s.BytesRead != int64(len(payload)) {
		t.Fatalf("bytes mismatch: got %d want %d", s.BytesRead, len(payload))
	}
	if s.TimedOut {
		t.Fatalf("did not expect timeout on fast source")
	}
}

// TestStreamIdleTimeoutReader_EarlyClose verifies that Close is idempotent
// and does not panic when invoked before / after a timeout fires.
// 关键词: Close 幂等, pumpStop 单次关闭
func TestStreamIdleTimeoutReader_EarlyClose(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	r := NewStreamIdleTimeoutReader(pr, 100*time.Millisecond, 100*time.Millisecond)
	if err := r.Close(); err != nil {
		t.Fatalf("first Close err=%v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close err=%v", err)
	}
}

// TestStreamIdleTimeoutReader_UnderlyingError ensures that errors from the
// underlying source are propagated as-is, not masked as idle-timeout.
// 关键词: 底层 error 透传, 不误判为 timeout
func TestStreamIdleTimeoutReader_UnderlyingError(t *testing.T) {
	sentinel := errors.New("boom")
	src := &errorReader{err: sentinel}
	r := NewStreamIdleTimeoutReader(src, 200*time.Millisecond, 200*time.Millisecond)
	defer r.Close()

	buf := make([]byte, 8)
	_, err := r.Read(buf)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if IsStreamIdleTimeout(err) {
		t.Fatalf("error wrongly reported as idle timeout")
	}
}

// TestStreamIdleTimeoutReader_ConcurrentSnapshot exercises Snapshot from
// another goroutine while Read is in flight to confirm no data races.
// 关键词: Snapshot 并发安全
func TestStreamIdleTimeoutReader_ConcurrentSnapshot(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	r := NewStreamIdleTimeoutReader(pr, 0, 200*time.Millisecond)
	defer r.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			pw.Write([]byte("chunk"))
			time.Sleep(20 * time.Millisecond)
		}
		pw.Close()
	}()

	stopSnap := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopSnap:
				return
			default:
				_ = r.Snapshot()
			}
		}
	}()

	got, err := io.ReadAll(r)
	close(stopSnap)
	wg.Wait()
	if err != nil {
		t.Fatalf("ReadAll err=%v", err)
	}
	if len(got) != 25 {
		t.Fatalf("unexpected bytes: %q", got)
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}
