package pcaputil

import (
	"bytes"
	"encoding/binary"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// PPPoEHeaderObservation is a snapshot of a native refusal's observed six-byte
// header. Raw is that header only, never a reconstructed PPP/IP/application PDU.
// A public map or JSON DTO cannot construct the private witness behind it.
type PPPoEHeaderObservation struct {
	Raw                         []byte
	Fields                      map[string]any
	Status, ExpertCode, Summary string
	Domain                      CaptureDomain
	SourceBytes                 ByteSource
}

type pppoeHeaderObservation struct {
	raw                   [6]byte
	fields                map[string]any
	status, code, summary string
	domain                CaptureDomain
	source                ByteSource
}

func clonePPPoEHeaderSource(s ByteSource) ByteSource {
	s.PacketRefs = append([]PacketReference(nil), s.PacketRefs...)
	s.ParentPDUs = append([]uint64(nil), s.ParentPDUs...)
	if s.CompletedIn != nil {
		v := *s.CompletedIn
		s.CompletedIn = &v
	}
	return s
}

func (e *ProtocolEvent) NativePPPoEHeaderObservation() (PPPoEHeaderObservation, bool) {
	if e == nil || e.nativePPPoEHeader == nil {
		return PPPoEHeaderObservation{}, false
	}
	n := e.nativePPPoEHeader
	return PPPoEHeaderObservation{Raw: bytes.Clone(n.raw[:]), Fields: cloneSession(n.fields), Status: n.status, ExpertCode: n.code, Summary: n.summary, Domain: n.domain, SourceBytes: clonePPPoEHeaderSource(n.source)}, true
}

// The actual physical header must still match this owned snapshot. Editable
// Raw/Session/SourceBytes fields do not supply either native or range authority.
func (e *ProtocolEvent) NativePPPoEHeaderWireMatches(header []byte) bool {
	return e != nil && e.nativePPPoEHeader != nil && len(header) == 6 && bytes.Equal(header, e.nativePPPoEHeader.raw[:])
}

func pppoeHeaderRefusal(p gopacket.Packet, evidence captureEvidence, ci gopacket.CaptureInfo) *ProtocolEvent {
	peersKnown := false
	carrierKnown := false
	var carrier layers.EthernetType
	names := make([]string, 0, len(evidence.networkPrefix)+8)
	for _, typ := range evidence.networkPrefix {
		if len(names) < maxCaptureTransportLayers {
			names = append(names, typ.String())
		}
	}
	limited := evidence.networkPrefixLimited
	for _, layer := range p.Layers() {
		kind := layer.LayerType()
		if kind != gopacket.LayerTypePayload && kind != gopacket.LayerTypeDecodeFailure {
			if len(names) < maxCaptureTransportLayers {
				names = append(names, kind.String())
			} else {
				limited = true
			}
		}
		switch v := layer.(type) {
		case *layers.Ethernet:
			peersKnown = len(v.SrcMAC) == 6 && len(v.DstMAC) == 6
			carrier, carrierKnown = v.EthernetType, true
		case *layers.Dot1Q:
			carrier, carrierKnown = v.Type, true
		case *layers.SNAP:
			carrier, carrierKnown = v.Type, true
		case *layers.LinuxSLL:
			peersKnown = false
			carrier, carrierKnown = v.EthernetType, true
		case *layers.LinuxSLL2:
			peersKnown = false
			carrier, carrierKnown = v.ProtocolType, true
		case *layers.PPPoE:
			h := v.LayerContents()
			if len(h) != 6 {
				return nil
			}
			status, code, reason := "malformed", "PPPoEHeaderAdmission", ""
			switch {
			case h[0]>>4 != 1:
				status, code, reason = "limited", "PPPoEHeaderProfile", "PPPoE version is not the selected version 1"
			case h[0]&15 != 1:
				status, code, reason = "limited", "PPPoEHeaderProfile", "PPPoE type is not the selected type 1"
			case carrierKnown && carrier == layers.EthernetTypePPPoEDiscovery && v.Code == layers.PPPoECodeSession:
				reason = "PPPoE discovery EtherType cannot select PPP session payload"
			case carrierKnown && carrier == layers.EthernetTypePPPoESession && v.Code != layers.PPPoECodeSession:
				reason = "PPPoE session EtherType requires Code 0"
			case v.Code != layers.PPPoECodeSession:
				return nil // Discovery keeps its existing bounded tag decoder; never PPP.
			case v.SessionId == 0 || v.SessionId == 0xffff:
				reason = "PPPoE session ID is reserved"
			case int(binary.BigEndian.Uint16(h[4:6])) != len(v.LayerPayload()):
				reason = "PPPoE declared payload exceeds its captured envelope"
			case pppoeChildIPExceedsPayload(v.LayerPayload()):
				reason = "declared child IP length exceeds the PPPoE payload boundary"
			case !peersKnown:
				status, code, reason = "context-required", "PPPoEOwnerUnconfirmed", "complete Ethernet source/destination pair unavailable"
			default:
				return nil
			}
			fields := map[string]any{
				"Observed Header Layers":   names,
				"Header Observation Scope": "PPPoE fixed header only; child identity not admitted",
				"PPPoE Version":            h[0] >> 4, "PPPoE Type": h[0] & 15, "PPPoE Code": h[1],
				"PPPoE Session ID": binary.BigEndian.Uint16(h[2:4]), "PPPoE Payload Length": binary.BigEndian.Uint16(h[4:6]),
				"PPPoE Session Owner": "unconfirmed", "Parent Admission Reason": reason,
				"Child Header Identity": "unconfirmed", "Fragment Reassembly Admitted": false, "Network Transport Admitted": false,
			}
			if carrierKnown {
				fields["PPPoE Enclosing EtherType"] = uint16(carrier)
			}
			if limited {
				fields["Header Layer Projection Limited"] = true
			}
			summary := "PPPoE header observed; " + reason + "; inner identity not admitted"
			if status == "context-required" {
				summary = "PPPoE header observed; session owner unconfirmed; complete Ethernet peer pair unavailable"
			}
			n := &pppoeHeaderObservation{fields: cloneSession(fields), status: status, code: code, summary: summary, domain: evidence.Ref.Domain, source: networkObservationSource(evidence)}
			copy(n.raw[:], h)
			return &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "network-encapsulation-refusal", Domain: evidence.Ref.Domain, Status: status, Completeness: "headers", ExpertCode: code, Summary: summary, Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: clonePPPoEHeaderSource(n.source), CarrierLayers: append([]string(nil), evidence.carrierLayers...), nativePPPoEHeader: n}
		}
	}
	return nil
}

func (a *binParser) refuseUnconfirmedPPPoEOwner(p gopacket.Packet, evidence captureEvidence, ci gopacket.CaptureInfo) bool {
	if e := pppoeHeaderRefusal(p, evidence, ci); e != nil {
		if e.Status == "malformed" {
			a.malformed.Add(1)
		}
		a.emit(e)
		return true
	}
	return false
}

// ObservePPPoEHeaderAtCapture derives a fresh native header refusal from the
// actual physical frame. This is not a DTO restoration API and admits no child
// state. Workers can independently prove header facts when frame bytes exist;
// metadata-only DTOs cannot obtain a native witness or frame ranges.
func ObservePPPoEHeaderAtCapture(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType, reference PacketReference) (*ProtocolEvent, bool) {
	if len(raw) == 0 || reference.Number == 0 {
		return nil, false
	}
	base := CaptureDomain{Section: reference.Domain.Section, Interface: reference.Domain.Interface}
	ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: reference.Number, Domain: base}})
	p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: link}, gopacket.Default)
	p.Metadata().CaptureInfo = ci
	e := packetEvidence(p)
	if e.Ref.Domain != reference.Domain {
		return nil, false
	}
	_, networks, tunnels, unsupported := networkCarrierProfile(p)
	if unsupported || networks > 1 && (tunnels == 0 || tunnels > 4 || networks != tunnels+1) {
		return nil, false
	}
	event := pppoeHeaderRefusal(p, e, ci)
	return event, event != nil
}

// PPP's compressed one-byte protocol field is valid. Bound only the existing
// IPv4/IPv6 child profiles; Ethernet padding outside Length is never child data.
func pppoeChildIPExceedsPayload(payload []byte) bool {
	w := payload
	if len(w) >= 2 && w[0] == 0xff && w[1] == 3 {
		w = w[2:]
	}
	if len(w) == 0 {
		return false
	}
	protocol := uint16(w[0])
	if w[0]&1 != 0 {
		w = w[1:]
	} else {
		if len(w) < 2 {
			return false
		}
		protocol = binary.BigEndian.Uint16(w[:2])
		w = w[2:]
	}
	switch protocol {
	case 0x21:
		return len(w) >= 20 && w[0]>>4 == 4 && int(binary.BigEndian.Uint16(w[2:4])) > len(w)
	case 0x57:
		return len(w) >= 40 && w[0]>>4 == 6 && 40+int(binary.BigEndian.Uint16(w[4:6])) > len(w)
	}
	return false
}
