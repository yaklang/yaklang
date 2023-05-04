package yserx

type JavaSerializationStreamer struct {
	handle uint64

	objects []JavaSerializable
}

func NewJavaSerializationStreamer() *JavaSerializationStreamer {
	return &JavaSerializationStreamer{handle: 0x007e0000}
}
