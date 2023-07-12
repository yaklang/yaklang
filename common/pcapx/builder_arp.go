package pcapx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

var arpLayerExports = map[string]interface{}{
	"arp_question":    WithArpLayerBuilderConfigRequest,
	"arp_answer":      WithArpLayerBuilderConfigResponse,
	"arp_source":      WithArpLayerBuilderConfigSource,
	"arp_destination": WithArpLayerBuilderConfigDestination,
}

type ArpLayerBuilderConfig struct {
	AddrType        layers.LinkType
	Protocol        layers.EthernetType
	HwAddressSize   uint8
	ProtAddressSize uint8

	// req-1
	// rsp-0
	Operation             uint16
	SourceHwAddress       []byte
	SourceProtocolAddress []byte
	DstHwAddress          []byte
	DstProtocolAddress    []byte
}

type ArpLayerBuilderConfigOption func(*ArpLayerBuilderConfig)

func ArpLayerBuilderConfigAddrType(addrType layers.LinkType) ArpLayerBuilderConfigOption {
	return func(a *ArpLayerBuilderConfig) {
		a.AddrType = addrType
	}
}

func ArpLayerBuilderConfigProtocolTypeRaw(protocol any) ArpLayerBuilderConfigOption {
	switch ret := strings.ToLower(utils.InterfaceToString(protocol)); ret {
	case "ipv4", "ip", "ip4":
		return func(a *ArpLayerBuilderConfig) {
			a.Protocol = layers.EthernetTypeIPv4
			a.HwAddressSize = 6
			a.ProtAddressSize = 4
		}
	case "ipv6", "ip6":
		return func(a *ArpLayerBuilderConfig) {
			a.Protocol = layers.EthernetTypeIPv6
			a.HwAddressSize = 6
			a.ProtAddressSize = 16
		}
	default:
		return func(a *ArpLayerBuilderConfig) {
			a.Protocol = layers.EthernetType(utils.Atoi(ret))
			a.HwAddressSize = 6
		}
	}
}

func ArpLayerBUilderConfigOperation(operation uint16) ArpLayerBuilderConfigOption {
	return func(a *ArpLayerBuilderConfig) {
		a.Operation = operation
	}
}

func WithArpLayerBuilderConfigProtocol(protocol layers.EthernetType) ArpLayerBuilderConfigOption {
	return func(a *ArpLayerBuilderConfig) {
		a.Protocol = protocol
	}
}

func WithArpLayerBuilderConfigRequest() ArpLayerBuilderConfigOption {
	return func(config *ArpLayerBuilderConfig) {
		config.Operation = 1
	}
}

func WithArpLayerBuilderConfigResponse() ArpLayerBuilderConfigOption {
	return func(config *ArpLayerBuilderConfig) {
		config.Operation = 0
	}
}

func WithArpLayerBuilderConfigSource(protocolAddr any, hardwareAddr any) ArpLayerBuilderConfigOption {
	ipAddr := net.ParseIP(utils.InterfaceToString(protocolAddr))
	hwAddr, err := net.ParseMAC(utils.InterfaceToString(hardwareAddr))
	if err != nil {
		log.Warnf("parse mac failed: %s", err)
	}
	return func(config *ArpLayerBuilderConfig) {
		if ipAddr != nil {
			config.SourceProtocolAddress = ipAddr
		}
		if hwAddr != nil {
			config.SourceHwAddress = hwAddr
		} else {
			config.SourceHwAddress = make([]byte, 6)
		}
	}
}

func WithArpLayerBuilderConfigDestination(protocolAddr any, hardwareAddr any) ArpLayerBuilderConfigOption {
	ipAddr := net.ParseIP(utils.InterfaceToString(protocolAddr))
	hwAddr, err := net.ParseMAC(utils.InterfaceToString(hardwareAddr))
	if err != nil {
		log.Warnf("parse mac failed: %s", err)
	}
	return func(config *ArpLayerBuilderConfig) {
		if ipAddr != nil {
			config.DstProtocolAddress = ipAddr
		}
		if hwAddr != nil {
			config.DstHwAddress = hwAddr
		} else {
			config.DstHwAddress = make([]byte, 6)
		}
	}
}

func (a *ArpLayerBuilderConfig) Create() *layers.ARP {
	return &layers.ARP{
		AddrType:        layers.LinkTypeEthernet,
		Protocol:        layers.EthernetTypeIPv4,
		HwAddressSize:   6,
		ProtAddressSize: 4,
	}
}
