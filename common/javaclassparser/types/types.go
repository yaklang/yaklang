package types

type ClassReader interface {
	ReadUint8() uint8
	ReadUint16() uint16
	ReadUint32() uint32
	ReadUint64() uint64
	ReadUint16s() []uint16
	ReadBytes(length uint32) []byte
}

type ClassWriter interface {
	Write1Byte(value uint8)
	Write2Byte(value uint16)
	Write4Byte(value uint32)
	Write8Byte(value uint64)
	WriteBytes(value []byte)
}
