package pcaputil

import (
	"encoding/binary"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// The dependency substitutes captured length for a zero IPv4 Total Length to
// observe host TSO buffers. That inference does not establish a fragment's wire
// extent. Refuse it before fragment clocks, ID quarantine, budgets or bytes can
// change; a subsequent intact capture remains independently admissible.
func (a *binParser) refuseIPv4FragmentExtent(p gopacket.Packet, evidence captureEvidence, ci gopacket.CaptureInfo) bool {
	for _, layer := range p.Layers() {
		ip, ok := layer.(*layers.IPv4)
		if !ok || ip.Version != 4 || ip.FragOffset == 0 && ip.Flags&layers.IPv4MoreFragments == 0 {
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
			"Observed Header Layers": names, "Observed IP Version": uint8(4), "Observed Total Length": uint16(0),
			"Captured Header Bytes": len(h), "Observed Fragment ID": binary.BigEndian.Uint16(h[4:6]),
			"Observed Fragment Offset": int(ip.FragOffset) * 8, "Observed More Fragments": ip.Flags&layers.IPv4MoreFragments != 0,
			"Header Observation Scope":     "IPv4 fixed header and fragment flags; datagram extent unavailable",
			"Fragment Reassembly Admitted": false, "Child Header Identity": "unconfirmed", "Captured Length Substitution": "not evidence of a fragment wire extent",
		}
		a.malformed.Add(1)
		const reason = "IPv4 fragment Total Length is zero; datagram extent unavailable; reassembly not admitted"
		a.emit(&ProtocolEvent{Timestamp: ci.Timestamp, Transport: "network", Protocol: "ip", Profile: "ipv4-fragment-extent-refusal", Domain: evidence.Ref.Domain,
			Status: "malformed", Completeness: "headers", ExpertCode: "IPv4FragmentExtentUnavailable", Error: reason, Summary: reason,
			Session: cloneSession(fields), semanticSession: cloneSession(fields), SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...)})
		return true
	}
	return false
}
