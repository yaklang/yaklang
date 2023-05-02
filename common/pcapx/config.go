package pcapx

import (
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"sync"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	ifaceHandler sync.Map
)

type Config struct {
	Iface       string
	ToServerSet bool
	ToServer    bool
	IsHttps     bool

	TCPLocalAddress  string
	TCPRemoteAddress string
}

type ConfigOption func(config *Config)

func WithIface(iface string) ConfigOption {
	return func(config *Config) {
		config.Iface = iface
	}
}

func WithLocalAddress(addr string) ConfigOption {
	return func(config *Config) {
		config.TCPLocalAddress = addr
	}
}

func WithRemoteAddress(addr string) ConfigOption {
	return func(config *Config) {
		config.TCPRemoteAddress = addr
	}
}

func WithToServer() ConfigOption {
	return func(config *Config) {
		config.ToServerSet = true
		config.ToServer = true
	}
}

func WithToClient() ConfigOption {
	return func(config *Config) {
		config.ToServerSet = true
		config.ToServer = false
	}
}

func ParseTCPRaw(raw []byte) (*layers.IPv4, *layers.TCP, gopacket.Payload, error) {
	var ipv4 *layers.IPv4
	var ok bool
	var tcp *layers.TCP

	packet := gopacket.NewPacket(raw, layers.LayerTypeEtherIP, gopacket.Default)

	transportLayer := packet.TransportLayer()
	ipLayer := packet.NetworkLayer()
	tcp, ok = transportLayer.(*layers.TCP)

	if !ok {
		log.Error("Not a TCP packet")
		return nil, nil, nil, errors.New("Not a TCP packet")
	}

	if ipLayer.LayerType() == layers.LayerTypeIPv4 {
		ipv4, ok = ipLayer.(*layers.IPv4)
		if !ok {
			return nil, nil, nil, utils.Error("parse ipv4 layer failed")
		}
	}

	var payload gopacket.Payload
	if ret := packet.TransportLayer(); ret.LayerType() == layers.LayerTypeTCP {
		tcp, ok = ret.(*layers.TCP)
		if !ok {
			return nil, nil, nil, utils.Error("parse tcp layer failed")
		}
		payload = ret.LayerPayload()
	}
	return ipv4, tcp, payload, nil
}

func ParseTCPIPv4(raw []byte) (*layers.IPv4, *layers.TCP, gopacket.Payload, error) {
	packet := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	ipLayer := packet.NetworkLayer()
	var ipv4 *layers.IPv4
	var ok bool
	if ipLayer.LayerType() == layers.LayerTypeIPv4 {
		ipv4, ok = ipLayer.(*layers.IPv4)
		if !ok {
			return nil, nil, nil, utils.Error("parse ipv4 layer failed")
		}
	}
	var tcp *layers.TCP
	var payload gopacket.Payload
	if ret := packet.TransportLayer(); ret.LayerType() == layers.LayerTypeTCP {
		tcp, ok = ret.(*layers.TCP)
		if !ok {
			return nil, nil, nil, utils.Error("parse tcp layer failed")
		}
		payload = ret.LayerPayload()
	}
	return ipv4, tcp, payload, nil
}

func ParseUDPIPv4(raw []byte) (*layers.IPv4, *layers.UDP, gopacket.Payload, error) {
	packet := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	ipLayer := packet.NetworkLayer()
	var ipv4 *layers.IPv4
	var ok bool
	if ipLayer.LayerType() == layers.LayerTypeIPv4 {
		ipv4, ok = ipLayer.(*layers.IPv4)
		if !ok {
			return nil, nil, nil, utils.Error("parse ipv4 layer failed")
		}
	}
	var udp *layers.UDP
	var payload gopacket.Payload
	if ret := packet.TransportLayer(); ret.LayerType() == layers.LayerTypeUDP {
		udp, ok = ret.(*layers.UDP)
		if !ok {
			return nil, nil, nil, utils.Error("parse udp layer failed")
		}
		payload = ret.LayerPayload()
	}
	return ipv4, udp, payload, nil
}

func ParseICMPIPv4(raw []byte) (*layers.IPv4, *layers.ICMPv4, gopacket.Payload, error) {
	packet := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	ipLayer := packet.NetworkLayer()
	var ipv4 *layers.IPv4
	var ok bool
	if ipLayer.LayerType() == layers.LayerTypeIPv4 {
		ipv4, ok = ipLayer.(*layers.IPv4)
		if !ok {
			return nil, nil, nil, utils.Error("parse ipv4 layer failed")
		}
	}
	var icmp *layers.ICMPv4
	var payload gopacket.Payload

	icmpLayer := packet.Layer(layers.LayerTypeICMPv4)
	if icmpLayer != nil {
		icmp, ok = icmpLayer.(*layers.ICMPv4)
		if ok {
			payload = icmpLayer.LayerPayload()
		} else {
			return nil, nil, nil, utils.Error("parse icmp layer failed")
		}
	}
	return ipv4, icmp, payload, nil
}

func ParseLinkLayer(raw []byte) (*layers.Ethernet, *layers.IPv4, *layers.TCP, error) {
	packet := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	var ok bool
	linkLayer := packet.LinkLayer()
	var ethernet *layers.Ethernet
	if linkLayer.LayerType() == layers.LayerTypeEthernet {
		ethernet, ok = linkLayer.(*layers.Ethernet)
		if !ok {
			return nil, nil, nil, utils.Error("parse ethernet layer failed")
		}
		return ethernet, nil, nil, nil
	}

	ipLayer := packet.NetworkLayer()
	var ipv4 *layers.IPv4
	if ipLayer.LayerType() == layers.LayerTypeIPv4 {
		ipv4, ok = ipLayer.(*layers.IPv4)
		if !ok {
			return nil, nil, nil, utils.Error("parse ipv4 layer failed")
		}
	}
	var tcp *layers.TCP
	if ret := packet.TransportLayer(); ret.LayerType() == layers.LayerTypeTCP {
		tcp, ok = ret.(*layers.TCP)
		if !ok {
			return nil, nil, nil, utils.Error("parse tcp layer failed")
		}
	}
	return ethernet, ipv4, tcp, nil
}
