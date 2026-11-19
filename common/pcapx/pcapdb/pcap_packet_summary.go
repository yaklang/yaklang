package pcapdb

import (
	"fmt"
	"net"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

const packetSummaryLimit = 512

// SummarizePacketData builds a bounded summary for one physical capture frame.
// It intentionally does not use protocol_messages: a frame summary remains
// available even when protocol analysis is disabled or cannot reassemble a
// complete application message.
func SummarizePacketData(raw []byte, packet Packet) string {
	decoded := gopacket.NewPacket(raw, layers.LinkType(packet.LinkType), gopacket.DecodeOptions{Lazy: true})
	return packetFrameSummary(decoded, packet)
}

func packetFrameSummary(decoded gopacket.Packet, packet Packet) string {
	var summary string
	switch transport := decoded.TransportLayer().(type) {
	case *layers.TCP:
		summary = fmt.Sprintf("[%s] Seq=%d Ack=%d Win=%d Len=%d",
			packetTCPFlags(transport.SYN, transport.ACK, transport.FIN, transport.RST, transport.PSH, transport.URG, transport.ECE, transport.CWR, transport.NS),
			transport.Seq, transport.Ack, transport.Window, len(transport.LayerPayload()))
	case *layers.UDP:
		summary = fmt.Sprintf("SrcPort=%d DstPort=%d Len=%d Payload=%d",
			uint16(transport.SrcPort), uint16(transport.DstPort), transport.Length, len(transport.LayerPayload()))
	default:
		summary = packetGenericSummary(packet)
	}
	return limitPacketSummary(summary)
}

func packetTCPFlags(syn, ack, fin, rst, psh, urg, ece, cwr, ns bool) string {
	type flag struct {
		set  bool
		name string
	}
	names := make([]string, 0, 9)
	for _, f := range []flag{{syn, "SYN"}, {ack, "ACK"}, {fin, "FIN"}, {rst, "RST"}, {psh, "PSH"}, {urg, "URG"}, {ece, "ECE"}, {cwr, "CWR"}, {ns, "NS"}} {
		if f.set {
			names = append(names, f.name)
		}
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, ", ")
}

func packetGenericSummary(packet Packet) string {
	protocol := strings.ToUpper(packet.Transport)
	if protocol == "" {
		protocol = packetNetworkLayer(packet.SourceIP)
	}
	if protocol == "" {
		protocol = strings.ToUpper(layers.LinkType(packet.LinkType).String())
	}
	if protocol == "" {
		protocol = "Packet"
	}
	return fmt.Sprintf("%s · %d bytes", protocol, packet.CapturedLength)
}

func packetNetworkLayer(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
}

func limitPacketSummary(summary string) string {
	summary = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, summary)
	summary = strings.Join(strings.Fields(summary), " ")
	runes := []rune(summary)
	if len(runes) <= packetSummaryLimit {
		return summary
	}
	return string(runes[:packetSummaryLimit-1]) + "…"
}
