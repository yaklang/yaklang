package javaclassparser

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type JavaBufferWriter struct {
	data       *bytes.Buffer
	charLength int
}

func NewJavaBufferWrite() *JavaBufferWriter {
	var data bytes.Buffer
	return &JavaBufferWriter{data: &data, charLength: 1}
}

func (j *JavaBufferWriter) WriteHex(v interface{}) error {
	switch v.(type) {
	case string:
		byts, err := codec.DecodeHex(v.(string))
		if err != nil {
			return err
		}
		j.data.Write(byts)
		return nil
	case []byte:
		j.data.Write(v.([]byte))
		return nil
	default:
		return ValueTypeError
	}
}

func (j *JavaBufferWriter) Write8Byte(v uint64) {
	var buf = make([]byte, 8)
	buf[0] = byte(v >> 56)
	buf[1] = byte(v >> 48)
	buf[2] = byte(v >> 40)
	buf[3] = byte(v >> 32)
	buf[4] = byte(v >> 24)
	buf[5] = byte(v >> 16)
	buf[6] = byte(v >> 8)
	buf[7] = byte(v)
	j.data.Write(buf)
}
func (j *JavaBufferWriter) Write4Byte(v uint32) {
	var buf = make([]byte, 4)
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
	j.data.Write(buf)
}
func (j *JavaBufferWriter) Write2Byte(v uint16) {
	var buf = make([]byte, 2)
	buf[0] = byte(v >> 8)
	buf[1] = byte(v)
	j.data.Write(buf)
}
func (j *JavaBufferWriter) Write1Byte(v uint8) {
	var buf = make([]byte, 1)
	buf[0] = v
	j.data.Write(buf)
}

func (j *JavaBufferWriter) WriteBytes(v []byte) {
	j.data.Write(v)
}

func (j *JavaBufferWriter) WriteString(v string) {
	//bs := utils.ToJavaOverLongString([]byte(v), j.charLength)
	bs := []byte(v)
	j.Write2Byte(uint16(len(bs)))
	j.data.Write(bs)
}

func (j *JavaBufferWriter) WriteLString(v string) {
	j.Write4Byte(uint32(len(v)))
	j.data.Write([]byte(v))
}
func (j *JavaBufferWriter) Write(v []byte) {
	j.data.Write(v)
}
func (j *JavaBufferWriter) Bytes() []byte {
	return j.data.Bytes()
}
