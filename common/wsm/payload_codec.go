package wsm

type PayloadCodecI interface {
	// EchoResultEncodeFormYak payload 内部对回显结果的编码，混合编程，执行 yaklang
	EchoResultEncodeFormYak(raw []byte) ([]byte, error)
	// EchoResultDecodeFormYak 对 payload 回显结果的解码
	EchoResultDecodeFormYak(raw []byte) ([]byte, error)
	SetPayloadScriptContent(content string)
}
