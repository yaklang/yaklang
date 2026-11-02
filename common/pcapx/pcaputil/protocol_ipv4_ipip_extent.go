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
		if !ok || ip.Version != 4 || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 || ip.Protocol != layers.IPProtocolIPv4 && ip.Protocol != layers.IPProtocolIPv6 {
			continue
		}
		h := ip.LayerContents()
		if len(h) < 20 || h[0]>>4 != 4 || int(h[0]&15)*4 != len(h) || binary.BigEndian.Uint16(h[2:4]) != 0 {
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
			"Observed Next Protocol": uint8(ip.Protocol), "Fragment Offset Unit": "bytes", "Child Header Identity": "unconfirmed",
			"Header Observation Scope":        "IPv4 fixed header; enclosing IP-in-IP extent unavailable",
			"IP-in-IP Normalization Admitted": false, "Network Transport Admitted": false, "Fragment Reassembly Admitted": false,
			"Captured Length Substitution": "not evidence of an enclosing IP-in-IP wire extent",
		}
		a.malformed.Add(1)
		const reason = "IPv4 IP-in-IP parent Total Length is zero; carrier extent unavailable; inner identity not admitted"
		a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "ipv4-carrier-extent-refusal", Domain: evidence.Ref.Domain,
			Status: "malformed", Completeness: "headers", ExpertCode: "IPv4CarrierExtentUnavailable", Error: reason, Summary: reason,
			Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...)})
		return true
	}
	return false
}
