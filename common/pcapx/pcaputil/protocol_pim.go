package pcaputil

import (
	"net/netip"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Observe control fields before Register normalization discards the carrier.
// A Null-Register is consumed here and can never create a transport flow.
// IP fragment assembly still precedes this admission: non-final fragments do
// not supply a complete PIM message, even if their prefix looks convincing.
func (a *binParser) observePIM(p gopacket.Packet, evidence captureEvidence) bool {
	lastNetworkPIM := false
	for _, layer := range p.Layers() {
		network, ok := layer.(gopacket.NetworkLayer)
		if !ok {
			continue
		}
		lastNetworkPIM = false
		switch ip := network.(type) {
		case *layers.IPv4:
			if ip.Protocol != 103 || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 {
				continue
			}
		case *layers.IPv6:
			if ip.NextHeader != 103 {
				continue
			}
		default:
			continue
		}
		lastNetworkPIM = true
		wire := network.LayerPayload()
		ci := p.Metadata().CaptureInfo
		e := &ProtocolEvent{Protocol: "pim", Transport: "network", Domain: evidence.Ref.Domain,
			Timestamp: ci.Timestamp, Source: network.NetworkFlow().Src().String(), Destination: network.NetworkFlow().Dst().String(),
			Rule: "pim_control", Entry: "PIMControlFields", Profile: "pim2-bounded-control-advertisements",
			Admission: "observed-IP-protocol-only", Completeness: "message",
			SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true}
		if len(wire) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
			a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "PIM message exceeds parser limit"))
			// Refusing to retain the body must not turn an observed nonempty
			// message into a zero-byte event or undercount its limited bytes.
			e.Length = len(wire)
			e.Completeness = "limited"
			a.messageBytes.Add(uint64(len(wire)))
			a.limited.Add(uint64(len(wire)))
			a.emit(e)
			continue
		}
		src, _ := netip.ParseAddr(e.Source)
		dst, _ := netip.ParseAddr(e.Destination)
		m, err := stream_parser.DecodePIMControl(wire, stream_parser.PIMWireContext{Source: src, Destination: dst, DeclaredBytes: pimDeclaredPayloadBytes(network)}, a.budget.MaxCollectionElements)
		if m != nil {
			e.Session = m
			e.semanticFields, e.semanticSession = cloneSession(m), cloneSession(m)
		}
		e.retainNativePIMFields(wire)
		a.finishProtocolDatagram(e, wire, nil, observedWireError(err))
		if err != nil {
			// The complete selected field tree was not available. In
			// particular, a negative field predicate must not treat unparsed
			// extensions or a collection limit as proved field absence.
			e.Completeness = "limited"
		}
		if err == nil {
			e.Admission = "observed-IP-protocol-and-complete-control-grammar"
			e.Summary = "PIMv2 " + m["Packet Name"].(string)
		}
		a.emit(e)
	}
	return lastNetworkPIM
}
