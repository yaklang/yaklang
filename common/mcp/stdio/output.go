package stdio

import (
	"context"
	"io"
	"os"
	"time"
)

const (
	outputWriteTimeout     = 5 * time.Second
	cancellationWriteGrace = 250 * time.Millisecond
)

// protocolWriter keeps writes ordered and joins every write before returning.
// Close must interrupt a blocked Write; an arbitrary io.Writer cannot provide
// that guarantee. File outputs are prepared separately for cancellable pipe I/O.
type protocolWriter struct {
	ctx    context.Context
	output io.WriteCloser
	err    error
}

func (w *protocolWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := w.output.Write(data)
		done <- result{n, err}
	}()
	timer := time.NewTimer(outputWriteTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.n, r.err
	case <-w.ctx.Done():
		// Allow a final, small failure response when the client is still reading.
		// A blocked or partial response is never followed by a second JSON frame.
		grace := time.NewTimer(cancellationWriteGrace)
		defer grace.Stop()
		select {
		case r := <-done:
			return r.n, r.err
		case <-grace.C:
			w.err = w.ctx.Err()
		case <-timer.C:
			w.err = os.ErrDeadlineExceeded
		}
	case <-timer.C:
		w.err = os.ErrDeadlineExceeded
	}
	_ = w.output.Close()
	r := <-done
	return r.n, w.err
}
