package pcaputil

import (
	"encoding/binary"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Retain a captured child decoder error for the strict capture API, while
// preventing it from becoming a second diagnostic or a transport candidate.
type ipv4IPIPExtentRefusedPacket struct{ gopacket.Packet }

// A zero Total Length is not an enclosing IP-in-IP wire boundary. In particular,
// normalization must not turn a dependency's TSO length substitution into an
// independently valid inner datagram, fragment contribution or TCP connection.
func (a *binParser) refuseIPv4IPIPExtent(p gopacket.Packet, evidence captureEvidence, ci gopacket.CaptureInfo) bool {
	for _, layer := range p.Layers() {
		ip, ok := layer.(*layers.IPv4)
		if !ok {
			continue
		}
		h := ip.LayerContents()
		// Failed option decodes retain the bounded header bytes but leave the
		// dependency's fixed fields at zero. They are observations, not defaults
		// from which to select a different carrier or an inner identity.
		if len(h) < 20 || h[0]>>4 != 4 || int(h[0]&15)*4 != len(h) || binary.BigEndian.Uint16(h[2:4]) != 0 || binary.BigEndian.Uint16(h[6:8])&0x3fff != 0 || h[9] != 4 && h[9] != 41 {
			continue
		}
		names := make([]string, 0, len(evidence.networkPrefix)+8)
		for _, kind := range evidence.networkPrefix {
			names = append(names, kind.String())
		}
		for _, prefix := range p.Layers() {
			if len(names) >= maxCaptureTransportLayers {
				names = nil
				break
			}
			names = append(names, prefix.LayerType().String())
			if prefix == layer {
				break
			}
		}
		fields := map[string]any{
			"Observed Header Layers": names, "Observed IP Version": uint8(4), "Observed Total Length": uint16(0), "Captured Header Bytes": len(h),
			"Observed Fragment ID": binary.BigEndian.Uint16(h[4:6]), "Observed Fragment Offset": 0, "Observed More Fragments": false,
			"Observed Next Protocol": h[9], "Fragment Offset Unit": "bytes", "Child Header Identity": "unconfirmed",
			"Header Observation Scope":        "IPv4 fixed header; enclosing IP-in-IP extent unavailable",
			"IP-in-IP Normalization Admitted": false, "Network Transport Admitted": false, "Fragment Reassembly Admitted": false,
			"Captured Length Substitution": "not evidence of an enclosing IP-in-IP wire extent",
		}
		a.malformed.Add(1)
		reason := "IPv4 IP-in-IP parent Total Length is zero; carrier extent unavailable; inner identity not admitted"
		if ip.Version != 4 {
			if failure := p.ErrorLayer(); failure != nil {
				fields["Parent Header Decode Complete"] = false
				fields["Parent Header Decode Error"] = failure.Error().Error()
				reason += "; " + failure.Error().Error()
			}
		}
		a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "ipv4-carrier-extent-refusal", Domain: evidence.Ref.Domain,
			Status: "malformed", Completeness: "headers", ExpertCode: "IPv4CarrierExtentUnavailable", Error: reason, Summary: reason,
			Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...)})
		return true
	}
	return false
}
