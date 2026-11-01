package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func matchedICMPv4PartialObservation(p gopacket.Packet, events []*pcaputil.ProtocolEvent, number uint64, domain pcaputil.CaptureDomain) bool {
	icmp, ok := p.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)
	if !ok || len(icmp.LayerContents()) != 8 {
		return false
	}
	for _, layer := range p.Layers() {
		if layer == icmp {
			break
		}
		if layer.LayerType() == layers.LayerTypeICMPv6 || layer.LayerType() == layers.LayerTypeTCP || layer.LayerType() == layers.LayerTypeUDP {
			return false
		}
	}
	typ, code, body := icmp.TypeCode.Type(), icmp.TypeCode.Code(), icmp.Payload
	required := 20
	if typ == 17 || typ == 18 {
		required = 12
	} else if typ != 13 && typ != 14 {
		return false
	}
	if len(body)+8 >= required {
		return false
	}
	for _, e := range events {
		if e == nil || e.Protocol != "icmp" || e.Transport != "network" || e.Profile != "icmp-header-observation" || e.Status != "incomplete" || e.Completeness != "incomplete" || e.ExpertCode != "ICMPHeaderTruncated" || e.Error != e.ExpertCode || !bytes.Equal(e.Raw, p.Data()) || e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(p.Data())) {
			continue
		}
		refs := e.SourceBytes.PacketRefs
		if e.SourceBytes.Kind != "captured" || len(refs) != 1 || refs[0].Number != number || refs[0].Domain != domain {
			continue
		}
		t, tok := gpeHeaderInteger(e.Session["Type"], 255)
		c, cok := gpeHeaderInteger(e.Session["Code"], 255)
		available, aok := gpeHeaderInteger(e.Session["Available ICMP Bytes"], 65535)
		need, nok := gpeHeaderInteger(e.Session["Required ICMP Header Bytes"], 65535)
		id, iok := gpeHeaderInteger(e.Session["Identifier"], 65535)
		seq, sok := gpeHeaderInteger(e.Session["Sequence"], 65535)
		if !tok || !cok || !aok || !nok || !iok || !sok || t != uint64(typ) || c != uint64(code) || available != uint64(8+len(body)) || need != uint64(required) || id != uint64(icmp.Id) || seq != uint64(icmp.Seq) {
			continue
		}
		valid := true
		for i, key := range []string{"Originate Timestamp", "Receive Timestamp", "Transmit Timestamp"} {
			value, present := e.Session[key]
			if typ < 17 && len(body) >= (i+1)*4 {
				word, ok := gpeHeaderInteger(value, 0xffffffff)
				if !ok || word != uint64(binary.BigEndian.Uint32(body[i*4:(i+1)*4])) {
					valid = false
				}
			} else if present {
				valid = false
			}
		}
		for _, key := range []string{"Address Mask", "MTU", "Gateway", "Pointer", "Rest of Header", "Quoted Packet"} {
			if _, present := e.Session[key]; present {
				valid = false
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func appendICMPv4PartialCapturedPrefix(d *packetDetail, raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{NoCopy: true})
	if !matchedICMPv4PartialObservation(p, events, raw.number, raw.reference.Domain) {
		return false
	}
	var names []string
	for _, layer := range p.Layers() {
		if layer.LayerType() == layers.LayerTypeICMPv4 {
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
