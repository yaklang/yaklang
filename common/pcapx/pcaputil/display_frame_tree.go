package pcaputil

import (
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// FrameDisplayInput describes one physical capture frame. All ranges returned
// by BuildFrameDisplayNodes are bit ranges relative to Raw.
type FrameDisplayInput struct {
	Raw            []byte
	LinkType       int
	PacketID       uint64
	CapturedLength int
	OriginalLength int
	Section        int64
	Interface      int
	TimestampNS    *int64
}

func displayLeaf(name string, value any, bitOffset, bitLength int64) DisplayNode {
	displayValue := fmt.Sprint(value)
	if raw, ok := value.([]byte); ok {
		displayValue = "0x" + hex.EncodeToString(raw)
	}
	node := DisplayNode{Name: name, DisplayValue: displayValue}
	if bitOffset >= 0 && bitLength > 0 {
		node.Ranges = []DisplayRange{{BitOffset: bitOffset, BitLength: bitLength}}
	}
	return node
}

// BuildFrameDisplayNodes parses a physical frame once for presentation. It
// decodes link/network/transport layers and nests an undecoded payload under
// the last valid protocol layer.
func BuildFrameDisplayNodes(input FrameDisplayInput) []DisplayNode {
	frame := []DisplayNode{
		displayLeaf("Number", input.PacketID, 0, 0),
		displayLeaf("Captured length", input.CapturedLength, 0, 0),
		displayLeaf("Original length", input.OriginalLength, 0, 0),
		displayLeaf("Link type", input.LinkType, 0, 0),
		displayLeaf("Section", input.Section, 0, 0),
		displayLeaf("Interface", input.Interface, 0, 0),
	}
	if input.TimestampNS != nil {
		frame = append(frame, displayLeaf("Timestamp (ns)", *input.TimestampNS, 0, 0))
	}
	result := []DisplayNode{{
		Name: fmt.Sprintf("Frame %d: %d bytes on wire, %d bytes captured",
			input.PacketID, input.OriginalLength, input.CapturedLength),
		Children: frame,
	}}
	decoded := gopacket.NewPacket(input.Raw, layers.LinkType(input.LinkType), gopacket.Default)
	offset := 0
	for _, layer := range decoded.Layers() {
		if layer.LayerType() == gopacket.LayerTypePayload || layer.LayerType() == gopacket.LayerTypeDecodeFailure {
			continue
		}
		header := layer.LayerContents()
		if len(header) == 0 {
			continue
		}
		start := offset * 8
		leaf := func(name string, value any, first, last int) DisplayNode {
			return displayLeaf(name, value, int64(start+first*8), int64((last-first)*8))
		}
		var fields []DisplayNode
		var summary string
		switch v := layer.(type) {
		case *layers.Ethernet:
			fields = []DisplayNode{leaf("Destination", v.DstMAC, 0, 6), leaf("Source", v.SrcMAC, 6, 12), leaf("Type", v.EthernetType, 12, 14)}
			summary = fmt.Sprintf("%s → %s", v.SrcMAC, v.DstMAC)
		case *layers.Dot1Q:
			fields = []DisplayNode{leaf("Priority", v.Priority, 0, 2), leaf("Drop eligible", v.DropEligible, 0, 2), leaf("VLAN ID", v.VLANIdentifier, 0, 2), leaf("Type", v.Type, 2, 4)}
			summary = fmt.Sprintf("VLAN %d · PCP %d", v.VLANIdentifier, v.Priority)
		case *layers.IPv4:
			fields = []DisplayNode{leaf("Version", v.Version, 0, 1), leaf("Header length", int(v.IHL)*4, 0, 1), leaf("Type of service", v.TOS, 1, 2), leaf("Total length", v.Length, 2, 4), leaf("Identification", v.Id, 4, 6), leaf("Flags", v.Flags, 6, 8), leaf("Fragment offset", v.FragOffset, 6, 8), leaf("Time to live", v.TTL, 8, 9), leaf("Protocol", v.Protocol, 9, 10), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 10, 12), leaf("Source address", v.SrcIP, 12, 16), leaf("Destination address", v.DstIP, 16, 20)}
			summary = fmt.Sprintf("%s → %s · %s · TTL %d", v.SrcIP, v.DstIP, v.Protocol, v.TTL)
			if len(header) > 20 {
				fields = append(fields, leaf("Options", header[20:], 20, len(header)))
			}
		case *layers.IPv6:
			fields = []DisplayNode{leaf("Version", v.Version, 0, 1), leaf("Traffic class", v.TrafficClass, 0, 2), leaf("Flow label", v.FlowLabel, 1, 4), leaf("Payload length", v.Length, 4, 6), leaf("Next header", v.NextHeader, 6, 7), leaf("Hop limit", v.HopLimit, 7, 8), leaf("Source address", v.SrcIP, 8, 24), leaf("Destination address", v.DstIP, 24, 40)}
			summary = fmt.Sprintf("%s → %s · %s · Hop limit %d", v.SrcIP, v.DstIP, v.NextHeader, v.HopLimit)
		case *layers.TCP:
			flags := TCPFlagNames(v)
			fields = []DisplayNode{leaf("Source port", uint16(v.SrcPort), 0, 2), leaf("Destination port", uint16(v.DstPort), 2, 4), leaf("Sequence number", v.Seq, 4, 8), leaf("Acknowledgment number", v.Ack, 8, 12), leaf("Header length", int(v.DataOffset)*4, 12, 13), leaf("Flags", strings.Join(flags, ", "), 12, 14), leaf("Window", v.Window, 14, 16), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 16, 18), leaf("Urgent pointer", v.Urgent, 18, 20)}
			summary = fmt.Sprintf("%s · %s · Seq=%d Ack=%d Win=%d Len=%d",
				TCPTransportPorts(v).Arrow(), TCPFlagBracketed(v), v.Seq, v.Ack, v.Window, TCPPayloadLength(v))
			if len(header) > 20 {
				fields = append(fields, leaf("Options", header[20:], 20, len(header)))
			}
		case *layers.UDP:
			fields = []DisplayNode{leaf("Source port", uint16(v.SrcPort), 0, 2), leaf("Destination port", uint16(v.DstPort), 2, 4), leaf("Length", v.Length, 4, 6), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 6, 8)}
			summary = fmt.Sprintf("%s · Len=%d · Payload=%d",
				UDPTransportPorts(v).Arrow(), v.Length, UDPPayloadLength(v))
		case *layers.ARP:
			switch v.Operation {
			case layers.ARPRequest:
				summary = fmt.Sprintf("Who has %s? Tell %s", net.IP(v.DstProtAddress), net.IP(v.SourceProtAddress))
			case layers.ARPReply:
				summary = fmt.Sprintf("%s is at %s", net.IP(v.SourceProtAddress), net.HardwareAddr(v.SourceHwAddress))
			default:
				summary = fmt.Sprintf("Operation %d", v.Operation)
			}
		case *layers.ICMPv4:
			summary = v.TypeCode.String()
		case *layers.ICMPv6:
			summary = v.TypeCode.String()
		default:
			// Unknown headers remain inspectable without inventing field ranges.
			value := reflect.ValueOf(layer)
			if value.Kind() == reflect.Pointer {
				value = value.Elem()
			}
			if value.Kind() == reflect.Struct {
				typ := value.Type()
				for i := 0; i < value.NumField(); i++ {
					field := typ.Field(i)
					if field.PkgPath != "" || field.Anonymous || !value.Field(i).CanInterface() {
						continue
					}
					fields = append(fields, displayLeaf(field.Name, value.Field(i).Interface(), 0, 0))
				}
			}
		}
		fields = append(fields, leaf("Header bytes", header, 0, len(header)))
		result = append(result, DisplayNode{Name: layer.LayerType().String(), DisplayValue: summary, Children: fields})
		offset += len(header)
	}
	if offset < len(input.Raw) {
		payload := displayLeaf("Payload", input.Raw[offset:], int64(offset*8), int64((len(input.Raw)-offset)*8))
		if len(result) > 1 {
			last := &result[len(result)-1]
			last.Children = append(last.Children, payload)
			return result
		}
		// Preserve the bytes even when no protocol layer could be decoded.
		result = append(result, payload)
	}
	return result
}
