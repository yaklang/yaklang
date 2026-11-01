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

func gtpv2FieldValue(name string, value any) (string, bool) {
	n, ok := detailRangeOffset(value)
	if !ok || n > 255 {
		return "", false
	}
	label := ""
	switch name {
	case "Message Type":
		label = stream_parser.GTPv2MessageName(uint64(n))
	case "IE Type", "Offending IE Type":
		label = stream_parser.GTPv2IEName(uint64(n))
	default:
		return "", false
	}
	return fmt.Sprintf("%d (%s)", n, label), true
}

// A collection-limited observation is reproduced with the count of IE nodes
// actually retained, not a larger viewer default. This counter grants no field
// identity: the complete re-decoded tree still has to match the source record.
func gtpv2ObservedIEBudget(values map[string]any) (int, bool) {
	count := 0
	var visit func(any, int) bool
	visit = func(value any, depth int) bool {
		rows := mapList(value)
		if depth > 8 && len(rows) != 0 {
			return false
		}
		for _, row := range rows {
			if row == nil || count >= 4096 {
				return false
			}
			count++
			if !visit(row["IEs"], depth+1) {
				return false
			}
		}
		return true
	}
	messages := mapList(values["Messages"])
	if len(messages) > 2 {
		return 0, false
	}
	for _, message := range messages {
		if !visit(message["IEs"], 0) {
			return 0, false
		}
	}
	return count, true
}

// Only one direct, source-reproduced bounded UDP observation may replace the
// legacy application tree. Logical IE positions remain relative to the owned
// UDP-PDU source, separate from the actual physical IP and UDP headers.
func appendGTPv2CapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	if p.ErrorLayer() != nil {
		return false
	}
	udp, ok := p.TransportLayer().(*layers.UDP)
	if !ok || udp.Length < 8 || int(udp.Length)-8 != len(udp.LayerPayload()) {
		return false
	}
	wire := udp.LayerPayload()
	var names []string
	offset, networks, udpOffset := 0, 0, -1
	for _, layer := range p.Layers() {
		if _, ok := layer.(gopacket.NetworkLayer); ok {
			networks++
		}
		header := layer.LayerContents()
		if offset > len(raw.data) || len(header) > len(raw.data)-offset || !bytes.Equal(header, raw.data[offset:offset+len(header)]) {
			return false
		}
		names = append(names, layer.LayerType().String())
		if layer == udp {
			udpOffset = offset
			offset += len(header)
			break
		}
		offset += len(header)
	}
	if networks != 1 || udpOffset < 0 || len(wire) > len(raw.data)-offset || !bytes.Equal(wire, raw.data[offset:offset+len(wire)]) {
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "gtpv2" || e.Transport != "udp" || e.Profile != "gtpv2-bounded-UDP-observation" || e.Rule != "gtpv2_observed" || e.Entry != "GTPv2ControlFields" || e.FlowID != 0 || e.TCPFlowID != 0 || e.TransactionID != 0 || e.ResponseTo != 0 || e.Length != len(wire) || !bytes.Equal(e.Raw, wire) {
			continue
		}
		s := e.SourceBytes
		if s.Kind != "captured" || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || s.ParentPDU != 0 || len(s.ParentPDUs) != 0 || len(s.PacketRefs) != 1 || s.PacketRefs[0] != raw.reference {
			continue
		}
		budget, ok := gtpv2ObservedIEBudget(e.Session)
		if !ok {
			continue
		}
		fields, _ := stream_parser.DecodeGTPv2Wire(wire, budget)
		if fields == nil {
			continue
		}
		want, err := json.Marshal(fields)
		if err != nil {
			continue
		}
		actual, err := json.Marshal(e.Session)
		if err != nil || !bytes.Equal(actual, want) {
			continue
		}
		headers := capturedNetworkPrefix(p, raw.data, names)
		if len(headers) == 0 {
			return false
		}
		d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
		for _, h := range headers {
			d.fields = append(d.fields, field{text: h.label + " header", branch: true, start: h.start, end: h.end, hasRange: true}, field{depth: 1, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
		}
		d.fields = append(d.fields, field{text: "UDP header", branch: true, start: udpOffset, end: udpOffset + 8, hasRange: true}, field{depth: 1, text: fmt.Sprintf("Source Port: %d", udp.SrcPort), start: udpOffset, end: udpOffset + 2, hasRange: true}, field{depth: 1, text: fmt.Sprintf("Destination Port: %d", udp.DstPort), start: udpOffset + 2, end: udpOffset + 4, hasRange: true}, field{depth: 1, text: fmt.Sprintf("Length: %d", udp.Length), start: udpOffset + 4, end: udpOffset + 6, hasRange: true})
		return true
	}
	return false
}
