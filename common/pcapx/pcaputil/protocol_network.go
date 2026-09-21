package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

type fragmentKey struct {
	domain   CaptureDomain
	src, dst string
	id       uint32
	protocol byte
	v6       bool
}
type ipFragment struct {
	start int
	data  []byte
	ref   PacketReference
}
type fragmentSet struct {
	parts []ipFragment
	end   int
	last  time.Time
	cost  int64
}
type fragmentStore struct {
	mu    sync.Mutex
	sets  map[fragmentKey]*fragmentSet
	clock time.Time
}

func (a *binParser) networkPacket(p gopacket.Packet) (gopacket.Packet, bool) {
	e := packetEvidence(p)
	ci := p.Metadata().CaptureInfo
	// gopacket exposes the outer NetworkLayer and TransportLayer. Normalize a
	// supported tunnel to its innermost IP before transport dispatch.
	var inner gopacket.NetworkLayer
	networks, tunnels := 0, 0
	for _, l := range p.Layers() {
		if n, ok := l.(gopacket.NetworkLayer); ok {
			inner = n
			networks++
		}
		switch l.(type) {
		case *layers.GRE, *layers.VXLAN:
			tunnels++
		}
	}
	if networks > 1 {
		if tunnels == 0 || tunnels > 4 || networks != tunnels+1 {
			a.networkDiagnostic(e, ci, "limited", "EncapsulationDepthOrProfile")
			return nil, true
		}
		raw := append(bytes.Clone(inner.LayerContents()), inner.LayerPayload()...)
		ci.CaptureLength, ci.Length = len(raw), len(raw)
		ci = withEvidence(ci, e)
		p = gopacket.NewPacket(raw, layers.LinkTypeRaw, gopacket.Default)
		p.Metadata().CaptureInfo = ci
	}
	var key fragmentKey
	var payload []byte
	off := -1
	more := false
	for _, l := range p.Layers() {
		switch v := l.(type) {
		case *layers.IPv4:
			if v.FragOffset != 0 || v.Flags&layers.IPv4MoreFragments != 0 {
				key = fragmentKey{e.Ref.Domain, v.SrcIP.String(), v.DstIP.String(), uint32(v.Id), byte(v.Protocol), false}
				off = int(v.FragOffset) * 8
				more = v.Flags&layers.IPv4MoreFragments != 0
				payload = v.Payload
			}
		case *layers.IPv6Fragment:
			ip, ok := p.NetworkLayer().(*layers.IPv6)
			if !ok {
				return nil, true
			}
			key = fragmentKey{e.Ref.Domain, ip.SrcIP.String(), ip.DstIP.String(), v.Identification, byte(v.NextHeader), true}
			off = int(v.FragmentOffset) * 8
			more = v.MoreFragments
			payload = v.Payload
		}
	}
	if off >= 0 {
		return a.fragment(p, e, key, off, more, payload)
	}
	var protocol string
	var fields map[string]any
	if arp, ok := p.Layer(layers.LayerTypeARP).(*layers.ARP); ok {
		protocol = "arp"
		fields = map[string]any{"Operation": arp.Operation, "Sender MAC": net.HardwareAddr(arp.SourceHwAddress).String(), "Sender IP": net.IP(arp.SourceProtAddress).String(), "Target IP": net.IP(arp.DstProtAddress).String(), "Observation": "unverified-neighbor"}
	} else if icmp, ok := p.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6); ok {
		protocol = "icmpv6"
		fields = map[string]any{"Type": icmp.TypeCode.Type(), "Code": icmp.TypeCode.Code()}
		typ := icmp.TypeCode.Type()
		b := icmp.Payload
		fixed := 0
		switch typ {
		case 133:
			fixed = 4
		case 134:
			fixed = 12
		case 135, 136:
			fixed = 20
			if len(b) >= 20 {
				fields["Target"] = net.IP(b[4:20]).String()
			}
		case 1, 2, 3, 4:
			fixed = len(b)
			if len(b) >= 4 {
				fields["Quoted Packet"] = bytes.Clone(b[4:])
				if typ == 2 {
					fields["MTU"] = binary.BigEndian.Uint32(b)
				}
			}
		case 128, 129:
			fixed = len(b)
			if len(b) >= 4 {
				fields["Identifier"], fields["Sequence"] = binary.BigEndian.Uint16(b), binary.BigEndian.Uint16(b[2:])
			}
		}
		if fixed > len(b) {
			a.networkDiagnostic(e, ci, "incomplete", "ICMPv6HeaderTruncated")
			return nil, true
		}
		if typ >= 133 && typ <= 136 {
			var opts []map[string]any
			for at := fixed; at < len(b); {
				if len(b)-at < 2 || b[at+1] == 0 || int(b[at+1])*8 > len(b)-at {
					a.networkDiagnostic(e, ci, "malformed", "NDOptionLength")
					return nil, true
				}
				n := int(b[at+1]) * 8
				if len(opts) >= a.budget.MaxCollectionElements {
					a.networkDiagnostic(e, ci, "limited", "NDOptionBudget")
					return nil, true
				}
				opts = append(opts, map[string]any{"Type": b[at], "Value": bytes.Clone(b[at+2 : at+n])})
				at += n
			}
			fields["Options"] = opts
			fields["Observation"] = "unverified-neighbor"
		}
	} else if icmp, ok := p.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4); ok {
		protocol = "icmp"
		fields = map[string]any{"Type": icmp.TypeCode.Type(), "Code": icmp.TypeCode.Code(), "Identifier": icmp.Id, "Sequence": icmp.Seq}
		if icmp.TypeCode.Type() == 3 || icmp.TypeCode.Type() == 11 {
			fields["Quoted Packet"] = bytes.Clone(icmp.Payload)
		}
	}
	if protocol != "" {
		event := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: protocol, Domain: e.Ref.Domain, Length: len(p.Data()), Raw: bytes.Clone(p.Data()), Status: "decoded", Completeness: "message", Session: fields, SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{e.Ref}}, semanticFields: fields}
		if n := p.NetworkLayer(); n != nil {
			event.Source, event.Destination = n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
		}
		event.Structured = map[string]any{"fields": fields}
		a.messages.Add(1)
		a.decoded.Add(1)
		a.emit(event)
		return nil, true
	}
	return p, false
}
func (a *binParser) networkDiagnostic(e captureEvidence, ci gopacket.CaptureInfo, status, code string) {
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Domain: e.Ref.Domain, Status: status, Completeness: status, ExpertCode: code, Error: code, SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{e.Ref}}})
}
func (a *binParser) fragment(p gopacket.Packet, e captureEvidence, key fragmentKey, off int, more bool, payload []byte) (gopacket.Packet, bool) {
	s := &a.fragments
	s.mu.Lock()
	defer s.mu.Unlock()
	ci := p.Metadata().CaptureInfo
	if s.sets == nil {
		s.sets = map[fragmentKey]*fragmentSet{}
	}
	if ci.Timestamp.After(s.clock) {
		s.clock = ci.Timestamp
	}
	for k, v := range s.sets {
		if s.clock.Sub(v.last) > 30*time.Second {
			a.buffered.Add(-v.cost)
			delete(s.sets, k)
			a.fragmentDiagnostic(v, "FragmentTimeout")
		}
	}
	invalid := func(code string) (gopacket.Packet, bool) {
		if v := s.sets[key]; v != nil {
			a.buffered.Add(-v.cost)
			delete(s.sets, key)
		}
		a.networkDiagnostic(e, ci, "malformed", code)
		return nil, true
	}
	if ci.CaptureLength < ci.Length || off+len(payload) > 65535 || len(payload) == 0 || more && len(payload)%8 != 0 {
		return invalid("FragmentLength")
	}
	v := s.sets[key]
	if v == nil {
		if len(s.sets) >= a.budget.MaxCollectionElements {
			a.networkDiagnostic(e, ci, "limited", "FragmentSessionBudget")
			return nil, true
		}
		v = &fragmentSet{end: -1}
		s.sets[key] = v
	}
	for _, part := range v.parts {
		if !key.v6 && off == part.start && bytes.Equal(part.data, payload) {
			return nil, true
		}
		if off < part.start+len(part.data) && part.start < off+len(payload) {
			return invalid("FragmentOverlapRejected")
		}
	}
	if len(v.parts) >= min(128, a.budget.MaxCollectionElements) {
		return invalid("FragmentRangeBudget")
	}
	if v.end >= 0 && off+len(payload) > v.end {
		return invalid("FragmentBeyondEnd")
	}
	if !more {
		if v.end >= 0 && v.end != off+len(payload) {
			return invalid("FragmentConflictingEnd")
		}
		for _, part := range v.parts {
			if part.start+len(part.data) > off+len(payload) {
				return invalid("FragmentBeyondEnd")
			}
		}
		v.end = off + len(payload)
	}
	cost := int64(len(payload) + 128)
	if !a.reserveEvidence(cost) {
		if len(v.parts) == 0 {
			delete(s.sets, key)
		}
		a.networkDiagnostic(e, ci, "limited", "FragmentByteBudget")
		return nil, true
	}
	v.cost += cost
	v.last = s.clock
	v.parts = append(v.parts, ipFragment{off, bytes.Clone(payload), e.Ref})
	sort.Slice(v.parts, func(i, j int) bool { return v.parts[i].start < v.parts[j].start })
	cursor := 0
	for _, part := range v.parts {
		if part.start != cursor {
			return nil, true
		}
		cursor += len(part.data)
	}
	if cursor != v.end {
		return nil, true
	}
	raw := make([]byte, 0, cursor)
	refs := make([]PacketReference, 0, len(v.parts))
	for _, part := range v.parts {
		raw = append(raw, part.data...)
		refs = append(refs, part.ref)
	}
	a.buffered.Add(-v.cost)
	delete(s.sets, key)
	var ip gopacket.SerializableLayer
	if key.v6 {
		ip = &layers.IPv6{Version: 6, SrcIP: net.ParseIP(key.src), DstIP: net.ParseIP(key.dst), NextHeader: layers.IPProtocol(key.protocol), HopLimit: 64}
	} else {
		ip = &layers.IPv4{Version: 4, IHL: 5, SrcIP: net.ParseIP(key.src), DstIP: net.ParseIP(key.dst), Protocol: layers.IPProtocol(key.protocol), TTL: 64, Id: uint16(key.id)}
	}
	b := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, gopacket.Payload(raw)); err != nil {
		return invalid("FragmentSerialization")
	}
	q := gopacket.NewPacket(b.Bytes(), layers.LinkTypeRaw, gopacket.Default)
	ci.CaptureLength, ci.Length = len(b.Bytes()), len(b.Bytes())
	e.refs = refs
	ci = withEvidence(ci, e)
	q.Metadata().CaptureInfo = ci
	return q, false
}
func (a *binParser) closeFragments() {
	s := &a.fragments
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.sets {
		a.buffered.Add(-v.cost)
		a.fragmentDiagnostic(v, "FragmentCaptureEnded")
	}
	s.sets = nil
}
func (a *binParser) fragmentDiagnostic(v *fragmentSet, code string) {
	if len(v.parts) == 0 {
		return
	}
	refs := make([]PacketReference, 0, len(v.parts))
	for _, p := range v.parts {
		refs = append(refs, p.ref)
	}
	a.emit(&ProtocolEvent{Timestamp: v.last, Transport: "network", Protocol: "ip", Domain: refs[0].Domain, Status: "incomplete", Completeness: "incomplete", ExpertCode: code, Error: code, SourceBytes: ByteSource{Kind: "reassembled", PacketRefs: refs}})
}

func decodeDHCPv6(w []byte, depth, limit int) (map[string]any, error) {
	if depth > 8 || len(w) < 4 {
		return nil, fmt.Errorf("DHCPv6 depth/header")
	}
	typ := w[0]
	out := map[string]any{"Message Type": typ, "Observation": "observed"}
	at := 4
	if typ == 12 || typ == 13 {
		if len(w) < 34 {
			return nil, fmt.Errorf("DHCPv6 relay header")
		}
		out["Hop Count"], out["Link Address"], out["Peer Address"] = w[1], net.IP(w[2:18]).String(), net.IP(w[18:34]).String()
		at = 34
	} else {
		out["Transaction ID"] = uint32(w[1])<<16 | uint32(w[2])<<8 | uint32(w[3])
	}
	options, err := dhcp6Options(w[at:], depth, limit)
	out["Options"] = options
	return out, err
}
func dhcp6Options(w []byte, depth, limit int) ([]map[string]any, error) {
	if depth > 8 {
		return nil, protocolError(ErrResourceExceeded, "DHCPv6 option depth")
	}
	var out []map[string]any
	for len(w) > 0 {
		if len(w) < 4 {
			return nil, fmt.Errorf("DHCPv6 option header")
		}
		code, n := binary.BigEndian.Uint16(w), int(binary.BigEndian.Uint16(w[2:]))
		w = w[4:]
		if n > len(w) {
			return nil, fmt.Errorf("DHCPv6 option length")
		}
		b := w[:n]
		w = w[n:]
		if len(out) >= limit {
			return nil, protocolError(ErrResourceExceeded, "DHCPv6 option count")
		}
		o := map[string]any{"Code": code, "Raw": bytes.Clone(b)}
		prefix := -1
		switch code {
		case 1, 2:
			if n < 2 {
				return nil, fmt.Errorf("DHCPv6 DUID length")
			}
			kind := binary.BigEndian.Uint16(b)
			if kind == 1 && n < 9 || kind == 2 && n < 7 || kind == 3 && n < 5 || kind == 4 && n != 18 {
				return nil, fmt.Errorf("DHCPv6 DUID profile length")
			}
			o["DUID Type"] = kind
			o["DUID"] = bytes.Clone(b)
		case 3, 25:
			if n < 12 {
				return nil, fmt.Errorf("DHCPv6 IA length")
			}
			o["IAID"], o["T1"], o["T2"] = binary.BigEndian.Uint32(b), binary.BigEndian.Uint32(b[4:]), binary.BigEndian.Uint32(b[8:])
			prefix = 12
		case 5:
			if n < 24 {
				return nil, fmt.Errorf("DHCPv6 IAADDR length")
			}
			o["Address"], o["Preferred Lifetime"], o["Valid Lifetime"] = net.IP(b[:16]).String(), binary.BigEndian.Uint32(b[16:]), binary.BigEndian.Uint32(b[20:])
			prefix = 24
		case 26:
			if n < 25 || b[8] > 128 {
				return nil, fmt.Errorf("DHCPv6 IAPREFIX length")
			}
			o["Prefix Length"], o["Prefix"], o["Preferred Lifetime"], o["Valid Lifetime"] = b[8], net.IP(b[9:25]).String(), binary.BigEndian.Uint32(b), binary.BigEndian.Uint32(b[4:])
			prefix = 25
		case 13:
			if n < 2 {
				return nil, fmt.Errorf("DHCPv6 status length")
			}
			o["Status"], o["Message"] = binary.BigEndian.Uint16(b), string(b[2:])
		case 9:
			inner, err := decodeDHCPv6(b, depth+1, limit)
			if err != nil {
				return nil, err
			}
			o["Relay Message"] = inner
		}
		if prefix >= 0 {
			inner, err := dhcp6Options(b[prefix:], depth+1, limit)
			if err != nil {
				return nil, err
			}
			o["Options"] = inner
		}
		out = append(out, o)
	}
	return out, nil
}

func unsupportedNetworkDecode(p gopacket.Packet) bool {
	if p.Layer(layers.LayerTypeTCP) != nil {
		return false
	}
	if p.Layer(layers.LayerTypeICMPv4) != nil || p.Layer(layers.LayerTypeICMPv6) != nil {
		return true
	}
	if e := p.ErrorLayer(); e != nil {
		message := e.Error().Error()
		return message == "Layer type not currently supported" || strings.HasPrefix(message, "Unable to decode LinkType ")
	}
	return false
}
