package netstackvm

import (
	"bytes"
	"net"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
)

// pcapBridge 用于在虚拟机网络和物理网络之间桥接数据包
// 它负责处理MAC地址的转换，确保数据包能够正确地在两个网络之间传递
type pcapBridge struct {
	// external 是真实物理网卡的硬件地址(MAC地址)
	// 用作发送到物理网络的数据包的源MAC地址
	external net.HardwareAddr

	// internal 是虚拟机网卡的MAC地址
	// 用作发送到虚拟机的数据包的源MAC地址
	internal net.HardwareAddr
}

// handleOutbound 处理从虚拟机发出到物理网络的以太网数据包
// 将源MAC地址修改为物理网卡的MAC地址，以确保外部网络可以正确响应
func (p *pcapBridge) handleOutbound(eth *layers.Ethernet) *layers.Ethernet {
	eth.SrcMAC = p.external
	return eth
}

// handleInbound 处理从物理网络发往虚拟机的以太网数据包
// 如果目标MAC地址是虚拟机的MAC地址，则将源MAC地址修改为物理网卡的MAC地址
func (p *pcapBridge) handleInbound(eth *layers.Ethernet) *layers.Ethernet {
	if bytes.Equal(eth.DstMAC, p.internal) {
		eth.SrcMAC = p.external
	}
	return eth
}

// handleOutboundARP 处理从虚拟机发出的ARP数据包
// 如果发送方硬件地址是虚拟机的MAC地址，则将其替换为物理网卡的MAC地址
// 这确保ARP响应能够正确到达虚拟机
func (p *pcapBridge) handleOutboundARP(pkt header.ARP) header.ARP {
	if bytes.Equal(pkt.HardwareAddressSender(), p.internal) {
		copy(pkt.HardwareAddressSender(), p.external)
	}
	return pkt
}
