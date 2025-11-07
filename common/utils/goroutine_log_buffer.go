package utils

import (
	"bytes"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	commonlog "github.com/yaklang/yaklang/common/log"
)

// GoroutineLogBuffer multiplexes writes coming from different goroutines into
// per-goroutine buffers. Attach the current goroutine before running the work
// you want to capture; flush the buffer when the task completes.
//
// Typical usage:
//
//	b := utils.NewGoroutineLogBuffer(os.Stdout)
//	buf, detach := b.Attach()
//	defer func() {
//	    b.Flush("task name", buf)
//	    detach()
//	}()
//	... // perform work that writes logs via b
type GoroutineLogBuffer struct {
	fallback    io.Writer
	mu          sync.Mutex
	buffers     sync.Map
	defaultBuf  *bytes.Buffer
	onlyFailed  bool
	defaultName string
}

type GoroutineLogBufferOption func(*GoroutineLogBuffer)

// WithGoroutineLogFailedOnly toggles the behavior to only flush buffers when
// the associated work failed.
func WithGoroutineLogFailedOnly(b bool) GoroutineLogBufferOption {
	return func(buffer *GoroutineLogBuffer) {
		buffer.onlyFailed = b
	}
}

// NewGoroutineLogBuffer creates a GoroutineLogBuffer that writes to fallback
// when there is no active buffer for the calling goroutine. If fallback is
// nil, io.Discard is used.
func NewGoroutineLogBuffer(fallback io.Writer, opts ...GoroutineLogBufferOption) *GoroutineLogBuffer {
	if fallback == nil {
		fallback = io.Discard
	}
	buf := &GoroutineLogBuffer{
		fallback:    fallback,
		defaultBuf:  new(bytes.Buffer),
		defaultName: "GLOBAL",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(buf)
		}
	}
	return buf
}

// Write implements io.Writer, routing the bytes to the buffer registered for
// the calling goroutine when present, or falling back to the shared writer.
func (b *GoroutineLogBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if buf, ok := b.buffers.Load(currentGoroutineID()); ok {
		buf.(*bytes.Buffer).Write(p)
		return len(p), nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.defaultBuf.Write(p)
	return len(p), nil
}

// Attach registers a new buffer for the current goroutine and returns it along
// with a detach function. Detach should be called after the goroutine finishes
// using the buffer to avoid leaks.
func (b *GoroutineLogBuffer) Attach() (*bytes.Buffer, func()) {
	buf := new(bytes.Buffer)
	id := currentGoroutineID()
	if id == 0 {
		return buf, func() {}
	}
	b.buffers.Store(id, buf)
	return buf, func() {
		b.buffers.Delete(id)
	}
}

// Flush writes the collected buffer to the fallback writer with a simple
// header/footer so the output stands out in shared logs. When the buffer is
// configured to only flush failures and `failed` is false, the call is a no-op.
func (b *GoroutineLogBuffer) Flush(label string, buf *bytes.Buffer, failed bool) {
	if buf == nil {
		return
	}
	if b.onlyFailed && !failed {
		return
	}
	content := buf.Bytes()
	if len(content) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	fmt.Fprintf(b.fallback, "\n===== %s =====\n", label)
	b.fallback.Write(content)
	if !bytes.HasSuffix(content, []byte("\n")) {
		b.fallback.Write([]byte("\n"))
	}
	fmt.Fprintln(b.fallback, "===== END =====")
	buf.Reset()
}

// FlushDefault flushes logs collected from goroutines without explicit
// attachments. It always flushes regardless of the failed flag.
func (b *GoroutineLogBuffer) FlushDefault() {
	b.Flush(b.defaultName, b.defaultBuf, true)
}

// FlushDefault flushes logs collected from goroutines without explicit
// attachments. It always flushes regardless of the failed flag.

func currentGoroutineID() int64 {
	buf := make([]byte, 64)
	n := runtime.Stack(buf, false)
	if n <= 0 {
		return 0
	}
	fields := strings.Fields(string(buf[:n]))
	if len(fields) < 2 {
		return 0
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// GoroutineLogCapture wires os.Stdout/os.Stderr and the logging subsystems to a
// GoroutineLogBuffer, enabling per-goroutine log buffering with a single
// cleanup function.
type GoroutineLogCapture struct {
	Buffer     *GoroutineLogBuffer
	origStdout *os.File
	origStderr *os.File
	stdoutR    *os.File
	stdoutW    *os.File
	stderrR    *os.File
	stderrW    *os.File
	copyWG     sync.WaitGroup
	stopOnce   sync.Once
}

// StartGoroutineLogCapture creates a new GoroutineLogBuffer (using fallback as
// the shared writer) and redirects stdout/stderr, the standard log package, and
// yaklang's common log package into that buffer. Call Stop to restore the
// previous state.
func StartGoroutineLogCapture(fallback io.Writer, opts ...GoroutineLogBufferOption) (*GoroutineLogCapture, error) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	if fallback == nil {
		if origStdout != nil {
			fallback = origStdout
		} else {
			fallback = io.Discard
		}
	}

	buffer := NewGoroutineLogBuffer(fallback, opts...)
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return nil, err
	}

	capture := &GoroutineLogCapture{
		Buffer:     buffer,
		origStdout: origStdout,
		origStderr: origStderr,
		stdoutR:    stdoutR,
		stdoutW:    stdoutW,
		stderrR:    stderrR,
		stderrW:    stderrW,
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	capture.copyWG.Add(2)
	go func() {
		defer capture.copyWG.Done()
		_, _ = io.Copy(buffer, stdoutR)
	}()
	go func() {
		defer capture.copyWG.Done()
		_, _ = io.Copy(buffer, stderrR)
	}()

	stdlog.SetOutput(buffer)
	commonlog.SetOutput(buffer)

	return capture, nil
}

// Stop restores stdout/stderr and log outputs back to their original targets.
// It is safe to call multiple times.
func (c *GoroutineLogCapture) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if c.stdoutW != nil {
			_ = c.stdoutW.Close()
		}
		if c.stderrW != nil {
			_ = c.stderrW.Close()
		}
		c.copyWG.Wait()
		if c.stdoutR != nil {
			_ = c.stdoutR.Close()
		}
		if c.stderrR != nil {
			_ = c.stderrR.Close()
		}
		if c.origStdout != nil {
			os.Stdout = c.origStdout
			stdlog.SetOutput(c.origStdout)
			commonlog.SetOutput(c.origStdout)
		}
		if c.origStderr != nil {
			os.Stderr = c.origStderr
		}
		if c.Buffer != nil {
			c.Buffer.FlushDefault()
		}
	})
}
