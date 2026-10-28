package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	parents []uint64
	start   int
	data    []byte
	refs    []PacketReference
	more    bool
}
type fragmentSet struct {
	parts    []ipFragment
	header   []byte
	end      int
	last     time.Time
	cost     int64
	rejected bool
}
type fragmentStore struct {
	mu            sync.Mutex
	sets          map[fragmentKey]*fragmentSet
	blocked       bool
	blockedDomain CaptureDomain
	blockedUntil  time.Time
}

func networkCarrierProfile(p gopacket.Packet) (inner gopacket.NetworkLayer, networks, tunnels int, unsupported bool) {
	for _, l := range p.Layers() {
		if n, ok := l.(gopacket.NetworkLayer); ok {
			inner = n
			networks++
		}
		switch v := l.(type) {
		case *capturePIM:
			if v.Type == 1 && !v.NullRegister {
				tunnels++
			}
		case *layers.GRE, *layers.VXLAN:
			tunnels++
		case *layers.Geneve:
			tunnels++
			// Only option-free v0 Ethernet data is selected for normalization.
			header := v.LayerContents()
			if len(header) != 8 || header[0] != 0 || header[1] != 0 || header[7] != 0 || v.Protocol != layers.EthernetTypeTransparentEthernetBridging {
				unsupported = true
			}
		case *layers.IPv4:
			if v.FragOffset == 0 && v.Flags&layers.IPv4MoreFragments == 0 && (v.Protocol == layers.IPProtocolIPv4 || v.Protocol == layers.IPProtocolIPv6) {
				tunnels++
			}
		case *layers.IPv6:
			if v.NextHeader == layers.IPProtocolIPv4 || v.NextHeader == layers.IPProtocolIPv6 {
				tunnels++
			}
		}
	}
	return
}

func outerNetworkHeaderNames(p gopacket.Packet) []string {
	e := evidenceFrom(p.Metadata().CaptureInfo)
	names := make([]string, 0, len(e.networkPrefix)+8)
	for _, typ := range e.networkPrefix {
		names = append(names, typ.String())
	}
	for _, layer := range p.Layers() {
		if layer.LayerType() == gopacket.LayerTypeDecodeFailure || layer.LayerType() == gopacket.LayerTypePayload {
			break
		}
		if len(names) >= maxCaptureTransportLayers {
			return nil
		}
		names = append(names, layer.LayerType().String())
		if _, network := layer.(gopacket.NetworkLayer); network {
			break
		}
	}
	return names
}

func (a *binParser) refuseUnselectedNetworkParent(p gopacket.Packet, e captureEvidence, ci gopacket.CaptureInfo) {
	names := outerNetworkHeaderNames(p)
	fields := map[string]any{"Observed Header Layers": names, "Header Observation Scope": "outer network prefix only", "Child Header Identity": "unconfirmed", "Encapsulation Admission": "unselected profile"}
	// Narrow the displayed interpretation, never the observed source identity.
	// The zero flow IDs and explicit refusal admit no child connection state.
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: e.Ref.Domain, Status: "limited", Completeness: "limited", ExpertCode: "EncapsulationDepthOrProfile", Error: "EncapsulationDepthOrProfile", Summary: "Encapsulation profile unavailable; inner identity unconfirmed", Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
}

func (a *binParser) networkPacket(p gopacket.Packet) (gopacket.Packet, bool) {
	e := packetEvidence(p)
	ci := p.Metadata().CaptureInfo
	if failure := p.ErrorLayer(); failure != nil {
		var truncated *ipHeaderTruncatedError
		if errors.As(failure.Error(), &truncated) {
			_, networks, tunnels, unsupported := networkCarrierProfile(p)
			if networks > 1 && (unsupported || tunnels == 0 || tunnels+e.normalizedTunnels > 4 || networks != tunnels+1) {
				a.refuseUnselectedNetworkParent(p, e, ci)
				return nil, true
			}
			fields := map[string]any{"Expected IP Version": truncated.expected, "Available Header Bytes": truncated.available, "Required Header Bytes": truncated.required, "Header Observation Scope": "incomplete fixed header; declared carrier only"}
			if truncated.hasVersion {
				fields["Observed IP Version"] = truncated.observed
			}
			names := make([]string, 0, len(e.networkPrefix)+8)
			for _, typ := range e.networkPrefix {
				names = append(names, typ.String())
			}
			for _, layer := range p.Layers() {
				if layer.LayerType() == gopacket.LayerTypeDecodeFailure || layer.LayerType() == gopacket.LayerTypePayload {
					break
				}
				if len(names) >= maxCaptureTransportLayers {
					names = nil
					fields["Header Layer Projection Limited"] = true
					break
				}
				names = append(names, layer.LayerType().String())
			}
			if e.networkPrefixLimited {
				fields["Header Layer Projection Limited"] = true
			}
			if len(names) > 0 {
				fields["Observed Header Layers"] = names
			}
			summary := fmt.Sprintf("IPv%d header incomplete: %d of %d fixed bytes available; transport not admitted", truncated.expected, truncated.available, truncated.required)
			a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Domain: e.Ref.Domain, Status: "incomplete", Completeness: "incomplete", ExpertCode: "IPHeaderTruncated", Error: failure.Error().Error(), Summary: summary, Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
			// Keep the original error-bearing packet for capture-level accounting.
			// No normalization, fragment clock or application parser sees it here;
			// the existing transport ingress rejects its decoder error as before.
			return p, false
		}
		var version *ipVersionError
		if errors.As(failure.Error(), &version) {
			_, networks, tunnels, unsupported := networkCarrierProfile(p)
			if networks > 1 && (unsupported || tunnels == 0 || tunnels+e.normalizedTunnels > 4 || networks != tunnels+1) {
				a.refuseUnselectedNetworkParent(p, e, ci)
				return nil, true
			}
			// A decoder's partial layer cannot admit a fragment/tunnel/native
			// message, advance clocks, or create an application connection.
			// Keep only the validated outer domain and literal physical source.
			fields := map[string]any{"Observed IP Version": version.observed, "Expected IP Version": version.expected}
			if e.networkPrefixLimited {
				fields["Header Layer Projection Limited"] = true
			}
			names := make([]string, 0, len(e.networkPrefix)+8)
			for _, typ := range e.networkPrefix {
				names = append(names, typ.String())
			}
			for _, layer := range p.Layers() {
				if layer.LayerType() == gopacket.LayerTypeDecodeFailure || layer.LayerType() == gopacket.LayerTypePayload {
					break
				}
				if len(names) >= maxCaptureTransportLayers {
					names = nil
					fields["Header Layer Projection Limited"] = true
					break
				}
				names = append(names, layer.LayerType().String())
			}
			if len(names) > 0 {
				fields["Observed Header Layers"] = names
			}
			a.malformed.Add(1)
			a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Domain: e.Ref.Domain, Status: "malformed", Completeness: "malformed", ExpertCode: "IPVersionMismatch", Error: version.Error(), Summary: version.Error(), Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
			return nil, true
		}
	}
	if a.observeSTP(p, e) {
		return nil, true
	}
	if a.observeISIS(p, e) {
		return nil, true
	}
	if decoded := p.Layers(); len(decoded) != 0 {
		if can, ok := decoded[0].(*socketCANLayer); ok {
			a.decodeCANRecord(can.wire, e, ci)
			return nil, true
		}
	}
	if eth, ok := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet); ok {
		etherType, payload := eth.EthernetType, eth.Payload
		// Follow the Ethernet carrier through 802.1Q / 802.1ad tags. Each
		// iteration consumes four captured bytes, including the next EtherType;
		// packetEvidence already retains the VLAN IDs in outer-to-inner order.
		for (etherType == layers.EthernetTypeDot1Q || etherType == layers.EthernetTypeQinQ) && len(payload) >= 4 {
			etherType = layers.EthernetType(binary.BigEndian.Uint16(payload[2:4]))
			payload = payload[4:]
		}
		if etherType == layers.EthernetType(0x8892) {
			frame := p.Data()
			// The DCP wire codec takes the observed Ethernet frame, not its PPI
			// metadata prefix. Select only a validated, declared PPI layer; never
			// guess/strip a prefix from ordinary Ethernet or change packet identity.
			if decoded := p.Layers(); len(decoded) > 0 {
				if ppi, ok := decoded[0].(*capturePPILayer); ok {
					frame = ppi.payload
				}
			}
			a.decodeDCPEthernet(frame, e, ci)
			return nil, true
		}
		if etherType == layers.EthernetType(0x88cc) {
			a.decodeLLDPEthernet(eth, payload, e, ci)
			return nil, true
		}
		if etherType == layers.EthernetType(0x88b8) {
			// Consume near-matches too: an unknown-EtherType diagnostic must not
			// replace the strict GOOSE probe's rejection.
			a.decodeGOOSEEthernet(eth, payload, e, ci)
			return nil, true
		}
		if etherType == 0x88ba || etherType == 0x88a4 {
			a.decodeIndustrialEthernet(eth, etherType, payload, e, ci)
			return nil, true
		}
	}
	if a.observePIM(p, e) {
		return nil, true
	}
	// gopacket exposes the outer NetworkLayer and TransportLayer. Normalize a
	// supported tunnel to its innermost IP before transport dispatch.
	inner, networks, tunnels, unsupportedTunnel := networkCarrierProfile(p)
	if networks > 1 {
		// The inner carrier can become visible only after outer reassembly.
		// Every contributing fragment belongs to that same reconstructed domain.
		e.refs = append([]PacketReference(nil), e.refs...)
		if !e.carrierOwned {
			for i := range e.refs {
				e.refs[i].Domain = e.Ref.Domain
			}
		}
		if unsupportedTunnel || tunnels == 0 || tunnels+e.normalizedTunnels > 4 || networks != tunnels+1 {
			a.refuseUnselectedNetworkParent(p, e, ci)
			return nil, true
		}
		e.normalizedTunnels += tunnels
		e.preserveNetworkPrefix(p, inner)
		raw := append(bytes.Clone(inner.LayerContents()), inner.LayerPayload()...)
		// Fragmentation of an outer carrier does not mean that the inner
		// IPv6 ND message itself contained a Fragment header.
		e.ipv6Fragment = false
		ci.CaptureLength, ci.Length = len(raw), len(raw)
		ci = withEvidence(ci, e)
		p = gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw, datagramDecodeAs: a.datagramDecodeAs}, gopacket.Default)
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
			// RFC 8200 associates fragments by addresses and Identification, not
			// Next Header. Only the offset-zero fragment supplies that field.
			key = fragmentKey{e.Ref.Domain, ip.SrcIP.String(), ip.DstIP.String(), v.Identification, 0, true}
			off = int(v.FragmentOffset) * 8
			more = v.MoreFragments
			payload = v.Payload
		}
	}
	if off >= 0 {
		e.preserveNetworkPrefix(p, p.NetworkLayer())
		q, consumed := a.fragment(p, e, key, off, more, payload)
		// fragment() releases its store mutex before this dispatch. A complete
		// datagram must pass through tunnel normalization and native observation
		// before transport dispatch; the decoded outer IP is not the inner peer.
		if !consumed && q != nil {
			return a.networkPacket(q)
		}
		return q, consumed
	}
	if q, handled := a.observeL2TPv3(p, e); handled {
		if q != nil {
			return a.networkPacket(q)
		}
		return nil, true
	}

	// Check native observation admission before constructing or cloning field trees,
	// quoted packets, ND options or the captured Raw projection. Capture buffers
	// retained by the driver/caller are outside this parser's resource contract.
	nativeProtocol := ""
	if p.Layer(layers.LayerTypeARP) != nil {
		nativeProtocol = "arp"
	} else if p.Layer(layers.LayerTypeICMPv4) != nil {
		nativeProtocol = "icmp"
	} else if p.Layer(layers.LayerTypeICMPv6) != nil {
		nativeProtocol = "icmpv6"
	}
	if nativeProtocol != "" && len(p.Data()) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		err := protocolError(ErrResourceExceeded, "native network frame exceeds parser message limit")
		status, typed := classifySessionError(err)
		event := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: nativeProtocol, Domain: e.Ref.Domain, Length: len(p.Data()), Status: status, Completeness: status, ExpertCode: string(typed.Kind), Error: err.Error(), sessionError: typed, SourceBytes: networkObservationSource(e)}
		event.CarrierLayers = append([]string(nil), e.carrierLayers...)
		if n := p.NetworkLayer(); n != nil {
			event.Source, event.Destination = n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
		}
		if arp, ok := p.Layer(layers.LayerTypeARP).(*layers.ARP); ok {
			if arp.Protocol == layers.EthernetTypeIPv4 && arp.ProtAddressSize == 4 {
				event.Source, event.Destination = net.IP(arp.SourceProtAddress).String(), net.IP(arp.DstProtAddress).String()
			} else {
				event.Source, event.Destination = fmt.Sprintf("%x", arp.SourceProtAddress), fmt.Sprintf("%x", arp.DstProtAddress)
			}
		}
		a.limited.Add(uint64(len(p.Data())))
		a.emit(event)
		return nil, true
	}

	var protocol string
	var fields map[string]any
	if arp, ok := p.Layer(layers.LayerTypeARP).(*layers.ARP); ok {
		protocol = "arp"
		fields = map[string]any{"Operation": arp.Operation, "Sender MAC": net.HardwareAddr(arp.SourceHwAddress).String(), "Sender IP": net.IP(arp.SourceProtAddress).String(), "Target IP": net.IP(arp.DstProtAddress).String(), "Observation": "unverified-neighbor"}
	} else if icmp, ok := p.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6); ok {
		protocol = "icmpv6"
		// RFC 6980 section 5 forbids a Fragment header on ND types133..137,
		// including atomic fragments. Keep a diagnostic, never a valid ND
		// projection. SEND CPA/message types outside this range are distinct.
		if e.ipv6Fragment && icmp.TypeCode.Type() >= 133 && icmp.TypeCode.Type() <= 137 {
			a.malformed.Add(1)
			a.networkDiagnostic(e, ci, "malformed", "NDFragmentForbidden")
			return nil, true
		}
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
		if e.carrierOwned {
			// The logical native network PDU can consume the packet before the
			// transport callback. Preserve its actual full header path for the
			// UI instead of pairing an inner ICMP/ARP label with outer peers.
			names := make([]string, 0, len(e.networkPrefix)+8)
			for _, typ := range e.networkPrefix {
				names = append(names, typ.String())
			}
			for _, layer := range p.Layers() {
				if layer.LayerType() == gopacket.LayerTypePayload || layer.LayerType() == gopacket.LayerTypeDecodeFailure {
					break
				}
				if len(names) >= maxCaptureTransportLayers {
					break
				}
				names = append(names, layer.LayerType().String())
			}
			fields["Observed Header Layers"] = names
		}
		event := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: protocol, Domain: e.Ref.Domain, Length: len(p.Data()), Raw: bytes.Clone(p.Data()), Status: "decoded", Completeness: "message", Session: cloneSession(fields), SourceBytes: networkObservationSource(e), semanticFields: cloneSession(fields), semanticSession: cloneSession(fields)}
		event.CarrierLayers = append([]string(nil), e.carrierLayers...)
		if n := p.NetworkLayer(); n != nil {
			event.Source, event.Destination = n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
		}
		if protocol == "arp" {
			arp := p.Layer(layers.LayerTypeARP).(*layers.ARP)
			if arp.Protocol == layers.EthernetTypeIPv4 && arp.ProtAddressSize == 4 {
				event.Source, event.Destination = net.IP(arp.SourceProtAddress).String(), net.IP(arp.DstProtAddress).String()
			} else {
				event.Source, event.Destination = fmt.Sprintf("%x", arp.SourceProtAddress), fmt.Sprintf("%x", arp.DstProtAddress)
			}
		}
		event.Structured = map[string]any{"fields": cloneSession(fields)}
		a.messages.Add(1)
		a.decoded.Add(1)
		a.emit(event)
		return nil, true
	}
	return p, false
}

// decodeGOOSEEthernet admits IEC 61850 GOOSE only when the caller has matched
// the dedicated Ethernet protocol identifier and the existing bounded
// APPID/length/PDU probe accepts the payload. The PDU decoder then validates
// the complete message before it can be reported as decoded. A failed probe
// is consumed without a GOOSE claim so an EtherType by itself is insufficient.
func (a *binParser) decodeGOOSEEthernet(eth *layers.Ethernet, payload []byte, evidence captureEvidence, ci gopacket.CaptureInfo) {
	// Ethernet GOOSE is a complete frame; inspect the bounded gocbRef even when
	// it exceeds the TCP/UDP stream probe budget.
	if probeGOOSE(payload, len(payload)).Verdict != ProbeAccept {
		return
	}

	length := int(binary.BigEndian.Uint16(payload[2:4]))
	e := &ProtocolEvent{
		Timestamp:   ci.Timestamp,
		Transport:   "ethernet",
		Protocol:    "goose",
		Profile:     "iec61850-goose",
		Admission:   "ether-type-and-wire-signature",
		Source:      eth.SrcMAC.String(),
		Destination: eth.DstMAC.String(),
		Domain:      evidence.Ref.Domain,
		Length:      length,
		Status:      "deferred",
		Summary:     "GOOSE",
		Raw:         bytes.Clone(payload),
		SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{evidence.Ref}},
	}
	if length > a.config.MaxMessageBytes {
		e.Status, e.Completeness = "limited", "limited"
		e.ExpertCode, e.Error = "GOOSEMessageLimit", "GOOSE message exceeds parser limit"
		a.limited.Add(uint64(length))
		a.emit(e)
		return
	}
	if length > len(payload) {
		e.Status, e.Completeness = "incomplete", "incomplete"
		e.ExpertCode, e.Error = "GOOSELengthTruncated", "GOOSE declared length exceeds captured Ethernet payload"
		a.incomplete.Add(1)
		a.emit(e)
		return
	}

	// The IEC length excludes Ethernet padding. Feed only the declared GOOSE
	// frame to both the YAML parser and the session-field decoder.
	wire := payload[:length]
	e.Raw = bytes.Clone(wire)
	if spec := a.specs["iec61850/GOOSE"]; spec != nil {
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
	}
	result, err := e.Decode()
	if err == nil {
		e.Session, err = (&binGOOSE{}).consume(wire)
	}
	if err != nil {
		e.Status, e.Completeness = "malformed", "malformed"
		e.ExpertCode, e.Error = "GOOSEMalformed", err.Error()
		a.malformed.Add(1)
	} else {
		e.Status, e.Completeness = "decoded", "message"
		e.Structured = result
		e.retainRuleEnvelope(result)
		e.Summary = fmt.Sprintf("GOOSE state %v sequence %v", e.Session["State Number"], e.Session["Sequence Number"])
		a.messages.Add(1)
		a.messageBytes.Add(uint64(length))
		a.decoded.Add(1)
	}
	a.emit(e)
}

func networkObservationSource(e captureEvidence) ByteSource {
	if e.byteSourceKind != "" {
		b := ByteSource{Kind: e.byteSourceKind, PacketRefs: append([]PacketReference(nil), e.refs...), ParentPDUs: append([]uint64(nil), e.parents...)}
		if len(e.parents) > 0 {
			b.ParentPDU = e.parents[len(e.parents)-1]
		}
		return b
	}
	if len(e.refs) != 0 {
		return ByteSource{Kind: "reassembled", PacketRefs: append([]PacketReference(nil), e.refs...)}
	}
	return ByteSource{Kind: "captured", PacketRefs: []PacketReference{e.Ref}}
}

func (a *binParser) networkDiagnostic(e captureEvidence, ci gopacket.CaptureInfo, status, code string) {
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Domain: e.Ref.Domain, Status: status, Completeness: status, ExpertCode: code, Error: code, SourceBytes: networkObservationSource(e)})
}
func (a *binParser) fragment(p gopacket.Packet, e captureEvidence, key fragmentKey, off int, more bool, payload []byte) (gopacket.Packet, bool) {
	ci := p.Metadata().CaptureInfo
	if ci.CaptureLength < ci.Length {
		// A proved local capture cut is not evidence of malformed bytes on
		// the network. Keep existing complete ranges and allow a later full
		// retransmission to repair it; never retain the ambiguous prefix.
		a.networkDiagnostic(e, ci, "incomplete", "FragmentCaptureTruncated")
		return nil, true
	}
	// RFC 6946: atomic fragments neither join nor remove a matching pending
	// datagram, including one quarantined after an overlap.
	if key.v6 && off == 0 && !more {
		header, err := fragmentHeader(p, true)
		if err != nil || p.Metadata().Truncated || ci.CaptureLength < ci.Length || len(header)-40+len(payload) > 65535 {
			a.networkDiagnostic(e, ci, "malformed", "FragmentLength")
			return nil, true
		}
		return a.reassembledPacket(header, payload, e, ci, true)
	}
	s := &a.fragments
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sets == nil {
		s.sets = map[fragmentKey]*fragmentSet{}
	}
	var expired []fragmentKey
	for k, v := range s.sets {
		// Interfaces, sections and tunnels can have independent capture clocks.
		// Only this domain's actual timestamp can prove a known lease expired.
		// Unknown time remains unknown within the existing count/byte bounds.
		if k.domain == key.domain && !ci.Timestamp.IsZero() && !v.last.IsZero() && ci.Timestamp.Sub(v.last) > 30*time.Second {
			expired = append(expired, k)
		}
	}
	sortFragmentKeys(expired)
	for _, k := range expired {
		v := s.sets[k]
		a.buffered.Add(-v.cost)
		delete(s.sets, k)
		a.fragmentDiagnostic(v, "FragmentTimeout")
	}
	if s.blocked {
		if key.domain != s.blockedDomain || s.blockedUntil.IsZero() || ci.Timestamp.IsZero() || ci.Timestamp.Before(s.blockedUntil) {
			a.networkDiagnostic(e, ci, "limited", "FragmentQuarantineBudget")
			return nil, true
		}
		s.blocked, s.blockedDomain, s.blockedUntil = false, CaptureDomain{}, time.Time{}
	}
	if v := s.sets[key]; v != nil && v.rejected {
		return nil, true
	}
	invalid := func(code string) (gopacket.Packet, bool) {
		if v := s.sets[key]; v != nil && v.cost >= 128 {
			// Discard all bytes, but retain a budgeted tombstone until the
			// original deadline. Later fragments must not resurrect the datagram.
			a.buffered.Add(-(v.cost - 128))
			v.parts, v.header, v.cost, v.rejected = nil, nil, 128, true
		} else {
			delete(s.sets, key) // An unreserved, empty provisional set owns no bytes.
			if len(s.sets) < a.budget.MaxCollectionElements && a.reserveEvidence(128) {
				s.sets[key] = &fragmentSet{last: ci.Timestamp, cost: 128, rejected: true}
			} else {
				// Without space to remember a rejected ID, fail closed for fragment
				// admission for one bounded lifetime. Unfragmented traffic and IPv6
				// atomic fragments remain independent.
				s.blocked, s.blockedDomain = true, key.domain
				s.blockedUntil = time.Time{}
				if !ci.Timestamp.IsZero() {
					s.blockedUntil = ci.Timestamp.Add(30 * time.Second)
				}
			}
		}
		a.networkDiagnostic(e, ci, "malformed", code)
		return nil, true
	}
	if p.Metadata().Truncated || ci.CaptureLength < ci.Length || off+len(payload) > 65535 || len(payload) == 0 || more && len(payload)%8 != 0 {
		return invalid("FragmentLength")
	}
	if key.v6 && off == 0 {
		f, ok := p.Layer(layers.LayerTypeIPv6Fragment).(*layers.IPv6Fragment)
		if !ok || !completeFragmentFirstHeader(byte(f.NextHeader), payload) {
			return invalid("FragmentFirstHeader")
		}
	}
	v := s.sets[key]
	if v == nil {
		if len(s.sets) >= a.budget.MaxCollectionElements {
			a.networkDiagnostic(e, ci, "limited", "FragmentSessionBudget")
			return nil, true
		}
		v = &fragmentSet{end: -1, last: ci.Timestamp}
		s.sets[key] = v
	}
	for _, part := range v.parts {
		if off == part.start && bytes.Equal(part.data, payload) {
			if part.more != more {
				return invalid("FragmentConflictingEnd")
			}
			if key.v6 && off == 0 {
				header, err := fragmentHeader(p, true)
				if err != nil || !bytes.Equal(v.header, header) {
					return invalid("FragmentOverlapRejected")
				}
			}
			// RFC8200 4.5 explicitly allows dropping exact duplicates while
			// preserving other fragments. Neither evidence cost nor contributor
			// references grow, and conflicting/partial overlaps still fail closed.
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
	refs := e.refs
	if len(refs) == 0 {
		refs = []PacketReference{e.Ref}
	}
	contributors := len(refs)
	for _, part := range v.parts {
		contributors += len(part.refs)
	}
	if contributors > min(128, a.budget.MaxCollectionElements) {
		return invalid("FragmentRangeBudget")
	}
	// Reconstructed fragments can themselves contain reconstructed carriers.
	// Retain and charge every physical contributor, not only the final arrival.
	cost := int64(len(payload) + 128*len(refs) + 8*len(e.parents))
	var header []byte
	if off == 0 {
		var err error
		header, err = fragmentHeader(p, key.v6)
		if err != nil {
			return invalid("FragmentHeader")
		}
		cost += int64(len(header))
	}
	if !a.reserveEvidence(cost) {
		if len(v.parts) == 0 {
			delete(s.sets, key)
		}
		a.networkDiagnostic(e, ci, "limited", "FragmentByteBudget")
		return nil, true
	}
	v.cost += cost
	if header != nil {
		v.header = header
	}
	v.parts = append(v.parts, ipFragment{start: off, data: bytes.Clone(payload), refs: append([]PacketReference(nil), refs...), more: more, parents: append([]uint64(nil), e.parents...)})
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
	refs = make([]PacketReference, 0, contributors)
	var parents []uint64
	for _, part := range v.parts {
		raw = append(raw, part.data...)
		refs = append(refs, part.refs...)
		for _, id := range part.parents {
			if len(parents) == 0 || parents[len(parents)-1] != id {
				parents = append(parents, id)
			}
		}
	}
	length := len(v.header) + len(raw)
	if key.v6 {
		length -= 40
	}
	if len(v.header) == 0 || length > 65535 {
		return invalid("FragmentLength")
	}
	header = v.header
	a.buffered.Add(-v.cost)
	delete(s.sets, key)
	e.refs = refs
	e.parents = parents
	if e.carrierOwned {
		e.byteSourceKind = "reassembled"
	}
	return a.reassembledPacket(header, raw, e, ci, key.v6)
}

// RFC 8200 section 4.5 requires the offset-zero fragment to contain the
// extension chain and upper-layer header. Splitting TCP ports from the rest of
// its header must not establish a flow after reassembly. The walk consumes at
// least eight bytes per extension and never allocates retained state.
func completeFragmentFirstHeader(next byte, b []byte) bool {
	for {
		switch next {
		case 0, 43, 60, 51:
			if len(b) < 2 {
				return false
			}
			n := (int(b[1]) + 1) * 8
			if next == 51 {
				n = (int(b[1]) + 2) * 4
			}
			if n > len(b) {
				return false
			}
			next, b = b[0], b[n:]
		case 6:
			if len(b) < 20 {
				return false
			}
			n := int(b[12]>>4) * 4
			return n >= 20 && n <= len(b)
		case 17, 58, 50:
			return len(b) >= 8
		case 4:
			if len(b) < 20 {
				return false
			}
			n := int(b[0]&15) * 4
			return n >= 20 && n <= len(b)
		case 41:
			return len(b) >= 40
		default:
			// Unknown upper layers remain unknown to normal ingress; no
			// transport admission is inferred from their identifier.
			return true
		}
	}
}

// Retain the offset-zero IP header and pre-fragment extension headers. Rebuilding
// a minimal IP header would silently erase options, traffic class and hop limit.
func fragmentHeader(p gopacket.Packet, v6 bool) ([]byte, error) {
	if !v6 {
		ip, ok := p.NetworkLayer().(*layers.IPv4)
		if !ok || len(ip.LayerContents()) < 20 {
			return nil, fmt.Errorf("IPv4 header")
		}
		return bytes.Clone(ip.LayerContents()), nil
	}
	var header []byte
	next := 6
	for _, l := range p.Layers() {
		if ip, ok := l.(*layers.IPv6); ok {
			header = bytes.Clone(ip.LayerContents())
			continue
		}
		if len(header) == 0 {
			continue
		}
		if f, ok := l.(*layers.IPv6Fragment); ok {
			if len(header) < 40 || next >= len(header) {
				break
			}
			header[next] = byte(f.NextHeader)
			return header, nil
		}
		switch l.LayerType() {
		case layers.LayerTypeIPv6HopByHop, layers.LayerTypeIPv6Routing, layers.LayerTypeIPv6Destination, layers.LayerTypeIPSecAH:
			next = len(header)
			header = append(header, l.LayerContents()...)
		default:
			return nil, fmt.Errorf("unsupported pre-fragment header")
		}
	}
	return nil, fmt.Errorf("IPv6 fragment header")
}

func (a *binParser) reassembledPacket(header, payload []byte, e captureEvidence, ci gopacket.CaptureInfo, v6 bool) (gopacket.Packet, bool) {
	raw := append(bytes.Clone(header), payload...)
	if v6 {
		binary.BigEndian.PutUint16(raw[4:6], uint16(len(raw)-40))
	} else {
		binary.BigEndian.PutUint16(raw[2:4], uint16(len(raw)))
		binary.BigEndian.PutUint16(raw[6:8], binary.BigEndian.Uint16(raw[6:8])&0x4000)
		raw[10], raw[11] = 0, 0
		var sum uint32
		for i := 0; i < len(header); i += 2 {
			sum += uint32(binary.BigEndian.Uint16(raw[i : i+2]))
		}
		for sum>>16 != 0 {
			sum = (sum & 65535) + (sum >> 16)
		}
		binary.BigEndian.PutUint16(raw[10:12], ^uint16(sum))
	}
	q := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeRaw, datagramDecodeAs: a.datagramDecodeAs}, gopacket.Default)
	e.ipv6Fragment = v6
	if len(e.refs) == 0 {
		e.refs = []PacketReference{e.Ref}
	}
	ci.CaptureLength, ci.Length = len(raw), len(raw)
	ci = withEvidence(ci, e)
	q.Metadata().CaptureInfo = ci
	return q, false
}
func (a *binParser) closeFragments() {
	s := &a.fragments
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]fragmentKey, 0, len(s.sets))
	for k := range s.sets {
		keys = append(keys, k)
	}
	sortFragmentKeys(keys)
	for _, k := range keys {
		v := s.sets[k]
		a.buffered.Add(-v.cost)
		a.fragmentDiagnostic(v, "FragmentCaptureEnded")
	}
	s.sets = nil
	s.blocked, s.blockedDomain, s.blockedUntil = false, CaptureDomain{}, time.Time{}
}

// Diagnostics allocate PDU IDs and compete for bounded history just like
// decoded messages. A randomized map traversal must not decide which physical
// frames still have details after EOF or a batch of simultaneous expirations.
func sortFragmentKeys(keys []fragmentKey) {
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.domain.Section != b.domain.Section {
			return a.domain.Section < b.domain.Section
		}
		if a.domain.Interface != b.domain.Interface {
			return a.domain.Interface < b.domain.Interface
		}
		if a.domain.Encapsulation != b.domain.Encapsulation {
			return a.domain.Encapsulation < b.domain.Encapsulation
		}
		if a.src != b.src {
			return a.src < b.src
		}
		if a.dst != b.dst {
			return a.dst < b.dst
		}
		if a.id != b.id {
			return a.id < b.id
		}
		if a.protocol != b.protocol {
			return a.protocol < b.protocol
		}
		return !a.v6 && b.v6
	})
}
func (a *binParser) fragmentDiagnostic(v *fragmentSet, code string) {
	if len(v.parts) == 0 {
		return
	}
	refs := make([]PacketReference, 0, len(v.parts))
	for _, p := range v.parts {
		refs = append(refs, p.refs...)
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
	var clientSeen, serverSeen bool
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
		if code == 1 {
			if clientSeen {
				return nil, fmt.Errorf("DHCPv6 duplicate client identifier")
			}
			clientSeen = true
		}
		if code == 2 {
			if serverSeen {
				return nil, fmt.Errorf("DHCPv6 duplicate server identifier")
			}
			serverSeen = true
		}
		if code == 8 && n != 2 {
			return nil, fmt.Errorf("DHCPv6 elapsed time width")
		}
		if code == 14 && n != 0 {
			return nil, fmt.Errorf("DHCPv6 Rapid Commit width")
		}
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
