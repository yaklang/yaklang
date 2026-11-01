package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ospfv2FieldValue(name string, value any) (string, bool) {
	n, ok := detailRangeOffset(value)
	if !ok || n > 0xffffffff {
		return "", false
	}
	var label string
	switch name {
	case "Type":
		if n > 255 {
			return "", false
		}
		label = stream_parser.OSPFv2MessageName(uint64(n))
	case "LS Type":
		label = map[int]string{1: "Router-LSA", 2: "Network-LSA", 3: "Summary-Network-LSA", 4: "Summary-ASBR-LSA", 5: "AS-External-LSA", 7: "NSSA-External-LSA"}[n]
		if label == "" {
			label = "unselected LSA type"
		}
	case "Authentication Type":
		label = map[int]string{0: "None", 1: "Simple password; not authenticated", 2: "Cryptographic; not verified"}[n]
		if label == "" {
			label = "unselected authentication scheme"
		}
	case "TLV Type":
		label = map[int]string{1: "LLS Extended Options", 2: "LLS Cryptographic Authentication"}[n]
		if label == "" {
			label = "unselected LLS TLV"
		}
	case "Link Type":
		label = map[int]string{1: "Point-to-point", 2: "Transit network", 3: "Stub network", 4: "Virtual link"}[n]
		if label == "" {
			label = "unselected link type"
		}
	default:
		return "", false
	}
	return fmt.Sprintf("%d (%s)", n, label), true
}

// Only a source-reproduced native observation can replace the physical legacy
// body. Logical OSPF offsets are never reused as offsets in the captured frame.
func appendOSPFv2CapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	ip, ok := p.NetworkLayer().(*layers.IPv4)
	if !ok || p.TransportLayer() != nil || ip.Protocol != layers.IPProtocolOSPF || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 {
		return false
	}
	var wire []byte
	var names []string
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
		offset += len(h)
	}
	if networks != 1 || len(wire) == 0 || wire[0] != 2 || len(wire) > len(raw.data)-offset || !bytes.Equal(wire, raw.data[offset:offset+len(wire)]) {
		return false
	}
	fields, _ := stream_parser.DecodeOSPFv2Wire(wire, 4096)
	want, err := json.Marshal(fields)
	if err != nil || fields == nil {
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "ospf" || e.Transport != "network" || e.Profile != "ospfv2-bounded-control" || e.Rule != "ospfv2_observed" || e.Entry != "OSPFv2ControlFields" || e.FlowID != 0 || e.TCPFlowID != 0 || e.TransactionID != 0 || e.ResponseTo != 0 || e.Length != len(wire) || !bytes.Equal(e.Raw, wire) || e.Source != ip.SrcIP.String() || e.Destination != ip.DstIP.String() {
			continue
		}
		source := e.SourceBytes
		if source.Kind != "captured" || source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || source.ParentPDU != 0 || len(source.ParentPDUs) != 0 || len(source.PacketRefs) != 1 || source.PacketRefs[0] != raw.reference {
			continue
		}
		actual, err := json.Marshal(e.Session)
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
		return true
	}
	return false
}
