package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func validRSVPControlObservation(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "rsvp" || e.Transport != "network" || e.Profile != "rsvp-bounded-control" || e.Rule != "rsvp_observed" || e.Entry != "RSVPControlFields" || e.FlowID != 0 || e.TCPFlowID != 0 || e.TransactionID != 0 || e.ResponseTo != 0 || len(e.SourceBytes.PacketRefs) == 0 {
		return false
	}
	if e.Admission != "observed-IP-next-header46" && e.Admission != "observed-IP-next-header46-and-complete-base-wire" {
		return false
	}
	switch e.SourceBytes.Kind {
	case "captured", "reassembled", "decapsulated":
	default:
		return false
	}
	if e.Length != len(e.Raw) {
		return len(e.Raw) == 0 && e.Length > 0 && e.Status == "limited" && e.ExpertCode == "ResourceExceeded" && e.SourceBytes.SHA256 == ""
	}
	if len(e.Raw) == 0 {
		return e.SourceBytes.SHA256 == ""
	}
	return e.SourceBytes.SHA256 == fmt.Sprintf("%x", sha256.Sum256(e.Raw))
}

func rsvpFieldValue(name string, value any) (string, bool) {
	n, ok := detailRangeOffset(value)
	if !ok {
		return "", false
	}
	label := ""
	switch name {
	case "Message Type":
		label = stream_parser.RSVPMessageName(uint64(n))
	case "Class":
		label = stream_parser.RSVPObjectName(uint64(n))
	case "Option Vector":
		label = map[int]string{17: "Wildcard Filter", 10: "Fixed Filter", 18: "Shared Explicit"}[n&31]
		if label == "" {
			label = "unselected style"
		}
	default:
		return "", false
	}
	return fmt.Sprintf("%d (%s)", n, label), true
}
func rsvpObservedBudget(session map[string]any) (int, bool) {
	count := 0
	for _, row := range mapList(session["Objects"]) {
		if row == nil {
			return 0, false
		}
		count++
		if body, ok := row["Body"].(map[string]any); ok {
			count += len(mapList(body["Addresses"]))
		}
		if count > 4096 {
			return 0, false
		}
	}
	return count, true
}

func appendRSVPCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	if failure := p.ErrorLayer(); failure != nil {
		var wireError *pcaputil.RSVPObservedWireError
		if !errors.As(failure.Error(), &wireError) {
			return false
		}
	}
	if p.TransportLayer() != nil {
		return false
	}
	offset, networks, start := 0, 0, -1
	var wire []byte
	var names []string
	short := raw.ci.CaptureLength < raw.ci.Length
	for _, layer := range p.Layers() {
		selected := false
		switch l := layer.(type) {
		case *layers.IPv4:
			networks++
			if l.IHL < 5 || int(l.Length) < int(l.IHL)*4 || !short && int(l.Length) > len(raw.data)-offset || l.FragOffset != 0 || l.Flags&layers.IPv4MoreFragments != 0 {
				return false
			}
			selected = l.Protocol == 46
		case *layers.IPv6:
			networks++
			if l.Length == 0 || !short && 40+int(l.Length) > len(raw.data)-offset {
				return false
			}
			selected = l.NextHeader == 46
		case *layers.IPv6HopByHop:
			selected = l.NextHeader == 46
		case *layers.IPv6Destination:
			selected = l.NextHeader == 46
		case *layers.IPv6Routing:
			selected = l.NextHeader == 46
		case *layers.IPSecAH:
			selected = l.NextHeader == 46
		case *layers.IPv6Fragment:
			return false
		}
		header := layer.LayerContents()
		if offset > len(raw.data) || len(header) > len(raw.data)-offset || !bytes.Equal(header, raw.data[offset:offset+len(header)]) {
			return false
		}
		names = append(names, layer.LayerType().String())
		offset += len(header)
		if selected {
			start = offset
			wire = layer.LayerPayload()
			break
		}
	}
	if networks != 1 || start < 0 || len(wire) > len(raw.data)-start || !bytes.Equal(wire, raw.data[start:start+len(wire)]) {
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "rsvp" || e.Transport != "network" || e.Profile != "rsvp-bounded-control" || e.Rule != "rsvp_observed" || e.Entry != "RSVPControlFields" || e.FlowID != 0 || e.TCPFlowID != 0 || e.TransactionID != 0 || e.ResponseTo != 0 || e.Length != len(wire) {
			continue
		}
		s := e.SourceBytes
		if s.Kind != "captured" || s.ParentPDU != 0 || len(s.ParentPDUs) != 0 || len(s.PacketRefs) != 1 || s.PacketRefs[0] != raw.reference {
			continue
		}
		omitted := len(e.Raw) == 0 && len(wire) > 0 && e.ExpertCode == "ResourceExceeded"
		if !omitted {
			if !bytes.Equal(e.Raw, wire) || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) {
				continue
			}
			budget, ok := rsvpObservedBudget(e.Session)
			if !ok {
				continue
			}
			fields, _ := stream_parser.DecodeRSVPWire(wire, budget)
			want, err := json.Marshal(fields)
			if err != nil {
				continue
			}
			actual, err := json.Marshal(e.Session)
			if err != nil || !bytes.Equal(want, actual) {
				continue
			}
		}
		headers := capturedNetworkPrefix(p, raw.data, names)
		if len(headers) == 0 {
			return false
		}
		d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
		for _, h := range headers {
			d.fields = append(d.fields, field{text: h.label + " header", branch: true, start: h.start, end: h.end, hasRange: true}, field{depth: 1, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
		}
		return true
	}
	return false
}
