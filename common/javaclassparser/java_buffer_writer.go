package javaclassparser

import (
	"bytes"
	"yaklang/common/yak/yaklib/codec"
)

type JavaBufferWriter struct {
	data *bytes.Buffer
}

func NewJavaBufferWrite() *JavaBufferWriter {
	var data bytes.Buffer
	return &JavaBufferWriter{data: &data}
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

func (j *JavaBufferWriter) Write8Byte(v interface{}) error {
	value, err := Interface2Uint64(v)
	if err != nil {
		return ValueTypeError
	}
	var buf = make([]byte, 8)
	buf[0] = byte(value >> 56)
	buf[1] = byte(value >> 48)
	buf[2] = byte(value >> 40)
	buf[3] = byte(value >> 32)
	buf[4] = byte(value >> 24)
	buf[5] = byte(value >> 16)
	buf[6] = byte(value >> 8)
	buf[7] = byte(value)
	j.data.Write(buf)
	return nil
}
func (j *JavaBufferWriter) Write4Byte(v interface{}) error {
	value, err := Interface2Uint64(v)
	if err != nil {
		return ValueTypeError
	}
	var buf = make([]byte, 4)
	buf[0] = byte(value >> 24)
	buf[1] = byte(value >> 16)
	buf[2] = byte(value >> 8)
	buf[3] = byte(value)
	j.data.Write(buf)
	return nil
}
func (j *JavaBufferWriter) Write2Byte(v interface{}) error {
	value, err := Interface2Uint64(v)
	if err != nil {
		return ValueTypeError
	}
	var buf = make([]byte, 2)
	buf[0] = byte(value >> 8)
	buf[1] = byte(value)
	j.data.Write(buf)
	return nil
}
func (j *JavaBufferWriter) Write1Byte(v interface{}) error {
	value, err := Interface2Uint64(v)
	if err != nil {
		return ValueTypeError
	}
	var buf = make([]byte, 1)
	buf[0] = byte(value)
	j.data.Write(buf)
	return nil
}

func (j *JavaBufferWriter) WriteString(v string) error {
	j.Write2Byte(len(v))
	j.data.Write([]byte(v))
	return nil
}

func (j *JavaBufferWriter) WriteLString(v string) error {
	j.Write4Byte(len(v))
	j.data.Write([]byte(v))
	return nil
}
func (j *JavaBufferWriter) Write(v []byte) error {
	j.data.Write(v)
	return nil
}
func (j *JavaBufferWriter) Bytes() []byte {
	return j.data.Bytes()
}
