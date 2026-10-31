package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Keep the literal integer in fields/filters. Human-readable type names add
// context in the inspector without changing the original two-byte span.
func ospfv3FieldValue(name string, value any) (string, bool) {
	if name != "Type" && name != "LS Type" && name != "Referenced LS Type" {
		return "", false
	}
	n, ok := detailRangeOffset(value)
	if !ok || n > 65535 || name == "Type" && n > 255 {
		return "", false
	}
	if name == "Type" {
		label := stream_parser.OSPFv3MessageName(uint64(n))
		if label == "Unknown" {
			label = "unselected message type"
		}
		return fmt.Sprintf("%d (%s)", n, label), true
	}
	label := map[int]string{0x2001: "Router-LSA", 0x2002: "Network-LSA", 0x2003: "Inter-Area-Prefix-LSA", 0x2004: "Inter-Area-Router-LSA", 0x4005: "AS-External-LSA", 0x2007: "NSSA-LSA", 0x0008: "Link-LSA", 0x2009: "Intra-Area-Prefix-LSA"}[n]
	if name == "Referenced LS Type" && n == 0 {
		label = "no referenced LSA"
	}
	if label == "" {
		label = "unselected LSA type"
	}
	return fmt.Sprintf("0x%04x (%s)", n, label), true
}

// The source-reproduced native tree replaces the failed legacy LSA walker.
// Physical IPv6/AH fields remain separate from logical (possibly reassembled)
// OSPF bytes. A protocol label or an editable event map is insufficient.
func appendOSPFv3CapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	ip, ok := p.NetworkLayer().(*layers.IPv6)
	if !ok || p.TransportLayer() != nil {
		return false
	}
	context := stream_parser.OSPFv3WireContext{}
	context.Source, _ = netip.AddrFromSlice(ip.SrcIP)
	context.Destination, _ = netip.AddrFromSlice(ip.DstIP)
	var wire []byte
	var names []string
	var ahHeaders []field
	offset, networks := 0, 0
	for _, layer := range p.Layers() {
		if _, ok := layer.(gopacket.NetworkLayer); ok {
			networks++
		}
		if layer.LayerType() == layers.LayerTypeOSPF {
			wire = append(bytes.Clone(layer.LayerContents()), layer.LayerPayload()...)
			break
		}
		h := layer.LayerContents()
		if len(h) > len(raw.data)-offset || !bytes.Equal(h, raw.data[offset:offset+len(h)]) {
			return false
		}
		names = append(names, layer.LayerType().String())
		switch v := layer.(type) {
		case *layers.IPv6Fragment:
			return false // Physical fragments never claim a complete child.
		case *layers.IPv6Routing:
			if v.SegmentsLeft != 0 {
				if len(h) >= 24 && (v.RoutingType == 0 || v.RoutingType == 2) && (len(h)-8)%16 == 0 && int(v.SegmentsLeft) <= (len(h)-8)/16 {
					context.Destination = netip.AddrFrom16([16]byte(h[len(h)-16:]))
				} else {
					context.PseudoHeaderUnavailable = true
				}
			}
		case *layers.IPSecAH:
			if len(h) < 12 {
				return false
			}
			ahHeaders = append(ahHeaders, field{text: "AH header · authentication unverified", branch: true, start: offset, end: offset + len(h), hasRange: true},
				field{depth: 1, text: fmt.Sprintf("SPI: 0x%08x", v.SPI), start: offset + 4, end: offset + 8, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Sequence: %d", v.Seq), start: offset + 8, end: offset + 12, hasRange: true})
		}
		offset += len(h)
	}
	if networks != 1 || len(wire) == 0 || len(wire) > len(raw.data)-offset || !bytes.Equal(wire, raw.data[offset:offset+len(wire)]) {
		return false
	}
	fields, _ := stream_parser.DecodeOSPFv3Wire(wire, context, 4096)
	want, err := json.Marshal(fields)
	if err != nil || fields == nil {
		return false
	}
	for _, event := range events {
		if event == nil || event.Protocol != "ospf" || event.Transport != "network" || event.Profile != "ospfv3-bounded-control" || event.Rule != "ospfv3_observed" || event.Entry != "OSPFv3ControlFields" || event.FlowID != 0 || event.TCPFlowID != 0 || event.TransactionID != 0 || event.ResponseTo != 0 || event.Length != len(wire) || !bytes.Equal(event.Raw, wire) {
			continue
		}
		if event.Source != ip.SrcIP.String() || event.Destination != ip.DstIP.String() {
			continue
		}
		source := event.SourceBytes
		if source.Kind != "captured" || source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || source.ParentPDU != 0 || len(source.ParentPDUs) != 0 || len(source.PacketRefs) != 1 || source.PacketRefs[0] != raw.reference {
			continue
		}
		actual, err := json.Marshal(event.Session)
		if err != nil || !bytes.Equal(want, actual) {
			continue
		}
		headers := capturedNetworkPrefix(p, raw.data, names)
		if len(headers) == 0 {
			return false
		}
		d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
		for _, h := range headers {
			d.fields = append(d.fields, field{text: h.label + " header", branch: true, start: h.start, end: h.end, hasRange: true},
				field{depth: 1, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true},
				field{depth: 1, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
		}
		d.fields = append(d.fields, ahHeaders...)
		return true
	}
	return false
}
