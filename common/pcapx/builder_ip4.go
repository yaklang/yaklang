package pcapx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

type IPv4LayerBuilderConfig struct {
	Version    uint8
	IHL        uint8
	TOS        uint8
	Length     uint16
	Id         uint16
	Flags      layers.IPv4Flag
	FragOffset uint16
	TTL        uint8
	Protocol   layers.IPProtocol
	Checksum   uint16
	SrcIP      net.IP
	DstIP      net.IP
	Options    []layers.IPv4Option
}

type IPv4LayerBuilderConfigOption func(*IPv4LayerBuilderConfig)

func WithIPv4LayerBuilderConfigSrcIP(srcIP any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.SrcIP = net.ParseIP(utils.InterfaceToString(srcIP))
	}
}

func WithIPv4LayerBuilderConfigDstIP(dstIP any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.DstIP = net.ParseIP(utils.InterfaceToString(dstIP))
	}
}

func WithIPv4LayerBuilderConfigVersion(version any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.Version = uint8(utils.Atoi(utils.InterfaceToString(version)))
	}
}

func WithIPv4LayerBuilderConfigIHL(ihl any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.IHL = uint8(utils.Atoi(utils.InterfaceToString(ihl)))
	}
}

func WithIPv4LayerBuilderConfigTOS(tos any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.TOS = uint8(utils.Atoi(utils.InterfaceToString(tos)))
	}
}

func WithIPv4LayerBuilderConfigLength(length any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.Length = uint16(utils.Atoi(utils.InterfaceToString(length)))
	}
}

func WithIPv4LayerBuilderConfigId(id any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.Id = uint16(utils.Atoi(utils.InterfaceToString(id)))
	}
}

func WithIPv4LayerBuilderConfigFlags(flags any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.Flags = layers.IPv4Flag(utils.Atoi(utils.InterfaceToString(flags)))
	}
}

func WithIPv4LayerBuilderConfigFragOffset(fragOffset any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.FragOffset = uint16(utils.Atoi(utils.InterfaceToString(fragOffset)))
	}
}

func WithIPv4LayerBuilderConfigTTL(ttl any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		i.TTL = uint8(utils.Atoi(utils.InterfaceToString(ttl)))
	}
}

func WithIPv4LayerBuilderConfigProtocol(protocol any) IPv4LayerBuilderConfigOption {
	return func(i *IPv4LayerBuilderConfig) {
		switch ret := strings.ToLower(utils.InterfaceToString(protocol)); ret {
		case "icmp":
			i.Protocol = layers.IPProtocolICMPv4
		case "tcp":
			i.Protocol = layers.IPProtocolTCP
		case "udp":
			i.Protocol = layers.IPProtocolUDP
		case "sctp":
			i.Protocol = layers.IPProtocolSCTP
		case "ospf":
			i.Protocol = layers.IPProtocolOSPF
		case "gre":
			i.Protocol = layers.IPProtocolGRE
		case "igmp":
			i.Protocol = layers.IPProtocolIGMP
		default:
			i.Protocol = layers.IPProtocol(utils.Atoi(ret))
		}
	}
}

func (i *IPv4LayerBuilderConfig) Create() *layers.IPv4 {
	return &layers.IPv4{
		Version:    i.IHL,
		IHL:        i.IHL,
		TOS:        i.TOS,
		Length:     i.Length,
		Id:         i.Id,
		Flags:      i.Flags,
		FragOffset: i.FragOffset,
		TTL:        i.TTL,
		Protocol:   i.Protocol,
		SrcIP:      i.SrcIP,
		DstIP:      i.DstIP,
		Options:    i.Options,
	}
}
