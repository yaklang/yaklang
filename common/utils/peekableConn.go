package utils

import (
	"io"
	"net"
	"yaklang/common/log"
)

type bufferable interface {
	GetBuf() []byte
	GetReader() io.Reader
	SetBuf([]byte)
}

func _peekablePeek(r bufferable, i int) (_ []byte, fErr error) {
	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fErr = io.EOF
		}
	}()

	var buf = make([]byte, i)
	l := len(r.GetBuf())
	if i <= l {
		copy(buf, r.GetBuf()[:i])
		return buf, nil
	} else {
		copy(buf, r.GetBuf())
		var a = r.GetReader()
		if a == nil {
			return nil, io.EOF
		}
		n, err := io.ReadFull(a, buf[l:])
		r.SetBuf(buf[:l+n])
		return r.GetBuf(), err
	}
}

func _peekableRead(p bufferable, b []byte) (int, error) {
	l := len(p.GetBuf())
	if l <= 0 {
		return p.GetReader().Read(b)
	}
	rl := len(b)
	if rl <= l {
		copy(b, p.GetBuf()[:rl])
		p.SetBuf(p.GetBuf()[rl:])
		return rl, nil
	}
	if l > 0 {
		n := copy(b, p.GetBuf())
		p.SetBuf(p.GetBuf()[n:])
		return n, nil
	}
	return p.GetReader().Read(b)
}

func NewPeekableNetConn(r net.Conn) *BufferedPeekableConn {
	return &BufferedPeekableConn{
		Conn: r,
	}
}

func NewPeekableReader(r io.Reader) *BufferedPeekableReader {
	return &BufferedPeekableReader{
		Reader: r,
	}
}

func NewPeekableReaderWriter(r io.ReadWriter) *BufferedPeekableReaderWriter {
	return &BufferedPeekableReaderWriter{
		ReadWriter: r,
	}
}

type BufferedPeekableConn struct {
	net.Conn
	buf []byte
}

func (b *BufferedPeekableConn) GetOriginConn() net.Conn {
	return b.Conn
}

func (b *BufferedPeekableConn) Peek(i int) ([]byte, error) {
	return _peekablePeek(b, i)
}

func (b *BufferedPeekableConn) PeekByte() (byte, error) {
	buf, err := b.Peek(1)
	if err != nil {
		return 0, err
	}
	if len(buf) != 1 {
		return 0, io.EOF
	}
	return buf[0], nil
}

func (b *BufferedPeekableConn) PeekUint16() uint16 {
	buf, err := b.Peek(2)
	if err != nil {
		return 0
	}
	if len(buf) != 2 {
		return 0
	}
	return uint16(buf[0])<<8 | uint16(buf[1])
}

func (b *BufferedPeekableConn) Read(buf []byte) (int, error) {
	return _peekableRead(b, buf)
}

func (b *BufferedPeekableConn) GetReader() io.Reader {
	return b.Conn
}

func (b *BufferedPeekableConn) SetBuf(buf []byte) {
	b.buf = buf
}

func (b *BufferedPeekableConn) GetBuf() []byte {
	return b.buf
}

type BufferedPeekableReaderWriter struct {
	io.ReadWriter
	buf []byte
}

func (b *BufferedPeekableReaderWriter) Peek(i int) ([]byte, error) {
	return _peekablePeek(b, i)
}

func (b *BufferedPeekableReaderWriter) Read(buf []byte) (int, error) {
	return _peekableRead(b, buf)
}

func (b *BufferedPeekableReaderWriter) GetReader() io.Reader {
	return b.ReadWriter
}

func (b *BufferedPeekableReaderWriter) SetBuf(buf []byte) {
	b.buf = buf
}

func (b *BufferedPeekableReaderWriter) GetBuf() []byte {
	return b.buf
}

type BufferedPeekableReader struct {
	io.Reader
	buf []byte
}

func (b *BufferedPeekableReader) Peek(i int) ([]byte, error) {
	return _peekablePeek(b, i)
}

func (b *BufferedPeekableReader) Read(buf []byte) (int, error) {
	return _peekableRead(b, buf)
}

func (b *BufferedPeekableReader) GetReader() io.Reader {
	return b.Reader
}

func (b *BufferedPeekableReader) SetBuf(buf []byte) {
	b.buf = buf
}

func (b *BufferedPeekableReader) GetBuf() []byte {
	return b.buf
}

//func NewPeekableNetConn(r net.Conn) *PeekableNetConn {
//	return &PeekableNetConn{
//		Conn: r,
//	}
//}

//type PeekableNetConn struct {
//	net.Conn
//
//	buf []byte
//}
//
//func (p *PeekableNetConn) GetOriginConn() net.Conn {
//	return p.Conn
//}
//
//func (p *PeekableNetConn) Peek(i int) ([]byte, error) {
//	var buf = make([]byte, i)
//	l := len(p.buf)
//	if i <= l {
//		copy(buf, p.buf[:i])
//		return buf, nil
//	} else {
//		copy(buf, p.buf)
//		n, err := io.ReadFull(p.Conn, buf[l:])
//		p.buf = buf[:l+n]
//		return p.buf, err
//	}
//}
//
////
////func (p *PeekableNetConn) Write(b []byte) (int, error) {
////	return p.Conn.Write(b)
////}
//
//func (p *PeekableNetConn) Read(b []byte) (int, error) {
//	l := len(p.buf)
//	if l <= 0 {
//		return p.Conn.Read(b)
//	}
//	rl := len(b)
//	if rl <= l {
//		copy(b, p.buf[:rl])
//		p.buf = p.buf[rl:]
//		return rl, nil
//	}
//	if l > 0 {
//		n := copy(b, p.buf)
//		p.buf = p.buf[n:]
//		return n, nil
//	}
//	return p.Conn.Read(b)
//}
//
//func (p *PeekableNetConn) ReadN(length int) ([]byte, error) {
//	var b = make([]byte, length)
//	n, err := io.ReadFull(p, b)
//	return b[:n], err
//}
