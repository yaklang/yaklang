package pcaputil

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func IsTCP(packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}

	layer := packet.TransportLayer()
	if layer == nil {
		return false
	}
	if layer.LayerType() == layers.LayerTypeTCP {
		return true
	}

	return false
}

func IsUDP(packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}

	layer := packet.TransportLayer()
	if layer == nil {
		return false
	}
	if layer.LayerType() == layers.LayerTypeUDP {
		return true
	}

	return false
}

func LinkLayerName(packet gopacket.Packet) string {
	if l := packet.LinkLayer(); l != nil {
		switch l.LayerType() {
		case layers.LayerTypeEthernet:
			arpL := packet.Layer(layers.LayerTypeARP)
			if arpL != nil {
				return arpL.LayerType().String()
			}
		}
		return l.LayerType().String()
	}
	return ""
}

func NetworkLayerName(packet gopacket.Packet) string {
	if l := packet.NetworkLayer(); l != nil {
		switch l.LayerType() {
		case layers.LayerTypeIPv4:
			icmpLayer := packet.Layer(layers.LayerTypeICMPv4)
			if icmpLayer != nil {
				return icmpLayer.LayerType().String()
			}

			igmpLayer := packet.Layer(layers.LayerTypeIGMP)
			if igmpLayer != nil {
				return igmpLayer.LayerType().String()
			}
		case layers.LayerTypeIPv6:
			icmpLayer := packet.Layer(layers.LayerTypeICMPv6)
			if icmpLayer != nil {
				return icmpLayer.LayerType().String()
			}
		}
		return l.LayerType().String()
	}

	return ""
}

func TransportLayerName(packet gopacket.Packet) string {
	if l := packet.TransportLayer(); l != nil {
		return l.LayerType().String()
	}
	return ""
}

func ApplicationLayerName(packet gopacket.Packet) string {
	if l := packet.ApplicationLayer(); l != nil {
		if ret := l.LayerType().String(); ret == "Payload" {
			return ""
		} else {
			return ret
		}
	}
	return ""
}

func IsICMP(packet gopacket.Packet) (ret bool) {
	defer func() {
		if err := recover(); err != nil {
			ret = false
		}
	}()
	if l := packet.NetworkLayer(); l != nil {
		if l.LayerType() == layers.LayerTypeICMPv4 || l.LayerType() == layers.LayerTypeICMPv6 {
			ret = true
			return
		}
		if l.LayerType() == layers.LayerTypeIPv4 && l.(*layers.IPv4).Protocol == layers.IPProtocolICMPv4 {
			ret = true
			return
		}
		if l.LayerType() == layers.LayerTypeIPv6 && l.(*layers.IPv6).NextHeader == layers.IPProtocolICMPv6 {
			ret = true
			return
		}
	}
	return
}
