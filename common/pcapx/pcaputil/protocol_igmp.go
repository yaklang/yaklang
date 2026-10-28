package pcaputil

import (
	"bytes"
	"errors"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Only the native receiver can create this marker. It retains the actual
// ErrorLayer for strict capture accounting while preventing redispatch and a
// second generic IP event after the precise IGMP prefix has been published.
type igmpObservedErrorPacket struct{ gopacket.Packet }

// DecodeIGMPPacketFields selects a literal IPv4 protocol2 membership payload.
// Selection does not establish validity of the parent, checksum or membership
// state. Native ingress applies its parent admission before calling this.
func DecodeIGMPPacketFields(p gopacket.Packet, maxBytes, maxElements int) (result stream_parser.IGMPWireResult, offset int, selected bool, err error) {
	wire, offset, declared, ok := igmpIPv4Payload(p)
	if !ok || len(wire) > 0 && !stream_parser.IGMPMembershipType(wire[0]) {
		return
	}
	selected = true
	result, err = stream_parser.DecodeIGMPWire(wire, maxBytes, maxElements, stream_parser.IGMPWireContext{DeclaredBytes: declared})
	shiftMLDWireRanges(result.Fields, offset)
	return
}

// IGMP's gopacket layer has no reliable BaseLayer and drops v3 body errors.
// Locate its input through the complete, selected IPv4 header instead. A child
// failure must own this exact decoder-input slice; parent errors are not IGMP.
func igmpIPv4Payload(p gopacket.Packet) (wire []byte, offset, declared int, ok bool) {
	ip, valid := p.NetworkLayer().(*layers.IPv4)
	if !valid || ip.Version != 4 || ip.Protocol != layers.IPProtocolIGMP || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 {
		return
	}
	h := ip.LayerContents()
	if ip.IHL < 5 || len(h) != int(ip.IHL)*4 || len(h) < 20 || h[0]>>4 != 4 {
		return
	}
	at, located := nativeNetworkLayerOffset(p, ip)
	if !located || at+len(h) > len(p.Data()) {
		return
	}
	offset = at + len(h)
	wire = ip.LayerPayload()
	if len(wire) > len(p.Data())-offset || !bytes.Equal(wire, p.Data()[offset:offset+len(wire)]) {
		return nil, 0, 0, false
	}
	declared = -1
	if ip.Length != 0 {
		if int(ip.Length) < len(h) || len(wire) > int(ip.Length)-len(h) {
			return nil, 0, 0, false
		}
		declared = int(ip.Length) - len(h)
	}
	if failure := p.ErrorLayer(); failure != nil && !bytes.Equal(failure.LayerContents(), wire) {
		// Includes malformed/truncated IPv4 options and unrelated deeper errors.
		// Equality here proves known decoder-input coordinates, not a Raw search.
		return nil, 0, 0, false
	}
	return wire, offset, declared, true
}

// Call only after existing topology, fragment, IP-version and parent admission.
// This first profile deliberately does not use a lost GTP/unknown tunnel
// context as evidence of an admitted inner IGMP message.
func (a *binParser) observeIGMP(p gopacket.Packet, evidence captureEvidence) (observed, preserveDecodeError bool) {
	if evidence.normalizedTunnels != 0 {
		return false, false
	}
	wire, offset, _, ok := igmpIPv4Payload(p)
	if !ok || len(wire) > 0 && !stream_parser.IGMPMembershipType(wire[0]) {
		return false, false
	}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if len(p.Data()) > limit {
		// Do not clone a large captured frame merely to publish a budget refusal.
		ci := p.Metadata().CaptureInfo
		_, typed := classifySessionError(protocolError(ErrResourceExceeded, "IGMP native frame exceeds parser message limit"))
		e := &ProtocolEvent{Timestamp: ci.Timestamp, Protocol: "igmp", Transport: "network", Domain: evidence.Ref.Domain,
			Profile: "igmp-membership-wire", Admission: "observed-IPv4-protocol2-only", Length: len(p.Data()),
			Status: "limited", Completeness: "limited", ExpertCode: "ResourceExceeded", Error: "IGMP native frame exceeds parser message limit",
			SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true,
			sessionError: typed}
		e.CarrierLayers = append([]string(nil), evidence.carrierLayers...)
		if ip := p.NetworkLayer(); ip != nil {
			e.Source, e.Destination = ip.NetworkFlow().Src().String(), ip.NetworkFlow().Dst().String()
		}
		a.limited.Add(uint64(len(p.Data())))
		a.emit(e)
		return true, p.ErrorLayer() != nil
	}
	result, _, _, decodeErr := DecodeIGMPPacketFields(p, limit, a.budget.MaxCollectionElements)
	status, code := "decoded", ""
	if decodeErr != nil {
		status, code = "limited", "IGMPCollectionBudget"
		var failure *stream_parser.IGMPWireError
		if errors.As(decodeErr, &failure) {
			status, code = failure.Kind, failure.Code
		}
	}
	ci := p.Metadata().CaptureInfo
	completeness := status
	if status == "decoded" {
		completeness = "message"
	}
	e := &ProtocolEvent{Timestamp: ci.Timestamp, Protocol: "igmp", Transport: "network", Domain: evidence.Ref.Domain,
		Rule: "network_igmp", Entry: "IGMPFields", Profile: "igmp-membership-wire", Admission: "observed-IPv4-protocol2-only",
		Length: len(p.Data()), Raw: bytes.Clone(p.Data()), Status: status, Completeness: completeness,
		ExpertCode: code, Error: code, Summary: result.Summary, SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true}
	e.CarrierLayers = append([]string(nil), evidence.carrierLayers...)
	if ip := p.NetworkLayer(); ip != nil {
		e.Source, e.Destination = ip.NetworkFlow().Src().String(), ip.NetworkFlow().Dst().String()
	}
	if result.Fields != nil {
		e.Session, e.semanticFields, e.semanticSession = cloneSession(result.Fields), cloneSession(result.Fields), cloneSession(result.Fields)
		e.Structured = map[string]any{"fields": cloneSession(result.Fields)}
		e.retainNativeIGMPFields(result, offset, len(wire))
	}
	if status == "decoded" {
		e.Admission = "observed-IPv4-protocol2-and-complete-membership-grammar"
		a.messages.Add(1)
		a.decoded.Add(1)
	} else if status == "incomplete" {
		a.incomplete.Add(1)
	} else if status == "malformed" {
		a.malformed.Add(1)
	} else {
		a.limited.Add(uint64(len(p.Data())))
	}
	a.emit(e)
	return true, p.ErrorLayer() != nil
}

func (e *ProtocolEvent) retainNativeIGMPFields(r stream_parser.IGMPWireResult, offset, available int) {
	// Add igmpValues map[string][]nativeNetworkField to nativeNetworkObservation.
	n := &nativeNetworkObservation{protocol: "igmp", values: map[string]nativeNetworkField{}, igmpValues: map[string][]nativeNetworkField{}}
	for _, f := range r.Values {
		if f.Name == "" || f.Offset < 0 || f.Length <= 0 || f.Offset > available || f.Length > available-f.Offset {
			continue
		}
		n.igmpValues[f.Name] = append(n.igmpValues[f.Name], nativeNetworkField{value: f.Value, span: DisplayFieldRange{Offset: offset + f.Offset, Length: f.Length}})
	}
	e.nativeNetworkFields = n
}
