package pcapx

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
)

var udpOptions = map[string]any{
	"udp_srcPort": WithUDP_SrcPort,
	"udp_dstPort": WithUDP_DstPort,
}

func init() {
	for k, v := range udpOptions {
		Exports[k] = v
	}
}

type UDPOption func(config *layers.UDP) error

// udp_srcPort 设置 UDP 头部的源端口
// 在 yak 中通过 pcapx.udp_srcPort 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - in: 源端口号(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 UDP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 UDP 源端口
// raw = pcapx.PacketBuilder(pcapx.udp_srcPort(12345), pcapx.udp_dstPort(53))~
// println(len(raw))
// ```
func WithUDP_SrcPort(in any) UDPOption {
	return func(config *layers.UDP) error {
		config.SrcPort = layers.UDPPort(utils.InterfaceToInt(in))
		return nil
	}
}

// udp_dstPort 设置 UDP 头部的目的端口
// 在 yak 中通过 pcapx.udp_dstPort 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - in: 目的端口号(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 UDP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 UDP 目的端口
// raw = pcapx.PacketBuilder(pcapx.udp_srcPort(12345), pcapx.udp_dstPort(53))~
// println(len(raw))
// ```
func WithUDP_DstPort(in any) UDPOption {
	return func(config *layers.UDP) error {
		config.DstPort = layers.UDPPort(utils.InterfaceToInt(in))
		return nil
	}
}
