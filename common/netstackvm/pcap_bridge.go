package netstackvm

import (
	"bytes"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"net"
)

type pcapBridge struct {
	// external is the real device hardware address, used as source mac for packets sent to the network
	external net.HardwareAddr
	// internal device mac, used as source mac for packets sent to the VM
	internal net.HardwareAddr
}

func (p *pcapBridge) handleOutbound(eth *layers.Ethernet) *layers.Ethernet {
	eth.SrcMAC = p.external
	return eth
}

func (p *pcapBridge) handleInbound(eth *layers.Ethernet) *layers.Ethernet {
	if bytes.Equal(eth.DstMAC, p.internal) {
		eth.SrcMAC = p.external
	}
	return eth
}

func (p *pcapBridge) handleOutboundARP(pkt header.ARP) header.ARP {
	if bytes.Equal(pkt.HardwareAddressSender(), p.internal) {
		copy(pkt.HardwareAddressSender(), p.external)
	}
	return pkt
}
