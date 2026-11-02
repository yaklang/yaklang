package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ipv4FragmentExtentRefusal(e *pcaputil.ProtocolEvent) ([]string, bool) {
	if e == nil || e.Protocol != "ip" || e.Transport != "network" || e.Profile != "ipv4-fragment-extent-refusal" || e.ExpertCode != "IPv4FragmentExtentUnavailable" || e.Status != "malformed" || e.Completeness != "headers" || e.Session["Fragment Reassembly Admitted"] != false || e.Session["Child Header Identity"] != "unconfirmed" {
		return nil, false
	}
	length, observed := e.Session["Observed Total Length"]
	hlen := number(e.Session["Captured Header Bytes"])
	if !observed || number(length) != 0 || number(e.Session["Observed IP Version"]) != 4 || hlen < 20 || hlen > 60 || hlen%4 != 0 || number(e.Session["Observed Fragment Offset"]) == 0 && e.Session["Observed More Fragments"] != true {
		return nil, false
	}
	names := headerLayerNames(e.Session["Observed Header Layers"])
	return names, len(names) > 0 && len(names) <= 128 && names[len(names)-1] == "IPv4"
}

func ipv4IPIPExtentRefusal(e *pcaputil.ProtocolEvent) ([]string, bool) {
	if e == nil || e.Protocol != "ip" || e.Transport != "network" || e.Profile != "ipv4-carrier-extent-refusal" || e.ExpertCode != "IPv4CarrierExtentUnavailable" || e.Status != "malformed" || e.Completeness != "headers" || e.Session["IP-in-IP Normalization Admitted"] != false || e.Session["Network Transport Admitted"] != false || e.Session["Fragment Reassembly Admitted"] != false || e.Session["Child Header Identity"] != "unconfirmed" || e.Session["Header Observation Scope"] != "IPv4 fixed header; enclosing IP-in-IP extent unavailable" {
		return nil, false
	}
	length, observed := e.Session["Observed Total Length"]
	offset, flagsObserved := e.Session["Observed Fragment Offset"]
	hlen, next := number(e.Session["Captured Header Bytes"]), number(e.Session["Observed Next Protocol"])
	if !observed || number(length) != 0 || !flagsObserved || number(offset) != 0 || e.Session["Observed More Fragments"] != false || number(e.Session["Observed IP Version"]) != 4 || hlen < 20 || hlen > 60 || hlen%4 != 0 || next != 4 && next != 41 {
		return nil, false
	}
	names := headerLayerNames(e.Session["Observed Header Layers"])
	return names, len(names) > 0 && len(names) <= 128 && names[len(names)-1] == "IPv4"
}

// Only the actual packet can grant these literal fixed-header ranges. The
// refused envelope does not grant a transport peer, payload extent or child PDU.
func appendCapturedIPv4FragmentExtentFields(d *packetDetail, raw *capturedPacket, p gopacket.Packet, events []*pcaputil.ProtocolEvent) {
	for _, e := range events {
		names, ok := ipv4FragmentExtentRefusal(e)
		kind := "fragment"
		if !ok {
			names, ok = ipv4IPIPExtentRefusal(e)
			kind = "carrier"
		}
		if !ok || e.SourceBytes.Kind != "captured" || len(e.SourceBytes.PacketRefs) != 1 {
			continue
		}
		ref := e.SourceBytes.PacketRefs[0]
		if ref.Number != raw.number || ref.Domain != raw.reference.Domain {
			continue
		}
		offset := 0
		for i, layer := range p.Layers() {
			if i >= len(names) || layer.LayerType().String() != names[i] {
				break
			}
			h := layer.LayerContents()
			end := offset + len(h)
			if end > len(raw.data) || !bytes.Equal(h, raw.data[offset:end]) {
				break
			}
			if i != len(names)-1 {
				offset = end
				continue
			}
			_, ok := layer.(*layers.IPv4)
			if !ok || len(h) < 20 || h[0]>>4 != 4 || int(h[0]&15)*4 != len(h) || binary.BigEndian.Uint16(h[2:4]) != 0 {
				break
			}
			flags := binary.BigEndian.Uint16(h[6:8])
			identifier := binary.BigEndian.Uint16(h[4:6])
			if (flags&0x3fff != 0) != (kind == "fragment") || kind == "carrier" && number(e.Session["Observed Next Protocol"]) != uint64(h[9]) || number(e.Session["Captured Header Bytes"]) != uint64(len(h)) || number(e.Session["Observed Fragment ID"]) != uint64(identifier) || number(e.Session["Observed Fragment Offset"]) != uint64(flags&0x1fff)*8 || (e.Session["Observed More Fragments"] == true) != (flags&0x2000 != 0) {
				break
			}
			d.fields = append(d.fields,
				field{text: "Observed IPv4 header · " + kind + " extent unavailable", branch: true, start: offset, end: end, hasRange: true},
				field{depth: 1, text: "Observed Version: 4", start: offset, end: offset + 1, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed Header Length: %d bytes", len(h)), start: offset, end: offset + 1, hasRange: true},
				field{depth: 1, text: "Observed Total Length: 0 · invalid " + kind + " extent", start: offset + 2, end: offset + 4, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed Fragment ID: %d", identifier), start: offset + 4, end: offset + 6, hasRange: true},
				field{depth: 1, text: fmt.Sprintf("Observed Fragment Offset: %d bytes · MF=%t", int(flags&0x1fff)*8, flags&0x2000 != 0), start: offset + 6, end: offset + 8, hasRange: true})
			if kind == "carrier" {
				d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Declared Next Protocol: %s (%d) · child not admitted", layers.IPProtocol(h[9]), h[9]), start: offset + 9, end: offset + 10, hasRange: true})
			}
			return
		}
	}
}
