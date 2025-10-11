package knowledgebase

import (
	"io"
	"math"

	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/protobuf/encoding/protowire"
)

// 创建流式读取辅助函数
func consumeUint32(reader io.Reader) (uint32, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return 0, utils.Wrap(err, "read uint32")
	}
	v, n := protowire.ConsumeFixed32(buf)
	if n < 0 {
		return 0, utils.Errorf("consume fixed32: %d", n)
	}
	return v, nil
}

func consumeFloat32(reader io.Reader) (float32, error) {
	v, err := consumeUint32(reader)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func consumeVarint(reader io.Reader) (uint64, error) {
	// 变长编码最多10字节
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		if _, err := io.ReadFull(reader, buf[i:i+1]); err != nil {
			return 0, utils.Wrap(err, "read varint byte")
		}

		// 检查是否完整
		v, n := protowire.ConsumeVarint(buf[:i+1])
		if n > 0 {
			return v, nil
		}
	}
	return 0, utils.Error("invalid varint encoding")
}

func consumeBytes(reader io.Reader) ([]byte, error) {
	length, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read bytes length")
	}
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, utils.Wrap(err, "read bytes data")
	}
	return buf, nil
}

func consumeBool(reader io.Reader) (bool, error) {
	b, err := consumeVarint(reader)
	if err != nil {
		return false, utils.Wrap(err, "read bool")
	}
	return b != 0, nil
}

// protowire辅助函数
func pbWriteVarint(w io.Writer, value uint64) error {
	var i []byte
	i = protowire.AppendVarint(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteBytes(w io.Writer, value []byte) error {
	var i []byte
	i = protowire.AppendBytes(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteUint32(w io.Writer, value uint32) error {
	var i []byte
	i = protowire.AppendFixed32(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteFloat32(w io.Writer, value float32) error {
	var i []byte
	i = protowire.AppendFixed32(i, math.Float32bits(value))
	_, err := w.Write(i)
	return err
}

func pbWriteBool(w io.Writer, value bool) error {
	var i []byte
	if value {
		i = protowire.AppendVarint(i, uint64(1))
	} else {
		i = protowire.AppendVarint(i, uint64(0))
	}
	_, err := w.Write(i)
	return err
}
