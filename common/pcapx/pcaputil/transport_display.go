package pcaputil

import (
	"fmt"
	"strings"

	"github.com/gopacket/gopacket/layers"
)

// TransportPorts is the stable source/destination ordering used by packet
// summaries and protocol-tree node hints.
type TransportPorts struct {
	Source      uint16
	Destination uint16
}

func TCPTransportPorts(tcp *layers.TCP) TransportPorts {
	if tcp == nil {
		return TransportPorts{}
	}
	return TransportPorts{Source: uint16(tcp.SrcPort), Destination: uint16(tcp.DstPort)}
}

func UDPTransportPorts(udp *layers.UDP) TransportPorts {
	if udp == nil {
		return TransportPorts{}
	}
	return TransportPorts{Source: uint16(udp.SrcPort), Destination: uint16(udp.DstPort)}
}

func (p TransportPorts) Labeled() string {
	return fmt.Sprintf("SrcPort=%d DstPort=%d", p.Source, p.Destination)
}

func (p TransportPorts) Arrow() string {
	return fmt.Sprintf("%d → %d", p.Source, p.Destination)
}

// TCPFlagNames returns flags in one canonical order for both list summaries and
// tree fields.
func TCPFlagNames(tcp *layers.TCP) []string {
	if tcp == nil {
		return nil
	}
	flags := make([]string, 0, 9)
	for _, flag := range []struct {
		set  bool
		name string
	}{
		{tcp.SYN, "SYN"},
		{tcp.ACK, "ACK"},
		{tcp.FIN, "FIN"},
		{tcp.RST, "RST"},
		{tcp.PSH, "PSH"},
		{tcp.URG, "URG"},
		{tcp.ECE, "ECE"},
		{tcp.CWR, "CWR"},
		{tcp.NS, "NS"},
	} {
		if flag.set {
			flags = append(flags, flag.name)
		}
	}
	return flags
}

func TCPFlagText(tcp *layers.TCP) string {
	flags := TCPFlagNames(tcp)
	if len(flags) == 0 {
		return "None"
	}
	return strings.Join(flags, ", ")
}

func TCPFlagBracketed(tcp *layers.TCP) string {
	flags := TCPFlagNames(tcp)
	if len(flags) == 0 {
		return "No flags"
	}
	return "[" + strings.Join(flags, ", ") + "]"
}

func TCPPayloadLength(tcp *layers.TCP) int {
	if tcp == nil {
		return 0
	}
	return len(tcp.LayerPayload())
}

func UDPPayloadLength(udp *layers.UDP) int {
	if udp == nil {
		return 0
	}
	return len(udp.LayerPayload())
}
