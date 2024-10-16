package core

import (
	"bytes"
	"io"
)

type JavaByteCodeReader struct {
	reader     *bytes.Reader
	CurrentPos int
}

func (j *JavaByteCodeReader) ReadByte() (b byte, err error) {
	defer func() {
		j.CurrentPos++
	}()
	return j.reader.ReadByte()
}
func (j *JavaByteCodeReader) Read(p []byte) (n int, err error) {
	n, err = j.reader.Read(p)
	j.CurrentPos += n
	return
}

func (j *JavaByteCodeReader) Read2ByteInt() uint16 {
	b := make([]byte, 2)
	j.Read(b)
	return uint16(b[0])<<8 | uint16(b[1])
}

func (j *JavaByteCodeReader) Read4ByteInt() uint32 {
	b := make([]byte, 4)
	j.Read(b)
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

var _ io.Reader = (*JavaByteCodeReader)(nil)

func NewJavaByteCodeReader(data []byte) *JavaByteCodeReader {
	return &JavaByteCodeReader{reader: bytes.NewReader(data)}
}
