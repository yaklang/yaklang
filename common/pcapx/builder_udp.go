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

func WithUDP_SrcPort(in any) UDPOption {
	return func(config *layers.UDP) error {
		config.SrcPort = layers.UDPPort(utils.InterfaceToInt(in))
		return nil
	}
}

func WithUDP_DstPort(in any) UDPOption {
	return func(config *layers.UDP) error {
		config.DstPort = layers.UDPPort(utils.InterfaceToInt(in))
		return nil
	}
}
