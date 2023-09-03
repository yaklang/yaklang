package wsm

type PacketCodecI interface {
	// ClientRequestEncode 对请求包的 payload 进行编码
	ClientRequestEncode(raw []byte) ([]byte, error)
	// ServerResponseDecode webshell server 获取请求包中的 payload
	ServerResponseDecode(raw []byte) ([]byte, error)
	SetPacketScriptContent(content string)
}
