package pcapx

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
)

var loopbackLayerExports = map[string]interface{}{
	"loopback_payload": WithLoopback_Payload,
	"loopback_family":  WithLoopback_Family,
}

func init() {
	for k, v := range loopbackLayerExports {
		Exports[k] = v
	}
}

type LoopbackOption func(config *layers.Loopback) error

// loopback_payload 设置 loopback(回环)层所承载的负载数据
// 在 yak 中通过 pcapx.loopback_payload 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - payload: 负载字节数据
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 loopback 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 loopback 负载
// raw = pcapx.PacketBuilder(pcapx.loopback_payload([]byte("data")))~
// println(len(raw))
// ```
func WithLoopback_Payload(payload []byte) LoopbackOption {
	return func(config *layers.Loopback) error {
		config.Payload = payload
		return nil
	}
}

// loopback_family 设置 loopback(回环)层的协议族(ProtocolFamily)
// 在 yak 中通过 pcapx.loopback_family 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 协议族，取 layers.ProtocolFamily 类型值
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 loopback 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 loopback 协议族
// raw = pcapx.PacketBuilder(pcapx.loopback_payload([]byte("data")))~
// println(len(raw))
// ```
func WithLoopback_Family(i any) LoopbackOption {
	return func(config *layers.Loopback) error {
		switch ret := i.(type) {
		case layers.ProtocolFamily:
			config.Family = ret
			return nil
		default:
			return utils.Errorf("invalid link type: %v", i)
		}
	}
}
