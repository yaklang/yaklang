package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// A source-reproduced direct PIM observation already owns its partial tree.
// Do not append another failed legacy tree to it. Complete Register data with
// an admitted inner network remains on the ordinary packet inspection path.
func appendPIMCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	network := p.NetworkLayer()
	if network == nil || p.TransportLayer() != nil {
		return false
	}
	var names []string
	networks := 0
	for _, layer := range p.Layers() {
		if _, ok := layer.(gopacket.NetworkLayer); ok {
			networks++
		}
		if networks == 0 || layer == network {
			names = append(names, layer.LayerType().String())
		}
	}
	if networks != 1 {
		return false
	}
	h, wire := network.LayerContents(), network.LayerPayload()
	context := stream_parser.PIMWireContext{DeclaredBytes: -1}
	switch ip := network.(type) {
	case *layers.IPv4:
		if ip.Protocol != 103 || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 || len(h) < 20 || h[0]>>4 != 4 || int(h[0]&15)*4 != len(h) {
			return false
		}
		if total := int(binary.BigEndian.Uint16(h[2:4])); total >= len(h) {
			context.DeclaredBytes = total - len(h)
		}
	case *layers.IPv6:
		if ip.NextHeader != 103 || len(h) != 40 || h[0]>>4 != 6 {
			return false
		}
		if payload := int(binary.BigEndian.Uint16(h[4:6])); payload > 0 {
			context.DeclaredBytes = payload
		}
	default:
		return false
	}
	context.Source, _ = netip.ParseAddr(network.NetworkFlow().Src().String())
	context.Destination, _ = netip.ParseAddr(network.NetworkFlow().Dst().String())
	fields, decodeErr := stream_parser.DecodePIMControl(wire, context, 4096)
	if fields == nil {
		return false
	}
	status, expert, completeness, diagnostic := "decoded", "", "message", ""
	admission, summary := "observed-IP-protocol-and-complete-control-grammar", "PIMv2 "+textValue(fields["Packet Name"])
	if decodeErr != nil {
		status, expert = "malformed", "MalformedMessage"
		if errors.Is(decodeErr, stream_parser.ErrObservedWireContext) {
			status, expert = "context-required", "ContextRequired"
		} else if errors.Is(decodeErr, stream_parser.ErrObservedWireBudget) {
			status, expert = "limited", "ResourceExceeded"
		}
		completeness, diagnostic = status, expert+": "+decodeErr.Error()
		admission, summary = "observed-IP-protocol-only", "pim"
	}
	want, err := json.Marshal(fields)
	if err != nil {
		return false
	}
	for _, event := range events {
		if event == nil || event.Protocol != "pim" || event.Transport != "network" || event.Profile != "pim2-bounded-control-advertisements" || event.Rule != "pim_control" || event.Entry != "PIMControlFields" || event.FlowID != 0 || event.TCPFlowID != 0 || event.Status != status || event.ExpertCode != expert || event.Completeness != completeness || event.Error != diagnostic || event.Admission != admission || event.Summary != summary || event.Length != len(wire) || !bytes.Equal(event.Raw, wire) {
			continue
		}
		if event.Source != context.Source.String() || event.Destination != context.Destination.String() {
			continue
		}
		source := event.SourceBytes
		if source.Kind != "captured" || source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || source.ParentPDU != 0 || len(source.ParentPDUs) != 0 || len(source.PacketRefs) != 1 || source.PacketRefs[0].Number != raw.number || source.PacketRefs[0].Domain != raw.reference.Domain {
			continue
		}
		if status == "decoded" {
			if source.CompletedIn == nil || *source.CompletedIn != source.PacketRefs[0] {
				continue
			}
		} else if source.CompletedIn != nil {
			continue
		}
		actual, err := json.Marshal(event.Session)
		if err != nil || !bytes.Equal(actual, want) {
			continue
		}
		headers := capturedNetworkPrefix(p, raw.data, names)
		if len(headers) == 0 {
			return false
		}
		d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
		for _, header := range headers {
			d.fields = append(d.fields, field{text: header.label + " header", branch: true, start: header.start, end: header.end, hasRange: true}, field{depth: 1, text: "Source: " + header.source, start: header.sourceStart, end: header.sourceEnd, hasRange: true}, field{depth: 1, text: "Destination: " + header.destination, start: header.destStart, end: header.destEnd, hasRange: true})
		}
		return true
	}
	return false
}
