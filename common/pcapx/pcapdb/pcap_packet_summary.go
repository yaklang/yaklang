package pcapdb

import (
	"fmt"
	"net"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
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
			pcaputil.TCPFlagText(transport),
			transport.Seq, transport.Ack, transport.Window, pcaputil.TCPPayloadLength(transport))
	case *layers.UDP:
		summary = fmt.Sprintf("%s Len=%d Payload=%d",
			pcaputil.UDPTransportPorts(transport).Labeled(), transport.Length, pcaputil.UDPPayloadLength(transport))
	default:
		summary = packetGenericSummary(packet)
	}
	return limitPacketSummary(summary)
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
