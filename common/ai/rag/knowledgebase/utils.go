package knowledgebase

import (
	"io"

	"github.com/yaklang/yaklang/common/utils"
)

// 为了保持向后兼容，保留原有的函数名，但内部使用 utils.ProtoReader 和 utils.ProtoWriter

// 创建流式读取辅助函数
func consumeUint32(reader io.Reader) (uint32, error) {
	pr := utils.NewProtoReader(reader)
	return pr.ReadUint32()
}

func consumeFloat32(reader io.Reader) (float32, error) {
	pr := utils.NewProtoReader(reader)
	return pr.ReadFloat32()
}

func consumeVarint(reader io.Reader) (uint64, error) {
	pr := utils.NewProtoReader(reader)
	return pr.ReadVarint()
}

func consumeBytes(reader io.Reader) ([]byte, error) {
	pr := utils.NewProtoReader(reader)
	return pr.ReadBytes()
}

func consumeBool(reader io.Reader) (bool, error) {
	pr := utils.NewProtoReader(reader)
	return pr.ReadBool()
}

// protowire辅助函数
func pbWriteVarint(w io.Writer, value uint64) error {
	pw := utils.NewProtoWriter(w)
	return pw.WriteVarint(value)
}

func pbWriteBytes(w io.Writer, value []byte) error {
	pw := utils.NewProtoWriter(w)
	return pw.WriteBytes(value)
}

func pbWriteUint32(w io.Writer, value uint32) error {
	pw := utils.NewProtoWriter(w)
	return pw.WriteUint32(value)
}

func pbWriteFloat32(w io.Writer, value float32) error {
	pw := utils.NewProtoWriter(w)
	return pw.WriteFloat32(value)
}

func pbWriteBool(w io.Writer, value bool) error {
	pw := utils.NewProtoWriter(w)
	return pw.WriteBool(value)
}
