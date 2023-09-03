package wsm

type PayloadCodecI interface {
	// EchoResultEncode payload 内部对回显结果的编码
	EchoResultEncode(raw []byte) ([]byte, error)
	// EchoResultDecode 对 payload 回显结果的解码
	EchoResultDecode(raw []byte) ([]byte, error)
	SetPayloadScriptContent(content string)
}
