package lowhttp

import (
	"bytes"
	"io"
	"sync"
)

// The unread streaming queue is bounded by the advertised stream receive
// window. DATA never blocks the connection read loop: stream WINDOW_UPDATE is
// sent only as the handler consumes bytes, allowing other streams to progress.
type h2BodyPipe struct {
	mu                         sync.Mutex
	cond                       *sync.Cond
	buf                        bytes.Buffer
	maxBufferedBytes           int
	readerClosed, writerClosed bool
	onRead                     func(int)
	onClose                    func()
}
type h2BodyReader struct{ p *h2BodyPipe }
type h2BodyWriter struct{ p *h2BodyPipe }

func newH2BodyPipe() (*h2BodyReader, *h2BodyWriter) {
	return newH2BodyPipeWithLimit(defaultStreamReceiveWindowSize)
}

func newH2BodyPipeWithLimit(limit int) (*h2BodyReader, *h2BodyWriter) {
	p := &h2BodyPipe{maxBufferedBytes: limit}
	p.cond = sync.NewCond(&p.mu)
	return &h2BodyReader{p}, &h2BodyWriter{p}
}
func (r *h2BodyReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	p := r.p
	p.mu.Lock()
	for p.buf.Len() == 0 && !p.readerClosed && !p.writerClosed {
		p.cond.Wait()
	}
	if p.readerClosed {
		p.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	n, _ := p.buf.Read(dst)
	onRead := p.onRead
	p.mu.Unlock()
	if n > 0 {
		if onRead != nil {
			onRead(n)
		}
		return n, nil
	}
	return 0, io.EOF
}
func (r *h2BodyReader) Close() error {
	p := r.p
	p.mu.Lock()
	first := !p.readerClosed
	p.readerClosed = true
	p.buf = bytes.Buffer{}
	callback := p.onClose
	p.cond.Broadcast()
	p.mu.Unlock()
	if first && callback != nil {
		callback()
	}
	return nil
}
func (w *h2BodyWriter) Write(src []byte) (int, error) {
	p := w.p
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readerClosed || p.writerClosed {
		return 0, io.ErrClosedPipe
	}
	if len(src) > p.maxBufferedBytes-p.buf.Len() {
		return 0, errH2BodyWindowExceeded
	}
	n, err := p.buf.Write(src)
	p.cond.Signal()
	return n, err
}
func (w *h2BodyWriter) Close() error {
	p := w.p
	p.mu.Lock()
	p.writerClosed = true
	p.cond.Broadcast()
	p.mu.Unlock()
	return nil
}
