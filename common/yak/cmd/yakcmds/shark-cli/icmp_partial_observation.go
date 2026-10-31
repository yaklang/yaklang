package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Suppress legacy fallback only for this exact physical message and a proved
// native body/option refusal. A profile label or a matching type alone is not
// enough; quotations and reassembled messages have different byte sources.
func matchedICMPv6PartialObservation(p gopacket.Packet, events []*pcaputil.ProtocolEvent, number uint64, domain pcaputil.CaptureDomain) bool {
	icmp, ok := p.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6)
	if !ok || len(icmp.LayerContents()) != 4 {
		return false
	}
	for _, l := range p.Layers() {
		if l == icmp {
			break
		}
		if l.LayerType() == layers.LayerTypeICMPv4 || l.LayerType() == layers.LayerTypeTCP || l.LayerType() == layers.LayerTypeUDP {
			return false
		}
	}
	typ, code, b := icmp.TypeCode.Type(), icmp.TypeCode.Code(), icmp.Payload
	fixed := 0
	switch typ {
	case 1, 2, 3, 4, 128, 129, 133:
		fixed = 4
	case 134:
		fixed = 12
	case 135, 136:
		fixed = 20
	case 137:
		fixed = 36
	default:
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "icmpv6" || e.Transport != "network" || e.Profile != "icmpv6-header-observation" || e.Completeness != e.Status || e.Error != e.ExpertCode || !bytes.Equal(e.Raw, p.Data()) || e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(p.Data())) {
			continue
		}
		refs := e.SourceBytes.PacketRefs
		if e.SourceBytes.Kind != "captured" || len(refs) != 1 || refs[0].Number != number || refs[0].Domain != domain {
			continue
		}
		t, tok := gpeHeaderInteger(e.Session["Type"], 255)
		c, cok := gpeHeaderInteger(e.Session["Code"], 255)
		if !tok || !cok || t != uint64(typ) || c != uint64(code) {
			continue
		}
		if typ == 135 || typ == 136 || typ == 137 {
			target, present := e.Session["Target"]
			if len(b) >= 20 {
				if target != net.IP(b[4:20]).String() {
					continue
				}
			} else if present {
				continue
			}
		}
		if typ == 137 {
			destination, present := e.Session["Destination"]
			if len(b) >= 36 {
				if destination != net.IP(b[20:36]).String() {
					continue
				}
			} else if present {
				continue
			}
		}
		if e.Status == "incomplete" && e.ExpertCode == "ICMPv6HeaderTruncated" && len(b) < fixed {
			available, aok := gpeHeaderInteger(e.Session["Available ICMPv6 Bytes"], 65535)
			required, rok := gpeHeaderInteger(e.Session["Required ICMPv6 Header Bytes"], 65535)
			if !aok || !rok || available != uint64(4+len(b)) || required != uint64(4+fixed) {
				continue
			}
			absent := true
			for _, key := range []string{"Identifier", "Sequence", "MTU", "Quoted Packet", "Options"} {
				if _, exists := e.Session[key]; exists {
					absent = false
				}
			}
			if absent {
				return true
			}
		}
		if e.Status == "malformed" && e.ExpertCode == "NDOptionLength" && typ >= 133 && typ <= 137 && len(b) >= fixed {
			if icmpv6InvalidOptionsMatch(e.Session["Options"], b[fixed:]) {
				return true
			}
		}
	}
	return false
}

func icmpv6InvalidOptionsMatch(value any, wire []byte) bool {
	count := 0
	var atIndex func(int) any
	switch v := value.(type) {
	case nil:
		atIndex = func(int) any { return nil }
	case []any:
		count, atIndex = len(v), func(i int) any { return v[i] }
	case []map[string]any:
		count, atIndex = len(v), func(i int) any { return v[i] }
	default:
		return false
	}
	for i := 0; len(wire) > 0; i++ {
		if len(wire) < 2 || wire[1] == 0 || int(wire[1])*8 > len(wire) {
			return i == count
		}
		if i >= count {
			return false
		}
		option, ok := atIndex(i).(map[string]any)
		if !ok || len(option) != 2 {
			return false
		}
		typ, ok := gpeHeaderInteger(option["Type"], 255)
		n := int(wire[1]) * 8
		if !ok || typ != uint64(wire[0]) || !bytes.Equal(bytesValue(option["Value"]), wire[2:n]) {
			return false
		}
		wire = wire[n:]
	}
	return false // A valid options tail does not justify a malformed diagnostic.
}

func appendICMPv6PartialCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	if !matchedICMPv6PartialObservation(p, events, raw.number, raw.reference.Domain) {
		return false
	}
	var names []string
	for _, layer := range p.Layers() {
		if layer.LayerType() == layers.LayerTypeICMPv6 {
			break
		}
		names = append(names, layer.LayerType().String())
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

func icmpv6PartialDecoderCovered(p gopacket.Packet, events []*pcaputil.ProtocolEvent, number uint64, domain pcaputil.CaptureDomain) bool {
	if p.ErrorLayer() == nil || !matchedICMPv6PartialObservation(p, events, number, domain) {
		return false
	}
	icmp := p.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6)
	return bytes.Equal(p.ErrorLayer().LayerContents(), icmp.Payload)
}
