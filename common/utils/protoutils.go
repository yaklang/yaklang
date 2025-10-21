package utils

import (
	"io"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// ProtoReader 提供从 io.Reader 读取 protobuf 编码数据的方法
type ProtoReader struct {
	reader io.Reader
}

// NewProtoReader 创建一个新的 ProtoReader
func NewProtoReader(reader io.Reader) *ProtoReader {
	return &ProtoReader{reader: reader}
}

// ReadUint32 读取一个 uint32 值
func (pr *ProtoReader) ReadUint32() (uint32, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(pr.reader, buf); err != nil {
		return 0, Wrap(err, "read uint32")
	}
	v, n := protowire.ConsumeFixed32(buf)
	if n < 0 {
		return 0, Errorf("consume fixed32: %d", n)
	}
	return v, nil
}

// ReadFloat32 读取一个 float32 值
func (pr *ProtoReader) ReadFloat32() (float32, error) {
	v, err := pr.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

// ReadVarint 读取一个 varint 值
func (pr *ProtoReader) ReadVarint() (uint64, error) {
	// 变长编码最多10字节
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		if _, err := io.ReadFull(pr.reader, buf[i:i+1]); err != nil {
			return 0, Wrap(err, "read varint byte")
		}

		// 检查是否完整
		v, n := protowire.ConsumeVarint(buf[:i+1])
		if n > 0 {
			return v, nil
		}
	}
	return 0, Error("invalid varint encoding")
}

// ReadBytes 读取字节数组
func (pr *ProtoReader) ReadBytes() ([]byte, error) {
	length, err := pr.ReadVarint()
	if err != nil {
		return nil, Wrap(err, "read bytes length")
	}
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(pr.reader, buf); err != nil {
		return nil, Wrap(err, "read bytes data")
	}
	return buf, nil
}

// ReadString 读取字符串
func (pr *ProtoReader) ReadString() (string, error) {
	bytes, err := pr.ReadBytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ReadBool 读取布尔值
func (pr *ProtoReader) ReadBool() (bool, error) {
	b, err := pr.ReadVarint()
	if err != nil {
		return false, Wrap(err, "read bool")
	}
	return b != 0, nil
}

// ReadInt32 读取 int32 值
func (pr *ProtoReader) ReadInt32() (int32, error) {
	v, err := pr.ReadVarint()
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

// ReadInt64 读取 int64 值
func (pr *ProtoReader) ReadInt64() (int64, error) {
	v, err := pr.ReadVarint()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// ProtoWriter 提供向 io.Writer 写入 protobuf 编码数据的方法
type ProtoWriter struct {
	writer io.Writer
}

// NewProtoWriter 创建一个新的 ProtoWriter
func NewProtoWriter(writer io.Writer) *ProtoWriter {
	return &ProtoWriter{writer: writer}
}

// WriteVarint 写入一个 varint 值
func (pw *ProtoWriter) WriteVarint(value uint64) error {
	var buf []byte
	buf = protowire.AppendVarint(buf, value)
	_, err := pw.writer.Write(buf)
	return err
}

// WriteBytes 写入字节数组
func (pw *ProtoWriter) WriteBytes(value []byte) error {
	var buf []byte
	buf = protowire.AppendBytes(buf, value)
	_, err := pw.writer.Write(buf)
	return err
}

// WriteString 写入字符串
func (pw *ProtoWriter) WriteString(value string) error {
	return pw.WriteBytes([]byte(value))
}

// WriteUint32 写入 uint32 值
func (pw *ProtoWriter) WriteUint32(value uint32) error {
	var buf []byte
	buf = protowire.AppendFixed32(buf, value)
	_, err := pw.writer.Write(buf)
	return err
}

// WriteFloat32 写入 float32 值
func (pw *ProtoWriter) WriteFloat32(value float32) error {
	var buf []byte
	buf = protowire.AppendFixed32(buf, math.Float32bits(value))
	_, err := pw.writer.Write(buf)
	return err
}

// WriteBool 写入布尔值
func (pw *ProtoWriter) WriteBool(value bool) error {
	var buf []byte
	if value {
		buf = protowire.AppendVarint(buf, uint64(1))
	} else {
		buf = protowire.AppendVarint(buf, uint64(0))
	}
	_, err := pw.writer.Write(buf)
	return err
}

// WriteInt32 写入 int32 值
func (pw *ProtoWriter) WriteInt32(value int32) error {
	return pw.WriteVarint(uint64(value))
}

// WriteInt64 写入 int64 值
func (pw *ProtoWriter) WriteInt64(value int64) error {
	return pw.WriteVarint(uint64(value))
}

// WriteMagicHeader 写入魔数头（固定16字节）
func (pw *ProtoWriter) WriteMagicHeader(magic string) error {
	if len(magic) != 16 {
		return Errorf("magic header must be exactly 16 bytes, got %d", len(magic))
	}
	_, err := pw.writer.Write([]byte(magic))
	return err
}

// ReadMagicHeader 读取并验证魔数头（固定16字节）
func (pr *ProtoReader) ReadMagicHeader(expectedMagic string) error {
	if len(expectedMagic) != 16 {
		return Errorf("magic header must be exactly 16 bytes, got %d", len(expectedMagic))
	}
	magic := make([]byte, 16)
	if _, err := io.ReadFull(pr.reader, magic); err != nil {
		return Wrap(err, "read magic header")
	}
	if string(magic) != expectedMagic {
		return Errorf("invalid magic header: expected %q, got %q", expectedMagic, string(magic))
	}
	return nil
}
