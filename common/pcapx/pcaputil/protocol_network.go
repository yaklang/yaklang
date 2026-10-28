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
	return networkCarrierProfileUntil(p, nil)
}

func networkCarrierProfileUntil(p gopacket.Packet, stop gopacket.Layer) (inner gopacket.NetworkLayer, networks, tunnels int, unsupported bool) {
	for _, l := range p.Layers() {
		if stop != nil && l == stop {
			break
		}
		if n, ok := l.(gopacket.NetworkLayer); ok {
			inner = n
			networks++
		}
		switch v := l.(type) {
		case *capturePIM:
			if v.Type == 1 && !v.NullRegister {
				tunnels++
			}
		case *layers.GRE:
			// ACK-only PPTP has no child network to normalize. Counting it
			// as another network tunnel would reject a valid terminal header.
			if !selectedPPTPGRE(v) || len(v.LayerPayload()) != 0 {
				tunnels++
			}
			// Version zero and the legacy PPTP version-one decoder have
			// distinct existing layouts. Other version bits cannot select an
			// inner grammar merely because Protocol Type resembles an EtherType.
			if v.Version > 1 || v.Version == 1 && !selectedPPTPGRE(v) {
				unsupported = true
			}
		case *layers.VXLAN:
			tunnels++
			if !selectedVXLAN(v) {
				unsupported = true
			}
		case *captureVXLANGPE:
			tunnels++
			unsupported = true // only its complete header is observed
		case *layers.Geneve:
			tunnels++
			// Only option-free v0 Ethernet data is selected for normalization.
			header := v.LayerContents()
			if len(header) != 8 || header[0] != 0 || header[1] != 0 || header[7] != 0 || v.Protocol != layers.EthernetTypeTransparentEthernetBridging {
				unsupported = true
			}
		case *layers.IPv4:
			if v.Version == 4 && v.Protocol == layers.IPProtocolIPv6HopByHop && v.FragOffset == 0 && v.Flags&layers.IPv4MoreFragments == 0 {
				// IPv4 HOPOPT is an unselected carrier profile. Its typed
				// standalone-HBH refusal must not elevate any child identity or
				// advance fragment/session clocks before this existing gate.
				// Failed IPv4 decodes leave Protocol at its zero default; only a
				// successfully validated IPv4 may select this parent policy.
				unsupported = true
			}
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

func selectedVXLANEnclosingParent(p gopacket.Packet, layer gopacket.Layer, normalized int) bool {
	found := false
	for _, candidate := range p.Layers() {
		if candidate == layer {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	_, networks, tunnels, unsupported := networkCarrierProfileUntil(p, layer)
	return !unsupported && tunnels+normalized+1 <= 4 && (networks <= 1 || tunnels > 0 && networks == tunnels+1)
}

func (a *binParser) refuseVXLANHeader(p gopacket.Packet, e captureEvidence, ci gopacket.CaptureInfo, failure *vxlanHeaderError) {
	fields := map[string]any{"Observed Header Layers": outerNetworkHeaderNames(p), "Header Observation Scope": "outer network prefix only", "Child Header Identity": "unconfirmed", "Encapsulation Admission": "unselected profile", "Unselected Carrier": "VXLAN", "Parent Admission Reason": failure.reason, "Available VXLAN Bytes": failure.available, "Required VXLAN Header Bytes": 8, "Reserved Receive Rule": "ignored per RFC 7348"}
	h := failure.layer.LayerContents()
	if len(h) > 0 {
		fields["Observed VXLAN Flags"], fields["VXLAN I Bit"] = h[0], failure.layer.ValidIDFlag
	}
	if len(h) >= 7 {
		fields["Observed VXLAN VNI"] = failure.layer.VNI
	}
	if failure.truncated {
		if failure.innerTruncated {
			fields["Header Observation Scope"] = "validated VXLAN fixed header; incomplete Ethernet child prefix"
			fields["Available Ethernet Header Bytes"] = len(failure.layer.LayerPayload())
			fields["Required Ethernet Header Bytes"] = 14
		} else {
			fields["VXLAN Header Truncated"] = true
		}
	}
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: e.Ref.Domain, Status: "limited", Completeness: "limited", ExpertCode: "VXLANHeaderAdmission", Error: failure.Error(), Summary: failure.Error() + "; inner identity unconfirmed", Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
}

func (a *binParser) observeVXLANGPEHeader(p gopacket.Packet, e captureEvidence, ci gopacket.CaptureInfo, layer *captureVXLANGPE) {
	h := layer.LayerContents()
	vni := uint32(h[4])<<16 | uint32(h[5])<<8 | uint32(h[6])
	fields := map[string]any{"Observed Header Layers": outerNetworkHeaderNames(p), "Header Observation Scope": "validated VXLAN-GPE v0 header only", "Child Header Identity": "unconfirmed", "Encapsulation Admission": "unselected payload profile", "Unselected Carrier": "VXLAN-GPE", "VXLAN-GPE Version": uint8(0), "VXLAN-GPE Flags": h[0], "VXLAN-GPE VNI": vni, "VXLAN-GPE Next Protocol Present": h[0]&4 != 0, "VXLAN-GPE Next Protocol": h[3], "VXLAN-GPE OAM": h[0]&1 != 0, "VXLAN-GPE Replicated BUM": h[0]&2 != 0, "Reserved Receive Rule": "ignored", "Payload Profile Implemented": false}
	summary := fmt.Sprintf("VXLAN-GPE v0 header VNI=%d next=%d; payload profile not selected; inner identity unconfirmed", vni, h[3])
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: e.Ref.Domain, Status: "limited", Completeness: "headers", ExpertCode: "VXLANProfileUnselected", Summary: summary, Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
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
	summary := "Encapsulation profile unavailable; inner identity unconfirmed"
	// Report a literal GRE version only directly after the observed outer IP.
	// A child behind some other unselected parent has no admitted GRE identity.
	network := p.NetworkLayer()
	directGRE := false
	switch ip := network.(type) {
	case *layers.IPv4:
		directGRE = ip.Protocol == layers.IPProtocolGRE
	case *layers.IPv6:
		directGRE = ip.NextHeader == layers.IPProtocolGRE
	}
	if directGRE && e.normalizedTunnels < 4 {
		afterNetwork := false
		for _, layer := range p.Layers() {
			if layer == network {
				afterNetwork = true
				continue
			}
			if !afterNetwork {
				continue
			}
			if gre, ok := layer.(*layers.GRE); ok {
				if gre.Version > 1 {
					fields["Unselected Carrier"] = "GRE"
					fields["Observed GRE Version"] = gre.Version
					summary = fmt.Sprintf("GRE version %d not selected; inner identity unconfirmed", gre.Version)
				} else if gre.Version == 1 && !selectedPPTPGRE(gre) {
					fields["Unselected Carrier"] = "PPTP"
					fields["Observed GRE Version"] = uint8(1)
					fields["Parent Admission Reason"] = "unselected enhanced GRE header"
					summary = "PPTP header profile unavailable; inner identity unconfirmed"
				}
				break
			}
			if _, inner := layer.(gopacket.NetworkLayer); inner {
				break
			}
		}
	}
	// Narrow the displayed interpretation, never the observed source identity.
	// The zero flow IDs and explicit refusal admit no child connection state.
	a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: e.Ref.Domain, Status: "limited", Completeness: "limited", ExpertCode: "EncapsulationDepthOrProfile", Error: "EncapsulationDepthOrProfile", Summary: summary, Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
}

func (a *binParser) networkPacket(p gopacket.Packet) (gopacket.Packet, bool) {
	e := packetEvidence(p)
	ci := p.Metadata().CaptureInfo
	inner, networks, tunnels, unsupportedTunnel := networkCarrierProfile(p)
	if networks > 1 {
		// Preserve the existing full observation domain before narrowing a
		// rejected interpretation, including sources reconstructed from fragments.
		e.refs = append([]PacketReference(nil), e.refs...)
		if !e.carrierOwned {
			for i := range e.refs {
				e.refs[i].Domain = e.Ref.Domain
			}
		}
	}
	if failure := p.ErrorLayer(); failure != nil {
		var vxlan *vxlanHeaderError
		if errors.As(failure.Error(), &vxlan) && selectedVXLANEnclosingParent(p, vxlan.layer, e.normalizedTunnels) {
			a.refuseVXLANHeader(p, e, ci, vxlan)
			// Retain the typed decoder error for strict API/accounting. The UDP
			// ingress guard prevents redispatch as an outer application.
			return p, false
		}
	}
	for _, layer := range p.Layers() {
		if gpe, ok := layer.(*captureVXLANGPE); ok && selectedVXLANEnclosingParent(p, gpe, e.normalizedTunnels) {
			a.observeVXLANGPEHeader(p, e, ci, gpe)
			return nil, true
		}
	}
	if unsupportedTunnel || networks > 1 && (tunnels == 0 || tunnels+e.normalizedTunnels > 4 || networks != tunnels+1) {
		// Parent admission precedes every child observation, including ARP,
		// topology/discovery codecs and errors in incomplete inner IP headers.
		a.refuseUnselectedNetworkParent(p, e, ci)
		return nil, true
	}
	if failure := p.ErrorLayer(); failure != nil {
		var pptp *pptpHeaderError
		if errors.As(failure.Error(), &pptp) {
			// The enclosing topology/profile gate above takes precedence: an
			// error in a child behind unselected Geneve/GRE is not a PPTP claim.
			fields := map[string]any{"Observed Header Layers": outerNetworkHeaderNames(p), "Header Observation Scope": "outer network prefix only", "Child Header Identity": "unconfirmed", "Encapsulation Admission": "unselected profile", "Unselected Carrier": "PPTP", "Observed GRE Version": uint8(1), "Parent Admission Reason": pptp.reason, "GRE Header Flags": pptp.flags, "Available GRE Bytes": pptp.available, "Required GRE Bytes": pptp.required}
			if pptp.hasProtocol {
				fields["GRE Protocol Type"] = pptp.protocol
			}
			if pptp.truncated {
				fields["Declared GRE Bytes Unavailable"] = true
			}
			a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: e.Ref.Domain, Status: "limited", Completeness: "limited", ExpertCode: "PPTPHeaderAdmission", Error: pptp.Error(), Summary: pptp.Error() + "; inner identity unconfirmed", Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
			// The error-bearing packet preserves capture decoder-error accounting
			// and the strict API result; its child decoder has never run.
			return p, false
		}
		var truncated *ipHeaderTruncatedError
		var options *ipv6OptionsHeaderError
		var ah *ahHeaderError
		if errors.As(failure.Error(), &options) || errors.As(failure.Error(), &ah) {
			code := "IPv6OptionsHeaderBounds"
			if options != nil && options.placement {
				code = "IPv6OptionsHeaderOrder"
			}
			if ah != nil {
				code = "AHHeaderBounds"
			}
			a.malformed.Add(1)
			a.networkHeaderRefusal(p, e, ci, code, failure.Error().Error())
			return p, false // Keep strict capture API and decoder-error accounting.
		}
		if errors.As(failure.Error(), &truncated) {
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
	for _, layer := range p.Layers() {
		gre, ok := layer.(*layers.GRE)
		if !ok || !selectedPPTPGRE(gre) || len(gre.LayerPayload()) != 0 {
			continue
		}
		fields := map[string]any{"Observed GRE Version": uint8(1), "PPTP Call ID": uint16(gre.Key), "PPTP Payload Length": uint16(0), "PPTP Acknowledgment Number": gre.Ack, "Header Observation Scope": "PPTP acknowledgment header only", "Child Header Identity": "not present", "PPTP Data Path Scope": "one direction; call-control association unavailable"}
		a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "pptp-acknowledgment", Domain: e.Ref.Domain, Status: "limited", Completeness: "headers", ExpertCode: "PPTPAcknowledgment", Summary: fmt.Sprintf("PPTP acknowledgment call=%d ack=%d; no PPP payload", uint16(gre.Key), gre.Ack), Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(e)})
		return nil, true
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
	if networks > 1 {
		e.normalizedTunnels += tunnels
		e.preserveNetworkPrefix(p, inner)
		raw := bytes.Clone(inner.LayerContents())
		if ip, ok := inner.(*layers.IPv6); ok && ip.HopByHop != nil {
			// IPv6's Payload begins after its embedded first HBH. Keep that
			// validated header when reconstructing the complete inner datagram.
			raw = append(raw, ip.HopByHop.LayerContents()...)
		}
		raw = append(raw, inner.LayerPayload()...)
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
	if a.observeESP(p, e) {
		if failure := p.ErrorLayer(); failure != nil {
			var short *espHeaderTruncatedError
			if errors.As(failure.Error(), &short) {
				return p, false
			}
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
		case 137:
			// RFC 4861 section 4.5: reserved, target and destination follow
			// the generic four-byte ICMPv6 header, before any ND options.
			fixed = 36
			if len(b) >= 20 {
				fields["Target"] = net.IP(b[4:20]).String()
			}
			if len(b) >= 36 {
				fields["Destination"] = net.IP(b[20:36]).String()
			}
		case 1, 2, 3, 4:
			fixed = 4 // RFC 4443: type-specific word precedes the quoted packet.
			if len(b) >= 4 {
				fields["Quoted Packet"] = bytes.Clone(b[4:])
				if typ == 2 {
					fields["MTU"] = binary.BigEndian.Uint32(b)
				}
			}
		case 128, 129:
			fixed = 4 // Identifier and sequence are mandatory, even with no data.
			if len(b) >= 4 {
				fields["Identifier"], fields["Sequence"] = binary.BigEndian.Uint16(b), binary.BigEndian.Uint16(b[2:])
			}
		}
		if fixed > len(b) {
			fields["Available ICMPv6 Bytes"] = 4 + len(b)
			fields["Required ICMPv6 Header Bytes"] = 4 + fixed
			a.emitNativeNetworkObservation(p, e, ci, protocol, fields, "incomplete", "ICMPv6HeaderTruncated")
			return nil, true
		}
		if typ >= 133 && typ <= 137 {
			var opts []map[string]any
			for at := fixed; at < len(b); {
				if len(b)-at < 2 || b[at+1] == 0 || int(b[at+1])*8 > len(b)-at {
					fields["Options"] = opts
					a.emitNativeNetworkObservation(p, e, ci, protocol, fields, "malformed", "NDOptionLength")
					return nil, true
				}
				n := int(b[at+1]) * 8
				if len(opts) >= a.budget.MaxCollectionElements {
					fields["Options"] = opts
					a.emitNativeNetworkObservation(p, e, ci, protocol, fields, "limited", "NDOptionBudget")
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
		a.emitNativeNetworkObservation(p, e, ci, protocol, fields, "decoded", "")
		return nil, true
	}
	return p, false
}

// A complete generic header proves the observed family, not a complete body.
// Preserve only available fields and their source on a refusal; quoted inner
// decoder errors do not invalidate a complete outer ICMP message.
func (a *binParser) emitNativeNetworkObservation(p gopacket.Packet, e captureEvidence, ci gopacket.CaptureInfo, protocol string, fields map[string]any, status, code string) {
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
	completeness := status
	if status == "decoded" {
		completeness = "message"
	}
	event := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: protocol, Domain: e.Ref.Domain, Length: len(p.Data()), Raw: bytes.Clone(p.Data()), Status: status, Completeness: completeness, ExpertCode: code, Error: code, Session: cloneSession(fields), SourceBytes: networkObservationSource(e), semanticFields: cloneSession(fields), semanticSession: cloneSession(fields)}
	if code != "" {
		event.Profile = "icmpv6-header-observation"
		name := layers.CreateICMPv6TypeCode(fields["Type"].(uint8), fields["Code"].(uint8)).String()
		event.Summary = "ICMPv6 " + name + "; " + code
		if code == "ICMPv6HeaderTruncated" {
			event.Summary = fmt.Sprintf("ICMPv6 %s; incomplete fixed header (%v/%v bytes)", name, fields["Available ICMPv6 Bytes"], fields["Required ICMPv6 Header Bytes"])
		} else if code == "NDOptionLength" {
			event.Summary = "ICMPv6 " + name + "; invalid ND option length"
		}
	}
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
	switch status {
	case "decoded":
		a.messages.Add(1)
		a.decoded.Add(1)
	case "incomplete":
		a.incomplete.Add(1)
	case "malformed":
		a.malformed.Add(1)
	case "limited":
		a.limited.Add(uint64(len(p.Data())))
	}
	a.emit(event)
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

func (a *binParser) networkHeaderRefusal(p gopacket.Packet, evidence captureEvidence, ci gopacket.CaptureInfo, code, reason string) {
	names := make([]string, 0, len(evidence.networkPrefix)+8)
	for _, kind := range evidence.networkPrefix {
		names = append(names, kind.String())
	}
	for _, layer := range p.Layers() {
		kind := layer.LayerType()
		if kind == gopacket.LayerTypeDecodeFailure || kind == gopacket.LayerTypePayload || kind == layers.LayerTypeICMPv4 || kind == layers.LayerTypeICMPv6 || kind == layers.LayerTypeIPSecESP {
			break
		}
		if len(names) >= maxCaptureTransportLayers {
			names = nil
			break
		}
		names = append(names, kind.String())
	}
	fields := map[string]any{"Observed Header Layers": names, "Header Observation Scope": "validated network prefix before refused child header", "Child Header Identity": "unconfirmed", "Parent Header Refusal": code}
	e := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-header-refusal", Domain: evidence.Ref.Domain, Status: "malformed", Completeness: "malformed", ExpertCode: code, Error: reason, Summary: reason + "; child identity not admitted", Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...)}
	for _, layer := range p.Layers() {
		if layer.LayerType() == gopacket.LayerTypeDecodeFailure || layer.LayerType() == layers.LayerTypeICMPv4 || layer.LayerType() == layers.LayerTypeICMPv6 {
			break
		}
		h := layer.LayerContents()
		switch layer.(type) {
		case *layers.IPv4:
			if len(h) >= 20 && h[0]>>4 == 4 && int(h[0]&15)*4 == len(h) && int(binary.BigEndian.Uint16(h[2:4])) >= len(h) {
				e.Source, e.Destination = net.IP(h[12:16]).String(), net.IP(h[16:20]).String()
			}
		case *layers.IPv6:
			if len(h) == 40 && h[0]>>4 == 6 {
				e.Source, e.Destination = net.IP(h[8:24]).String(), net.IP(h[24:40]).String()
			}
		}
	}
	a.emit(e)
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
