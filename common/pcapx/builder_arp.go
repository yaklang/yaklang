package pcapx

import (
	"errors"
	"github.com/gopacket/gopacket/layers"
	"net"
)

var arpLayerExports = map[string]interface{}{
	"arp_request":   WithArp_RequestAuto,
	"arp_requestEx": WithArp_RequestEx,
	"arp_replyEx":   WithArp_ReplyToEx,
	"arp_reply":     WithArp_ReplyTo,
}

func init() {
	for k, v := range arpLayerExports {
		Exports[k] = v
	}
}

type ArpConfig func(arp *layers.ARP) error

// arp_requestEx 构造一个完整指定源信息的 ARP 请求层
// 在 yak 中通过 pcapx.arp_requestEx 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - selfIP: 本机(源) IP 地址
//   - selfMac: 本机(源) MAC 地址
//   - remoteIP: 要查询的目标 IP 地址
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 ARP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造 ARP 请求
// raw = pcapx.PacketBuilder(pcapx.arp_requestEx("192.168.1.2", "00:11:22:33:44:55", "192.168.1.1"))~
// println(len(raw))
// ```
func WithArp_RequestEx(selfIP, selfMac string, remoteIP string) ArpConfig {
	local := []byte(net.ParseIP(selfIP).To4())
	srcMac, _ := net.ParseMAC(selfMac)
	dstIP := []byte(net.ParseIP(remoteIP).To4())
	return func(arp *layers.ARP) error {
		arp.HwAddressSize = 6
		arp.ProtAddressSize = 4
		arp.Operation = layers.ARPRequest
		arp.SourceProtAddress = local
		if arp.SourceProtAddress == nil {
			return errors.New("invalid source ip: " + string(local))
		}
		arp.SourceHwAddress = srcMac
		arp.DstHwAddress = make([]byte, 6)
		arp.DstProtAddress = dstIP
		if arp.DstProtAddress == nil {
			return errors.New("invalid destination ip: " + string(dstIP))
		}
		return nil
	}
}

// arp_request 构造一个 ARP 请求层，自动使用本机默认网卡的 IP 与 MAC 作为源
// 在 yak 中通过 pcapx.arp_request 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - ip: 要查询的目标 IP 地址
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 ARP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：自动构造 ARP 请求
// raw = pcapx.PacketBuilder(pcapx.arp_request("192.168.1.1"))~
// println(len(raw))
// ```
func WithArp_RequestAuto(ip string) ArpConfig {
	// where is ip?
	return func(arp *layers.ARP) error {
		iface, _, local, err := getPublicRoute()
		if err != nil {
			return err
		}
		srcMac := iface.HardwareAddr
		return WithArp_RequestEx(local.String(), srcMac.String(), ip)(arp)
	}
}

// arp_replyEx 构造一个完整指定源与目的信息的 ARP 应答层
// 在 yak 中通过 pcapx.arp_replyEx 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - srcTarget: 应答中声明的源 IP 地址
//   - srcMac: 应答中声明的源 MAC 地址
//   - targetIp: 目标(接收方) IP 地址
//   - targetMac: 目标(接收方) MAC 地址
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 ARP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造 ARP 应答
// raw = pcapx.PacketBuilder(pcapx.arp_replyEx("192.168.1.1", "00:11:22:33:44:55", "192.168.1.2", "66:77:88:99:aa:bb"))~
// println(len(raw))
// ```
func WithArp_ReplyToEx(srcTarget, srcMac, targetIp, targetMac string) ArpConfig {
	return func(arp *layers.ARP) error {
		arp.Operation = layers.ARPReply
		arp.SourceProtAddress = net.ParseIP(srcTarget)
		if arp.SourceProtAddress == nil {
			return errors.New("invalid source ip: " + srcTarget)
		}
		arp.SourceHwAddress, _ = net.ParseMAC(srcMac)
		arp.DstHwAddress, _ = net.ParseMAC(targetMac)
		arp.DstProtAddress = net.ParseIP(targetIp)
		if arp.DstProtAddress == nil {
			return errors.New("invalid destination ip: " + targetIp)
		}
		return nil
	}
}

// arp_reply 构造一个 ARP 应答层，自动使用本机默认网卡的 IP 与 MAC 作为源
// 在 yak 中通过 pcapx.arp_reply 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - targetIp: 目标(接收方) IP 地址
//   - targetMac: 目标(接收方) MAC 地址
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 ARP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：自动构造 ARP 应答
// raw = pcapx.PacketBuilder(pcapx.arp_reply("192.168.1.2", "66:77:88:99:aa:bb"))~
// println(len(raw))
// ```
func WithArp_ReplyTo(targetIp, targetMac string) ArpConfig {
	iface, _, local, err := getPublicRoute()
	if err != nil {
		return func(arp *layers.ARP) error {
			return err
		}
	}
	srcMac := iface.HardwareAddr
	return WithArp_ReplyToEx(local.String(), srcMac.String(), targetIp, targetMac)
}
