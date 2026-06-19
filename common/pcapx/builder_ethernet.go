package pcapx

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

var ethernetLayerExports = map[string]interface{}{
	"ethernet_srcMac":        WithEthernet_SrcMac,
	"ethernet_dstMac":        WithEthernet_DstMac,
	"ethernet_nextLayerType": WithEthernet_NextLayerType,
}

func init() {
	for k, v := range ethernetLayerExports {
		Exports[k] = v
	}
}

type EthernetOption func(config *layers.Ethernet) error

// ethernet_srcMac 设置以太网帧的源 MAC 地址
// 在 yak 中通过 pcapx.ethernet_srcMac 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - srcMac: 源 MAC，可为 "00:11:22:33:44:55" 字符串或 net.HardwareAddr
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的以太网层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造带源 MAC 的以太网帧
// raw = pcapx.PacketBuilder(pcapx.ethernet_srcMac("00:11:22:33:44:55"))~
// println(len(raw))
// ```
func WithEthernet_SrcMac(srcMac any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := srcMac.(type) {
		case net.HardwareAddr:
			config.SrcMAC = ret
			return nil
		default:
			var err error
			config.SrcMAC, err = net.ParseMAC(utils.InterfaceToString(ret))
			if err != nil {
				return utils.Errorf("parse %v to mac failed: %s", ret, err)
			}
			return nil
		}
	}
}

// ethernet_dstMac 设置以太网帧的目的 MAC 地址
// 在 yak 中通过 pcapx.ethernet_dstMac 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - dstMac: 目的 MAC，可为 "00:11:22:33:44:55" 字符串或 net.HardwareAddr
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的以太网层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造带目的 MAC 的以太网帧
// raw = pcapx.PacketBuilder(pcapx.ethernet_dstMac("ff:ff:ff:ff:ff:ff"))~
// println(len(raw))
// ```
func WithEthernet_DstMac(dstMac any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := dstMac.(type) {
		case net.HardwareAddr:
			config.DstMAC = ret
			return nil
		default:
			var err error
			config.DstMAC, err = net.ParseMAC(utils.InterfaceToString(ret))
			if err != nil {
				return utils.Errorf("parse %v to mac failed: %s", ret, err)
			}
			return nil
		}
	}
}

// ethernet_nextLayerType 设置以太网帧的上层协议类型(EtherType)
// 在 yak 中通过 pcapx.ethernet_nextLayerType 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 上层协议类型，如 "ipv4"、"ipv6"、"arp"、"mpls"、"pppoe" 或数字
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的以太网层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定以太网上层为 ARP
// raw = pcapx.PacketBuilder(pcapx.ethernet_nextLayerType("arp"))~
// println(len(raw))
// ```
func WithEthernet_NextLayerType(i any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := strings.ToLower(utils.InterfaceToString(i)); ret {
		case "ipv4", "ip4", "ip":
			config.EthernetType = layers.EthernetTypeIPv4
		case "ipv6", "ip6":
			config.EthernetType = layers.EthernetTypeIPv6
		case "arp":
			config.EthernetType = layers.EthernetTypeARP
		case "mpls":
			config.EthernetType = layers.EthernetTypeMPLSUnicast
		case "pppoe":
			config.EthernetType = layers.EthernetTypePPPoEDiscovery
		case "pppoesession":
			config.EthernetType = layers.EthernetTypePPPoESession
		case "dot1q":
			config.EthernetType = layers.EthernetTypeDot1Q
		default:
			config.EthernetType = layers.EthernetType(codec.Atoi(ret))
		}
		return nil
	}
}
