package netstackvm

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func (vm *NetStackVirtualMachineEntry) SetFilterForSynScan() {
	vm.driver.SetPCAPInboundFilter(func(packet gopacket.Packet) bool {
		tcpLayerRaw := packet.Layer(layers.LayerTypeTCP)
		if tcpLayerRaw == nil {
			return false
		}
		tcpLayer, ok := tcpLayerRaw.(*layers.TCP)
		if !ok {
			return false
		}
		if tcpLayer.ACK && tcpLayer.SYN {
			return true
		}
		return false
	})
	vm.driver.SetPCAPOutboundFilter(func(packet gopacket.Packet) bool {
		tcpLayerRaw := packet.Layer(layers.LayerTypeTCP)
		if tcpLayerRaw == nil {
			return false
		}
		tcpLayer, ok := tcpLayerRaw.(*layers.TCP)
		if !ok {
			return false
		}
		if tcpLayer.SYN {
			return true
		}
		return false
	})
}
