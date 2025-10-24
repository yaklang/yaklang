package utils

import (
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
)

var NewPipe = bufpipe.NewPipe
var NewBufPipe = bufpipe.NewBufPipe

func NewMirrorPipe(count int) ([]*bufpipe.PipeReader, *bufpipe.PipeWriter) {
	if count <= 0 {
		return nil, nil
	}

	pr, pw := NewPipe()

	numPipes := count
	writers := make([]*bufpipe.PipeWriter, numPipes)
	readers := make([]*bufpipe.PipeReader, numPipes)
	for i := 0; i < count; i++ {
		readers[i], writers[i] = NewPipe()
	}
	go func() {
		defer func() {
			for _, w := range writers {
				w.Close()
			}
		}()
		var pipes = make([]io.Writer, numPipes)
		for i, w := range writers {
			pipes[i] = w
		}

		io.Copy(io.MultiWriter(pipes...), pr)
	}()
	return readers, pw
}

//type pipe struct {
//	cond       *sync.Cond
//	buf        *bytes.Buffer
//	rerr, werr error
//}
//
//func (p *pipe) BytesBuffer() *bytes.Buffer {
//	return p.buf
//}

// A PipeReader is the read half of a pipe.
//type PipeReader struct {
//	*pipe
//	count int
//}

//// A PipeWriter is the write half of a pipe.
//type PipeWriter struct {
//	*pipe
//	count int
//}
//
//func (p *PipeWriter) Count() int {
//	return p.count
//}
//
//func (p *PipeReader) Count() int {
//	return p.count
//}
//
//func (p *PipeWriter) BytesBuffer() *bytes.Buffer {
//	return p.buf
//}
//
//func (p *PipeReader) BytesBuffer() *bytes.Buffer {
//	return p.buf
//}
//
//// NewBufPipe creates a synchronous pipe using buf as its initial contents. It can be
//// used to connect code expecting an io.Reader with code expecting an io.Writer.
////
//// Unlike io.Pipe, writes never block because the internal buffer has variable
//// size. Reads block only when the buffer is empty.
////
//// It is safe to call Read and Write in parallel with each other or with Close.
//// Parallel calls to Read and parallel calls to Write are also safe: the
//// individual calls will be gated sequentially.
////
//// The new pipe takes ownership of buf, and the caller should not use buf after
//// this call. New is intended to prepare a PipeReader to read existing data. It
//// can also be used to set the initial size of the internal buffer for writing.
//// To do that, buf should have the desired capacity but a length of zero.
//func NewBufPipe(buf []byte) (*PipeReader, *PipeWriter) {
//	p := &pipe{
//		buf:  bytes.NewBuffer(buf),
//		cond: sync.NewCond(new(sync.Mutex)),
//	}
//	return &PipeReader{
//			pipe: p,
//		}, &PipeWriter{
//			pipe: p,
//		}
//}
//
//func NewPipe() (*PipeReader, *PipeWriter) {
//	return NewBufPipe(nil)
//}
//
//// Read implements the standard Read interface: it reads data from the pipe,
//// reading from the internal buffer, otherwise blocking until a writer arrives
//// or the write end is closed. If the write end is closed with an error, that
//// error is returned as err; otherwise err is io.EOF.
//func (r *PipeReader) Read(data []byte) (int, error) {
//	r.cond.L.Lock()
//	defer r.cond.L.Unlock()
//
//RETRY:
//	n, err := r.buf.Read(data)
//	r.count += n
//	// If not closed and no read, wait for writing.
//	if err == io.EOF && r.rerr == nil && n == 0 {
//		r.cond.Wait()
//		goto RETRY
//	}
//	if err == io.EOF {
//		return n, r.rerr
//	}
//	return n, err
//}
//
//// Close closes the reader; subsequent writes from the write half of the pipe
//// will return error ErrClosedPipe.
//func (r *PipeReader) Close() error {
//	return r.CloseWithError(nil)
//}
//
//// CloseWithError closes the reader; subsequent writes to the write half of the
//// pipe will return the error err.
//func (r *PipeReader) CloseWithError(err error) error {
//	r.cond.L.Lock()
//	defer r.cond.L.Unlock()
//
//	if err == nil {
//		err = ErrClosedPipe
//	}
//	r.werr = err
//	return nil
//}
//
//// Write implements the standard Write interface: it writes data to the internal
//// buffer. If the read end is closed with an error, that err is returned as err;
//// otherwise err is ErrClosedPipe.
//func (w *PipeWriter) Write(data []byte) (int, error) {
//	w.cond.L.Lock()
//	defer w.cond.L.Unlock()
//
//	if w.werr != nil {
//		return 0, w.werr
//	}
//
//	n, err := w.buf.Write(data)
//	w.count += n
//	w.cond.Signal()
//	return n, err
//}
//
//// WriteString implements the String Write interface: it writes data to the internal
//// buffer. If the read end is closed with an error, that err is returned as err;
//// otherwise err is ErrClosedPipe.
//func (w *PipeWriter) WriteString(data string) (int, error) {
//	return w.Write([]byte(data))
//}
//
//// Close closes the writer; subsequent reads from the read half of the pipe will
//// return io.EOF once the internal buffer get empty.
//func (w *PipeWriter) Close() error {
//	return w.CloseWithError(nil)
//}
//
//// CloseWithError closes the writer; subsequent reads from the read half of the pipe will
//// return err once the internal buffer get empty.
//func (w *PipeWriter) CloseWithError(err error) error {
//	w.cond.L.Lock()
//	defer w.cond.L.Unlock()
//
//	if err == nil {
//		err = io.EOF
//	}
//	w.rerr = err
//	w.cond.Broadcast()
//	return nil
//}
//
//type PerHandlerWriter struct {
//	writer       io.Writer
//	buf          []byte
//	preHandlerOk bool
//	handler      func([]byte) ([]byte, bool)
//}
//
//func (w *PerHandlerWriter) Write(p []byte) (n int, err error) {
//	if w.preHandlerOk {
//		return w.writer.Write(p)
//	}
//	w.buf = append(w.buf, p...)
//	if remainData, ok := w.handler(w.buf); !ok {
//		return len(p), nil
//	} else {
//		w.preHandlerOk = true
//		_, err = w.writer.Write(remainData)
//		if err == nil {
//			return len(p), err
//		}
//		return 0, err
//	}
//}
//
//func NewPerHandlerWriter(w io.Writer, handler func([]byte) ([]byte, bool)) *PerHandlerWriter {
//	return &PerHandlerWriter{
//		writer:  w,
//		handler: handler,
//	}
//}
