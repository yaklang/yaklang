package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type nativeNetworkField struct {
	value any
	span  DisplayFieldRange
}

// Only ingress can create this observation. Public Protocol/Session/Raw edits
// cannot create wire fields or rewrite their original source and byte ranges.
type nativeNetworkObservation struct {
	protocol, status, completeness, source, destination string
	id, flow                                            uint64
	length                                              int
	domain                                              CaptureDomain
	bytes                                               ByteSource
	values                                              map[string]nativeNetworkField
	mldValues                                           map[string][]nativeNetworkField
	igmpValues                                          map[string][]nativeNetworkField
	frozen                                              bool
}

func (e *ProtocolEvent) retainNativeMLDFields(p gopacket.Packet, result *stream_parser.MLDWireResult) {
	n := e.nativeNetworkFields
	if n == nil || n.frozen || n.protocol != "icmpv6" {
		return
	}
	layer := p.Layer(layers.LayerTypeICMPv6)
	if layer == nil {
		return
	}
	offset, ok := nativeNetworkLayerOffset(p, layer)
	if !ok {
		return
	}
	available := len(layer.LayerContents()) + len(layer.LayerPayload())
	n.mldValues = map[string][]nativeNetworkField{}
	for _, f := range result.Values {
		if f.Name == "" || f.Offset < 0 || f.Length <= 0 || f.Offset > available || f.Length > available-f.Offset {
			continue
		}
		n.mldValues[f.Name] = append(n.mldValues[f.Name], nativeNetworkField{value: f.Value, span: DisplayFieldRange{Offset: offset + f.Offset, Length: f.Length}})
	}
}

func nativeNetworkLayerOffset(p gopacket.Packet, target gopacket.Layer) (int, bool) {
	offset := 0
	for _, layer := range p.Layers() {
		h := layer.LayerContents()
		if len(h) > len(p.Data())-offset || !bytes.Equal(h, p.Data()[offset:offset+len(h)]) {
			return 0, false
		}
		if layer == target {
			return offset, true
		}
		if layer.LayerType() == gopacket.LayerTypeDecodeFailure || layer.LayerType() == gopacket.LayerTypePayload {
			return 0, false
		}
		offset += len(h)
	}
	return 0, false
}

func (e *ProtocolEvent) retainNativeNetworkFields(p gopacket.Packet) {
	var layer gopacket.Layer
	switch e.Protocol {
	case "arp":
		layer = p.Layer(layers.LayerTypeARP)
	case "icmp":
		layer = p.Layer(layers.LayerTypeICMPv4)
	case "icmpv6":
		layer = p.Layer(layers.LayerTypeICMPv6)
	default:
		return
	}
	if layer == nil {
		return
	}
	offset, ok := nativeNetworkLayerOffset(p, layer)
	if !ok {
		return
	}
	contents, payload := layer.LayerContents(), layer.LayerPayload()
	available := len(contents) + len(payload)
	if available > len(p.Data())-offset || !bytes.Equal(payload, p.Data()[offset+len(contents):offset+available]) {
		return
	}
	wire := p.Data()[offset : offset+available]
	n := &nativeNetworkObservation{protocol: e.Protocol, values: map[string]nativeNetworkField{}}
	add := func(name string, value any, at, size int) {
		if at >= 0 && size > 0 && at <= available && size <= available-at {
			n.values[name] = nativeNetworkField{value: value, span: DisplayFieldRange{Offset: offset + at, Length: size}}
		}
	}
	ipv4 := func(w []byte) string { return netip.AddrFrom4([4]byte(w)).String() }
	ipv6 := func(w []byte) string { return netip.AddrFrom16([16]byte(w)).String() }
	switch e.Protocol {
	case "arp":
		if len(contents) < 8 {
			return
		}
		hlen, plen := int(wire[4]), int(wire[5])
		if len(contents) < 8+2*(hlen+plen) {
			return
		}
		add("arp.opcode", binary.BigEndian.Uint16(wire[6:8]), 6, 2)
		if hlen == 6 {
			add("arp.src.hw_mac", net.HardwareAddr(wire[8:14]).String(), 8, 6)
		}
		if plen == 4 && binary.BigEndian.Uint16(wire[2:4]) == uint16(layers.EthernetTypeIPv4) {
			at := 8 + hlen
			add("arp.src.proto_ipv4", ipv4(wire[at:at+4]), at, 4)
			at = 8 + 2*hlen + plen
			add("arp.dst.proto_ipv4", ipv4(wire[at:at+4]), at, 4)
		}
	case "icmp":
		if len(contents) != 8 {
			return
		}
		typ, code := wire[0], wire[1]
		add("icmp.type", typ, 0, 1)
		add("icmp.code", code, 1, 1)
		switch typ {
		case 0, 8, 13, 14, 15, 16, 17, 18:
			add("icmp.ident", binary.BigEndian.Uint16(wire[4:6]), 4, 2)
			add("icmp.seq", binary.BigEndian.Uint16(wire[6:8]), 6, 2)
		}
		if typ == 3 && code == 4 {
			add("icmp.mtu", binary.BigEndian.Uint16(wire[6:8]), 6, 2)
		}
		if typ == 5 {
			add("icmp.redir_gw", ipv4(wire[4:8]), 4, 4)
		}
		if typ == 12 {
			add("icmp.pointer", wire[4], 4, 1)
		}
		if typ == 13 || typ == 14 {
			for i, name := range []string{"icmp.originate_timestamp", "icmp.receive_timestamp", "icmp.transmit_timestamp"} {
				at := 8 + i*4
				if available >= at+4 {
					add(name, binary.BigEndian.Uint32(wire[at:at+4]), at, 4)
				}
			}
		}
	case "icmpv6":
		if len(contents) != 4 {
			return
		}
		typ := wire[0]
		add("icmpv6.type", typ, 0, 1)
		add("icmpv6.code", wire[1], 1, 1)
		if (typ == 128 || typ == 129) && available >= 8 {
			add("icmpv6.echo.identifier", binary.BigEndian.Uint16(wire[4:6]), 4, 2)
			add("icmpv6.echo.sequence_number", binary.BigEndian.Uint16(wire[6:8]), 6, 2)
		}
		if typ == 2 && available >= 8 {
			add("icmpv6.mtu", binary.BigEndian.Uint32(wire[4:8]), 4, 4)
		}
		if available >= 24 {
			switch typ {
			case 135:
				add("icmpv6.nd.ns.target_address", ipv6(wire[8:24]), 8, 16)
			case 136:
				add("icmpv6.nd.na.target_address", ipv6(wire[8:24]), 8, 16)
			case 137:
				add("icmpv6.nd.rd.target_address", ipv6(wire[8:24]), 8, 16)
			}
		}
		if typ == 137 && available >= 40 {
			add("icmpv6.rd.na.destination_address", ipv6(wire[24:40]), 24, 16)
		}
	}
	e.nativeNetworkFields = n
}

func cloneNativeNetworkSource(s ByteSource) ByteSource {
	s.PacketRefs = append([]PacketReference(nil), s.PacketRefs...)
	s.ParentPDUs = append([]uint64(nil), s.ParentPDUs...)
	if s.CompletedIn != nil {
		ref := *s.CompletedIn
		s.CompletedIn = &ref
	}
	return s
}

func (e *ProtocolEvent) freezeNativeNetworkFields() {
	n := e.nativeNetworkFields
	if n == nil || n.frozen {
		return
	}
	n.status, n.completeness, n.source, n.destination = e.Status, e.Completeness, e.Source, e.Destination
	n.id, n.flow, n.length, n.domain = e.ID, e.FlowID, e.Length, e.Domain
	n.bytes = cloneNativeNetworkSource(e.SourceBytes)
	n.frozen = true
}

func (e *ProtocolEvent) ownedNativeNetworkField(name string) (nativeNetworkField, bool) {
	if e.nativeNetworkFields == nil || !e.nativeNetworkFields.frozen {
		return nativeNetworkField{}, false
	}
	f, ok := e.nativeNetworkFields.values[name]
	return f, ok
}

func nativeNetworkDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		protocol, name, label string
		typ                   DisplayFieldType
		bits                  int
	}{
		{"arp", "arp.opcode", "ARP opcode", DisplayUnsigned, 16},
		{"arp", "arp.src.hw_mac", "ARP sender MAC", DisplayString, 0},
		{"arp", "arp.src.proto_ipv4", "ARP sender IPv4", DisplayAddress, 32},
		{"arp", "arp.dst.proto_ipv4", "ARP target IPv4", DisplayAddress, 32},
		{"icmp", "icmp.type", "ICMP type", DisplayUnsigned, 8},
		{"icmp", "icmp.code", "ICMP code", DisplayUnsigned, 8},
		{"icmp", "icmp.ident", "ICMP identifier", DisplayUnsigned, 16},
		{"icmp", "icmp.seq", "ICMP sequence", DisplayUnsigned, 16},
		{"icmp", "icmp.mtu", "ICMP next-hop MTU", DisplayUnsigned, 16},
		{"icmp", "icmp.redir_gw", "ICMP redirect gateway", DisplayAddress, 32},
		{"icmp", "icmp.pointer", "ICMP parameter pointer", DisplayUnsigned, 8},
		{"icmp", "icmp.originate_timestamp", "ICMP originate timestamp word", DisplayUnsigned, 32},
		{"icmp", "icmp.receive_timestamp", "ICMP receive timestamp word", DisplayUnsigned, 32},
		{"icmp", "icmp.transmit_timestamp", "ICMP transmit timestamp word", DisplayUnsigned, 32},
		{"icmpv6", "icmpv6.type", "ICMPv6 type", DisplayUnsigned, 8},
		{"icmpv6", "icmpv6.code", "ICMPv6 code", DisplayUnsigned, 8},
		{"icmpv6", "icmpv6.echo.identifier", "ICMPv6 echo identifier", DisplayUnsigned, 16},
		{"icmpv6", "icmpv6.echo.sequence_number", "ICMPv6 echo sequence", DisplayUnsigned, 16},
		{"icmpv6", "icmpv6.mtu", "ICMPv6 MTU", DisplayUnsigned, 32},
		{"icmpv6", "icmpv6.nd.ns.target_address", "ICMPv6 solicitation target", DisplayAddress, 128},
		{"icmpv6", "icmpv6.nd.na.target_address", "ICMPv6 advertisement target", DisplayAddress, 128},
		{"icmpv6", "icmpv6.nd.rd.target_address", "ICMPv6 redirect target", DisplayAddress, 128},
		{"icmpv6", "icmpv6.rd.na.destination_address", "ICMPv6 redirect destination", DisplayAddress, 128},
	} {
		spec := spec
		b := displayCustom(spec.protocol, spec.name, spec.label, spec.typ, spec.bits, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if f, ok := e.ownedNativeNetworkField(spec.name); ok {
				emit(f.value)
			}
		})
		b.rangeKey = spec.name
		b.descriptor.Description = "Private native header value at its exact logical-PDU byte range; type and available-byte gated. No inferred neighbor trust, checksum verification or absolute timestamp."
		out = append(out, b)
	}
	return out
}
