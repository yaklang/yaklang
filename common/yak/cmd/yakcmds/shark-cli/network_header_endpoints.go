package sharkcli

import (
	"bytes"
	"encoding/binary"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type capturedNetworkHeader struct {
	label, source, destination string
	start, end                 int
	sourceStart, sourceEnd     int
	destStart, destEnd         int
}

// A refusal event describes its admitted header prefix, not its speculative
// deepest gopacket transport. Address projection must follow that exact prefix
// and validate complete literal IP headers before publishing either endpoint.
func capturedNetworkPrefix(p gopacket.Packet, wire []byte, names []string) []capturedNetworkHeader {
	var out []capturedNetworkHeader
	offset, limit := 0, len(wire)
	for i, layer := range p.Layers() {
		if i >= len(names) || layer.LayerType().String() != names[i] {
			break
		}
		if layer.LayerType() == layers.LayerTypeICMPv4 || layer.LayerType() == layers.LayerTypeICMPv6 {
			break // Quoted headers cannot become physical capture endpoints.
		}
		h := layer.LayerContents()
		if offset < 0 || offset > limit || len(h) > limit-offset || !bytes.Equal(h, wire[offset:offset+len(h)]) {
			break
		}
		start, end := offset, offset+len(h)
		switch v := layer.(type) {
		case *layers.IPv4:
			if len(h) < 20 || h[0]>>4 != 4 || int(h[0]&15)*4 != len(h) || int(binary.BigEndian.Uint16(h[2:4])) < len(h) {
				return out
			}
			out = append(out, capturedNetworkHeader{label: "IPv4", source: net.IP(h[12:16]).String(), destination: net.IP(h[16:20]).String(), start: start, end: end, sourceStart: start + 12, sourceEnd: start + 16, destStart: start + 16, destEnd: start + 20})
			limit = min(limit, start+int(binary.BigEndian.Uint16(h[2:4])))
			if binary.BigEndian.Uint16(h[6:8])&0x3fff != 0 {
				return out // Fragment payload does not prove another captured header.
			}
		case *layers.IPv6:
			if len(h) != 40 || h[0]>>4 != 6 {
				return out
			}
			out = append(out, capturedNetworkHeader{label: "IPv6", source: net.IP(h[8:24]).String(), destination: net.IP(h[24:40]).String(), start: start, end: end, sourceStart: start + 8, sourceEnd: start + 24, destStart: start + 24, destEnd: start + 40})
			payload := int(binary.BigEndian.Uint16(h[4:6]))
			if payload == 0 {
				return out // No jumbo-payload profile is selected here.
			}
			limit = min(limit, end+payload)
		case *layers.Ethernet:
			if len(h) != 14 {
				return out
			}
			if length := int(binary.BigEndian.Uint16(h[12:14])); length <= 1500 {
				limit = min(limit, end+length)
			}
		case *layers.UDP:
			if len(h) != 8 || v.Length < 8 || int(v.Length) > limit-start {
				return out
			}
			limit = start + int(v.Length)
			if i+1 < len(names) {
				// The supported UDP-to-Ethernet prefix here is actual-destination
				// VXLAN. Source hints, unknown profiles and GPE never select a child.
				if v.DstPort.LayerType() != layers.LayerTypeVXLAN || limit-end < 22 || wire[end]&8 == 0 {
					return out
				}
			}
		case *layers.VXLAN:
			if len(h) != 8 || h[0]&8 == 0 || limit-end < 14 {
				return out
			}
		case *layers.GRE:
			if v.Version != 0 {
				return out // No enhanced-GRE call-control proof is recreated in the UI.
			}
		case *layers.Geneve:
			if len(h) != 8 || h[0] != 0 || h[1] != 0 || h[7] != 0 || v.Protocol != layers.EthernetTypeTransparentEthernetBridging {
				return out
			}
		case *layers.IPv6Fragment:
			if v.FragmentOffset != 0 || v.MoreFragments {
				return out
			}
		}
		offset = end
	}
	return out
}

func presentationNetworkHeaders(p packetPresentation, events []*pcaputil.ProtocolEvent) []string {
	if len(p.nativeHeaders) > 0 {
		return p.nativeHeaders
	}
	if p.networkRefused {
		for _, event := range events {
			if event == nil {
				continue
			}
			selected := p.protocol == "VXLAN-GPE" && p.visibility == "tunnel-header" && validVXLANGPEHeaderObservation(event)
			if p.protocol == "PPTP" && p.visibility == "acknowledgment-header" {
				candidate := presentEvents([]*pcaputil.ProtocolEvent{event})
				selected = candidate.networkRefused && candidate.protocol == "PPTP" && candidate.visibility == "acknowledgment-header"
			}
			if selected {
				return headerLayerNames(event.Session["Observed Header Layers"])
			}
		}
	}
	return nil
}

func applyCapturedNetworkEndpoints(row packetSummary, presentation *packetPresentation, p gopacket.Packet, wire []byte, names []string) packetSummary {
	if !presentation.networkRefused {
		return row
	}
	// Reset legacy deepest-child addresses even if no complete physical IP
	// header can be confirmed (wrong version or fixed-header truncation).
	row.Source, row.Destination, row.PhysicalSource, row.PhysicalDestination = "", "", "", ""
	presentation.nativeSource, presentation.nativeDestination = "", ""
	if headers := capturedNetworkPrefix(p, wire, names); len(headers) > 0 {
		first, last := headers[0], headers[len(headers)-1]
		row.Source, row.Destination = last.source, last.destination
		row.PhysicalSource, row.PhysicalDestination = first.source, first.destination
		presentation.nativeSource, presentation.nativeDestination = last.source, last.destination
	}
	return row
}

func appendCapturedNetworkPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) {
	presentation := raw.presentation
	if !presentation.networkRefused {
		presentation = presentEvents(events)
	}
	if !presentation.networkRefused {
		return
	}
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	headers := capturedNetworkPrefix(p, raw.data, presentationNetworkHeaders(presentation, events))
	if len(headers) == 0 {
		return
	}
	d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
	for _, h := range headers {
		d.fields = append(d.fields, field{text: h.label + " header", branch: true, start: h.start, end: h.end, hasRange: true}, field{depth: 1, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
	}
}
