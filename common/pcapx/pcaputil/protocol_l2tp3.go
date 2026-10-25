package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

var captureL2TPv3Type = gopacket.RegisterLayerType(200005, gopacket.LayerTypeMetadata{Name: "L2TPv3"})

func (a *binParser) l2tp3UDPNeedsPacket(network gopacket.NetworkLayer, udp *layers.UDP, domain CaptureDomain) bool {
	if len(udp.Payload) < 2 || udp.Payload[1]&15 != 3 {
		return false
	}
	src := l2tp3Endpoint{network.NetworkFlow().Src().String(), uint16(udp.SrcPort)}
	dst := l2tp3Endpoint{network.NetworkFlow().Dst().String(), uint16(udp.DstPort)}
	a.l2tp3Mu.Lock()
	defer a.l2tp3Mu.Unlock()
	return a.l2tp3UDPAdmission(domain, src, dst, udp.Payload) != ""
}

// Inner bytes own a separate copy. An event callback cannot alter subsequent
// normalized transport by editing the public wire/field projections.
func (a *binParser) decodeL2TPv3Event(e *ProtocolEvent, wire []byte, udp, truncated bool, carrierErr error) (inner []byte, suffix string, parent uint64) {
	e.Protocol, e.Rule, e.Profile, e.nativeSessionMessage = "l2tp", "l2tp3", "l2tp3-bounded-wire-envelope", true
	e.Entry = "L2TPv3IPFields"
	if udp {
		e.Entry = "L2TPv3UDPFields"
	}
	e.Completeness = "message"
	e.Length = len(wire)
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	parent = e.ID
	var m map[string]any
	var err error
	if truncated {
		err = protocolError(ErrNeedMore, "L2TPv3 carrier capture is truncated")
	} else if carrierErr != nil {
		err = carrierErr
	} else if len(wire) > min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes) {
		err = protocolError(ErrResourceExceeded, "L2TPv3 wire exceeds parser limit")
	} else {
		m, err = stream_parser.DecodeL2TPv3Wire(wire, udp, a.budget.MaxCollectionElements)
		err = observedWireError(err)
	}
	if m != nil && err == nil {
		a.l2tp3Mu.Lock()
		if m["Control"] == true {
			a.l2tp3ControlObserve(e, m, wire, udp)
		} else {
			m, inner, suffix, err = a.l2tp3DataResolve(e, m, wire, udp)
		}
		a.l2tp3Mu.Unlock()
		if m["Unparsed Parameters"] == true || m["Unparsed Message Type"] == true || m["Ambiguous Parameters"] == true || m["Association Diagnostic"] != nil {
			e.Completeness = "limited"
		}
		if m["Control"] != true && m["Payload Decoded"] != true {
			m["Opaque Payload Bytes"] = uint64(len(wire) - int(routingNumber(m, "Payload Offset")))
			if m["Connection Context"] == nil {
				m["Connection Context"] = "not observed; cookie, sublayer and pseudowire are unconfirmed"
			}
			e.Completeness = "limited"
		}
		if err != nil {
			m["Association Diagnostic"] = err.Error()
		}
	}
	if m != nil {
		e.Session = m
		e.semanticFields, e.semanticSession = cloneSession(m), cloneSession(m)
	}
	if len(wire) > min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes) {
		a.finishProtocolDatagram(e, nil, nil, err)
		e.Length = len(wire)
		a.messageBytes.Add(uint64(len(wire)))
		a.limited.Add(uint64(len(wire)))
	} else {
		a.finishProtocolDatagram(e, wire, nil, err)
	}
	if err != nil {
		e.Completeness = "limited"
	} else {
		e.Summary = fmt.Sprintf("L2TPv3 %s", m["Packet Name"])
		if m["Control"] != true {
			if m["Payload Decoded"] == true {
				e.Summary += fmt.Sprintf(" · session=0x%08x · observed receiver pseudowire=%d", m["Session ID"], m["Pseudowire Type"])
			} else {
				e.Summary += fmt.Sprintf(" · session=0x%08x · receiver context unavailable; payload opaque", m["Session ID"])
			}
		}
	}
	a.emit(e)
	return
}

// IP115 is explicit carrier evidence. UDP data requires an observed receiver
// association or Decode-As; a service/header hint never admits its payload.
func (a *binParser) observeL2TPv3(p gopacket.Packet, evidence captureEvidence) (gopacket.Packet, bool) {
	network := p.NetworkLayer()
	if network == nil {
		return nil, false
	}
	udp, selected := false, false
	var carrierErr error
	switch ip := network.(type) {
	case *layers.IPv4:
		selected = ip.Protocol == 115
		if len(ip.Contents) < 20 || ip.Version != 4 || ip.IHL < 5 || int(binary.BigEndian.Uint16(ip.Contents[2:4])) < int(ip.IHL)*4 || int(binary.BigEndian.Uint16(ip.Contents[2:4])) != len(ip.Contents)+len(ip.Payload) {
			carrierErr = protocolError(ErrMalformedMessage, "L2TPv3 requires a complete bounded IPv4 total length")
		}
	case *layers.IPv6:
		selected = ip.NextHeader == 115
		if selected && (len(ip.Contents) < 40 || ip.Version != 6 || int(binary.BigEndian.Uint16(ip.Contents[4:6])) != len(ip.Payload)) {
			carrierErr = protocolError(ErrMalformedMessage, "L2TPv3 direct IPv6 payload length mismatch")
		}
	}
	wire := network.LayerPayload()
	source, destination := network.NetworkFlow().Src().String(), network.NetworkFlow().Dst().String()
	admission := "observed-IP-protocol"
	if !selected {
		u, ok := p.TransportLayer().(*layers.UDP)
		if !ok {
			return nil, false
		}
		src, dst := l2tp3Endpoint{source, uint16(u.SrcPort)}, l2tp3Endpoint{destination, uint16(u.DstPort)}
		a.l2tp3Mu.Lock()
		admission = a.l2tp3UDPAdmission(evidence.Ref.Domain, src, dst, u.Payload)
		a.l2tp3Mu.Unlock()
		if admission == "" {
			return nil, false
		}
		wire, udp, selected = u.Payload, true, true
		source = net.JoinHostPort(source, strconv.Itoa(int(u.SrcPort)))
		destination = net.JoinHostPort(destination, strconv.Itoa(int(u.DstPort)))
		if u.Length < 8 || int(u.Length) != len(wire)+8 {
			carrierErr = protocolError(ErrMalformedMessage, "L2TPv3 UDP length mismatch")
		}
	}
	if !selected {
		return nil, false
	}
	ci := p.Metadata().CaptureInfo
	e := &ProtocolEvent{Transport: "network", Domain: evidence.Ref.Domain, Timestamp: ci.Timestamp, Source: source, Destination: destination, Admission: admission, SourceBytes: networkObservationSource(evidence)}
	e.CarrierLayers = append([]string(nil), evidence.carrierLayers...)
	if udp {
		e.Transport = "udp"
	}
	inner, suffix, parent := a.decodeL2TPv3Event(e, wire, udp, ci.CaptureLength < ci.Length || p.Metadata().Truncated, carrierErr)
	if len(inner) == 0 {
		return nil, true
	}
	if evidence.normalizedTunnels >= 4 {
		a.networkDiagnostic(evidence, ci, "limited", "L2TPEncapsulationDepth")
		return nil, true
	}
	refs := CapturePacketReferences(p)
	evidence.preserveNetworkPrefix(p, nil)
	if len(evidence.networkPrefix) < maxCaptureTransportLayers {
		evidence.networkPrefix = append(evidence.networkPrefix, captureL2TPv3Type)
	} else {
		evidence.networkPrefixLimited = true
	}
	evidence.refs = append([]PacketReference(nil), refs...)
	evidence.parents = []uint64{parent}
	evidence.Ref.Domain.Encapsulation += suffix
	evidence.byteSourceKind = "decapsulated"
	evidence.carrierOwned = true
	evidence.carrierLayers = append(append([]string(nil), evidence.carrierLayers...), "L2TPv3")
	evidence.normalizedTunnels++
	evidence.ipv6Fragment = false
	ci.CaptureLength, ci.Length = len(inner), len(inner)
	q := gopacket.NewPacket(inner, protocolPacketDecoder{decoder: layers.LayerTypeEthernet}, gopacket.DecodeOptions{NoCopy: true})
	q.Metadata().CaptureInfo = withEvidence(ci, evidence)
	if status, code := l2tp3InnerIPDiagnostic(q); code != "" {
		a.networkDiagnostic(evidence, ci, status, code)
		return nil, true
	}
	return q, true
}

// A complete Ethernet pseudowire does not establish a complete inner IP
// datagram. Validate the original bytes: the generic IPv4 decoder rewrites
// TotalLength0 to the available capture length. Ethernet padding is allowed.
func l2tp3InnerIPDiagnostic(q gopacket.Packet) (string, string) {
	eth, ok := q.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	if !ok {
		return "", ""
	}
	typ, w := eth.EthernetType, eth.Payload
	for (typ == layers.EthernetTypeDot1Q || typ == layers.EthernetTypeQinQ) && len(w) >= 4 {
		typ, w = layers.EthernetType(binary.BigEndian.Uint16(w[2:4])), w[4:]
	}
	switch typ {
	case layers.EthernetTypeIPv4:
		if len(w) < 20 {
			return "incomplete", "L2TPInnerIPv4HeaderIncomplete"
		}
		h, n := int(w[0]&15)*4, int(binary.BigEndian.Uint16(w[2:4]))
		if w[0]>>4 != 4 || h < 20 || n < h {
			return "malformed", "L2TPInnerIPv4LengthOrVersionInvalid"
		}
		if n > len(w) {
			return "incomplete", "L2TPInnerIPv4DatagramIncomplete"
		}
	case layers.EthernetTypeIPv6:
		if len(w) < 40 {
			return "incomplete", "L2TPInnerIPv6HeaderIncomplete"
		}
		if w[0]>>4 != 6 {
			return "malformed", "L2TPInnerIPv6VersionInvalid"
		}
		if int(binary.BigEndian.Uint16(w[4:6]))+40 > len(w) {
			return "incomplete", "L2TPInnerIPv6DatagramIncomplete"
		}
	}
	return "", ""
}

// Raw UDP byte-sessions have no capture transport pipeline. They expose the
// same observed association and selected pseudowire fields, without inventing
// TCP delivery from caller-supplied datagrams.
func (a *binParser) l2tp3RawDatagram(e *ProtocolEvent, wire []byte, truncated bool, udpLength uint16) bool {
	a.l2tp3Mu.Lock()
	admission := a.l2tp3UDPAdmission(e.Domain, l2tp3EndpointOf(e.Source), l2tp3EndpointOf(e.Destination), wire)
	a.l2tp3Mu.Unlock()
	if admission == "" {
		return false
	}
	e.Admission = admission
	var err error
	if udpLength < 8 || int(udpLength) != len(wire)+8 {
		err = protocolError(ErrMalformedMessage, "L2TPv3 UDP length mismatch")
	}
	a.decodeL2TPv3Event(e, wire, true, truncated, err)
	return true
}
