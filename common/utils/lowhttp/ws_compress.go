/*
Ref: https://github.com/gobwas/ws

The MIT License (MIT)

Copyright (c) 2017-2021 Sergey Kamardin <gobwas@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package lowhttp

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"sync"
)

var compressionTail = []byte{
	0, 0, 0xff, 0xff,
}

var compressionReadTail = []byte{
	0, 0, 0xff, 0xff, 1, 0, 0, 0xff, 0xff,
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// trimFourBytesWriter is a tiny proxy-buffer that writes all but 4 last bytes to the
// destination.
type trimFourBytesWriter struct {
	dst  io.Writer
	tail []byte
}

func (c *trimFourBytesWriter) Write(p []byte) (int, error) {
	if c.tail == nil {
		c.tail = make([]byte, 0, 4)
	}

	extra := len(c.tail) + len(p) - 4

	if extra <= 0 {
		c.tail = append(c.tail, p...)
		return len(p), nil
	}

	// Now we need to write as many extra bytes as we can from the previous tail.
	if extra > len(c.tail) {
		extra = len(c.tail)
	}
	if extra > 0 {
		_, err := c.dst.Write(c.tail[:extra])
		if err != nil {
			return 0, err
		}

		// Shift remaining bytes in tail over.
		n := copy(c.tail, c.tail[extra:])
		c.tail = c.tail[:n]
	}

	// If p is less than or equal to 4 bytes,
	// all of it is is part of the tail.
	if len(p) <= 4 {
		c.tail = append(c.tail, p...)
		return len(p), nil
	}

	// Otherwise, only the last 4 bytes are.
	c.tail = append(c.tail, p[len(p)-4:]...)

	p = p[:len(p)-4]
	n, err := c.dst.Write(p)
	return n + 4, err
}

func (c *trimFourBytesWriter) reset(dst io.Writer) {
	c.tail = c.tail[:0]
	c.dst = dst
}

// Compressor is an interface holding deflate compression implementation.
type Compressor interface {
	io.Writer
	Flush() error
}

// WriteResetter is an optional interface that Compressor can implement.
type WriteResetter interface {
	Reset(io.Writer)
}

// rfc7692 7.2.1
// An endpoint uses the following algorithm to compress a message.
// 1.Compress all the octets of the payload of the message usingDEFLATE.
// 2.If the resulting data does not end with a	n empty DEFLATE block with no compression (the "BTYPE" bits are set to 00), append an empty DEFLATE block with no compression to the tail end.
// 3.Remove 4 octets (that are 0x00 0x00 0xff 0xff) from the tail end.
// After this step, the last octet of the compressed data contains(possibly part of) the DEFLATE header bits with the "BTYPE" bitsset to 00.
// msgWriter implements compression for an io.msgWriter object using Compressor.
// Essentially msgWriter is a thin wrapper around Compressor interface to meet
// PMCE specs.
//
// After all data has been written client should call Flush() method.
// If any error occurs after writing to or flushing a msgWriter, all subsequent
// calls to Write(), Flush() or Close() will return the error.
//
// msgWriter might be reused for different io.msgWriter objects after its Reset()
// method has been called.
// NOTE: msgWriter uses compressor constructor function instead of field to
// reach these goals:
//  1. To shrink Compressor interface and make it easier to be implemented.
//  2. If used as a field (and argument to the NewWriter()), Compressor object
//     will probably be initialized twice - first time to pass into msgWriter, and
//     second time during msgWriter initialization (which does Reset() internally).
//  3. To get rid of wrappers if Reset() would be a part of	Compressor.
//     E.g. non conformant implementations would have to provide it somehow,
//     probably making a wrapper with the same constructor function.
//  4. To make Reader and msgWriter API the same. That is, there is no Reset()
//     method for flate.Reader already, so we need to provide it as a wrapper
//     (see point #3), or drop the Reader.Reset() method.
type msgWriter struct {
	dest io.Writer
	c    Compressor
	err  error
	ctor func(io.Writer) Compressor
	tr   trimFourBytesWriter
}

var flateReaderPool sync.Pool

func getFlateReader(r io.Reader, dict []byte) io.Reader {
	fr, ok := flateReaderPool.Get().(io.Reader)
	if !ok {
		return flate.NewReaderDict(r, dict)
	}
	fr.(flate.Resetter).Reset(r, dict)
	return fr
}

func putFlateReader(fr io.Reader) {
	flateReaderPool.Put(fr)
}

var flateWriterPool sync.Pool

func getFlateWriter(w io.Writer) *flate.Writer {
	fw, ok := flateWriterPool.Get().(*flate.Writer)
	if !ok {
		fw, _ = flate.NewWriter(w, flate.DefaultCompression)
		return fw
	}
	fw.Reset(w)
	return fw
}

func putFlateWriter(w io.Writer) {
	if fw, ok := w.(*flate.Writer); ok {
		flateWriterPool.Put(fw)
	}
}

// NewWriter returns a new Writer.
func NewWriter(w io.Writer, ctor func(io.Writer) Compressor) *msgWriter {
	// NOTE: NewWriter() is chosen against structure with exported fields here
	// due its Reset() method, which in case of structure, would change
	// exported field.
	ret := &msgWriter{
		dest: w,
		ctor: ctor,
	}
	ret.Reset(w)
	return ret
}

// Reset resets Writer to compress data into dest.
// Any not flushed data will be lost.
func (w *msgWriter) Reset(dest io.Writer) {
	w.err = nil
	w.tr.reset(dest)
	if x, ok := w.c.(WriteResetter); ok {
		x.Reset(&w.tr)
	} else {
		w.c = w.ctor(&w.tr)
	}
}

// Write implements io.Writer.
func (w *msgWriter) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	n, w.err = w.c.Write(p)
	return n, w.err
}

// Flush writes any pending data into w.Dest.
func (w *msgWriter) Flush() error {
	if w.err != nil {
		return w.err
	}
	w.err = w.c.Flush()
	w.checkTail()
	return w.err
}

// Close closes Writer and a Compressor instance used under the hood (if it
// implements io.Closer interface).
func (w *msgWriter) Close() error {
	if w.err != nil {
		return w.err
	}
	if c, ok := w.c.(io.Closer); ok {
		w.err = c.Close()
	}
	w.checkTail()
	return w.err
}

// Err returns an error happened during any operation.
func (w *msgWriter) Err() error {
	return w.err
}

func (w *msgWriter) checkTail() {
	if w.err == nil && !bytes.Equal(w.tr.tail, compressionTail) {
		w.err = fmt.Errorf(
			"wsflate: bad compressor: unexpected stream tail: %#x vs %#x",
			w.tr.tail, compressionTail,
		)
	}
}

// WriterFunc is used to implement one off io.Writers.
type WriterFunc func(p []byte) (int, error)

func (f WriterFunc) Write(p []byte) (int, error) {
	return f(p)
}

/*
-----------------------------------------------
*/

func (fr *FrameReader) reset(frame *Frame) {
	fr.frame = frame
	fr.limitReader.N = int64(fr.frame.payloadLength)
	// trigger on first frame
	if fr.isDeflate && frame.RSV1() {
		fr.resetFlate()
	}
}

func (fr *FrameReader) resetFlate() {
	c := fr.c
	if c == nil {
		return
	}
	var buf []byte

	contextTakeover := c.Extensions.flateContextTakeover()
	if contextTakeover {
		if fr.dict == nil {
			fr.dict = &slidingWindow{}
		}
		fr.dict.init(32768)
		buf = fr.dict.buf
	}

	if fr.fragmentBuffer == nil {
		fr.fragmentBuffer = bytes.NewBuffer(nil)
	} else {
		fr.fragmentBuffer.Reset()
	}
	fr.flateTail.Reset(compressionTail)

	fr.flateReader = getFlateReader(io.MultiReader(fr.fragmentBuffer, fr.flateTail), buf)
}

func (fr *FrameReader) putFlateReader() {
	if fr.flateReader != nil {
		putFlateReader(fr.flateReader)
		fr.flateReader = nil
	}
}

func (fr *FrameReader) readPayloadN(n uint64) ([]byte, error) {
	var (
		nn  int
		p   []byte
		err error
	)
	frame := fr.frame

	p = make([]byte, n)
	nn, err = io.ReadFull(fr.r, p)
	p = p[:nn]

	frame.rawPayload = make([]byte, len(p))
	copy(frame.rawPayload, p)

	if frame.mask {
		maskBytes(frame.maskingKey, p, len(p))
	}

	if fr.fragmentBuffer != nil {
		fr.fragmentBuffer.Grow(nn)
		fr.fragmentBuffer.Write(p)
	}

	if fr.flateReader != nil && frame.FIN() && !frame.IsControl() { // 如果是压缩的帧，则需要考虑流式问题，需要在存储流式的帧然后进行最后统一解压
		frame.UnsetRSV1()
		c := fr.c
		p, err = io.ReadAll(fr.flateReader)
		if len(p) > 0 && errors.Is(err, io.ErrUnexpectedEOF) {
			err = nil
		}

		if c != nil && c.Extensions.flateContextTakeover() {
			fr.dict.write(p)
		}

		fr.putFlateReader()
	}

	return p, err
}

func (fw *FrameWriter) reset(opcode int, resetFlate bool) {
	fw.opcode = opcode
	if resetFlate {
		fw.resetFlate()
	}
}

func (fw *FrameWriter) resetFlate() {
	if fw.fw == nil {
		fw.fw = NewWriter(WriterFunc(fw.writeContinueDeflateFrame), func(w io.Writer) Compressor {
			return getFlateWriter(w)
		})
	} else {
		fw.fw.Reset(WriterFunc(fw.writeContinueDeflateFrame))
	}
}

func (fw *FrameWriter) putFlateWriter() {
	if fw.fw != nil {
		putFlateWriter(fw.fw.c)
		fw.fw = nil
	}
}

func (fw *FrameWriter) writeContinueDeflateFrame(data []byte) (int, error) {
	// todo: mask: client or server?
	// only first frame need to set rsv1
	n, err := fw.WriteDirect(false, fw.opcode != ContinueMessage, fw.opcode, true, data)
	if err != nil {
		return n, err
	}
	fw.opcode = ContinueMessage
	return n, nil
}

// FrameWriter.writeDeflateFrame -> msgWriter.Write(compress) -> tr.Write(trim last four bytes) -> FrameWriter.writeContinueDeflateFrame -> FrameWriter.directWrite(continue) --> FrameWriter.directWrite(fin)
func (fw *FrameWriter) writeDeflateFrame(data []byte) (int, error) {
	w := fw.fw

	n, err := w.Write(data)
	if err != nil {
		return n, err
	}

	if err = w.Flush(); err != nil {
		return n, err
	}

	if c := fw.c; c != nil && !c.Extensions.flateContextTakeover() {
		fw.putFlateWriter()
	}
	// write fin frame
	fw.WriteDirect(true, false, ContinueMessage, true, nil)

	return n, nil
}
