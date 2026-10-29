package pcaputil

import (
	"encoding/binary"
	"errors"
	"net/netip"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Observer and native views use the same bounded wire grammar, without
// changing the global dependency decoder or admitting an ICMP quote as traffic.
type captureOSPFv3Decoder struct{ network gopacket.NetworkLayer }

type captureOSPFv3 struct {
	layers.BaseLayer
	Fields map[string]any
}

func (*captureOSPFv3) LayerType() gopacket.LayerType { return layers.LayerTypeOSPF }

func (d captureOSPFv3Decoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	fields, err := stream_parser.DecodeOSPFv3Wire(w, ospfv3Context(d.network), 4096)
	l := &captureOSPFv3{BaseLayer: layers.BaseLayer{Contents: w[:min(16, len(w))], Payload: w[min(16, len(w)):]}, Fields: fields}
	b.AddLayer(l)
	if len(w) < 16 || len(w) >= 4 && int(binary.BigEndian.Uint16(w[2:4])) > len(w) {
		b.SetTruncated()
	}
	if errors.Is(err, stream_parser.ErrOSPFv3WireIntegrity) {
		return err // A resource/context limit cannot hide proven wire corruption.
	}
	if errors.Is(err, stream_parser.ErrObservedWireContext) || errors.Is(err, stream_parser.ErrObservedWireBudget) {
		// Unselected semantics and a local parser budget are not proof of a
		// malformed captured packet. Native events retain the precise refusal.
		return nil
	}
	return err
}

func ospfv3Context(network gopacket.NetworkLayer) stream_parser.OSPFv3WireContext {
	var context stream_parser.OSPFv3WireContext
	if ip, ok := network.(*layers.IPv6); ok {
		context.Source, _ = netip.AddrFromSlice(ip.SrcIP)
		context.Destination, _ = netip.AddrFromSlice(ip.DstIP)
		// RFC8200's pseudo-header uses the final destination of an active
		// routing header. Unknown routing semantics must not become a bad sum.
		next, w := ip.NextHeader, ip.LayerPayload()
		if ip.HopByHop != nil {
			next = ip.HopByHop.NextHeader
		}
		for count := 0; next != layers.IPProtocolOSPF && count < 16; count++ {
			if len(w) < 2 {
				context.PseudoHeaderUnavailable = true
				break
			}
			n := 0
			switch next {
			case layers.IPProtocolIPv6HopByHop, layers.IPProtocolIPv6Destination, layers.IPProtocolIPv6Routing:
				n = (int(w[1]) + 1) * 8
			case layers.IPProtocolAH:
				n = (int(w[1]) + 2) * 4
			default:
				context.PseudoHeaderUnavailable = true
				return context
			}
			if n > len(w) || n < 8 {
				context.PseudoHeaderUnavailable = true
				break
			}
			if next == layers.IPProtocolIPv6Routing && w[3] != 0 {
				if (w[2] == 0 || w[2] == 2) && n >= 24 && (n-8)%16 == 0 && int(w[3]) <= (n-8)/16 {
					context.Destination = netip.AddrFrom16([16]byte(w[n-16 : n]))
				} else {
					context.PseudoHeaderUnavailable = true
				}
			}
			next, w = layers.IPProtocol(w[0]), w[n:]
		}
		if next != layers.IPProtocolOSPF {
			context.PseudoHeaderUnavailable = true
		}
	}
	return context
}

func declaredOSPFv3Wire(p gopacket.Packet) ([]byte, gopacket.NetworkLayer, bool) {
	// This is the private analysis view: no application decoder has run.
	// Partial/error-bearing extension headers cannot select a child protocol.
	if p.ErrorLayer() != nil {
		return nil, nil, false
	}
	var network gopacket.NetworkLayer
	for _, l := range p.Layers() {
		var next layers.IPProtocol
		switch v := l.(type) {
		case *layers.IPv4:
			network = nil // IPv4 OSPFv2 is a separate profile.
		case *layers.IPv6:
			network, next = v, v.NextHeader
		case *layers.IPv6HopByHop:
			next = v.NextHeader
		case *layers.IPv6Destination:
			next = v.NextHeader
		case *layers.IPv6Routing:
			next = v.NextHeader
		case *layers.IPSecAH:
			next = v.NextHeader
		case *layers.IPv6Fragment, *layers.UDP, *layers.TCP, *layers.ICMPv4, *layers.ICMPv6:
			return nil, nil, false
		default:
			continue
		}
		if next == layers.IPProtocolOSPF && network != nil {
			return l.LayerPayload(), network, true
		}
	}
	return nil, nil, false
}

func (a *binParser) observeOSPFv3(p gopacket.Packet, evidence captureEvidence) bool {
	wire, network, selected := declaredOSPFv3Wire(p)
	if !selected {
		return false
	}
	ci := p.Metadata().CaptureInfo
	if !espParentLengthBounded(network, p.Metadata().Truncated) {
		a.malformed.Add(1)
		a.networkHeaderRefusal(p, evidence, ci, "OSPFv3ParentLengthInvalid", "OSPFv3 parent IPv6 extent is invalid; control body not admitted")
		return true
	}
	e := &ProtocolEvent{Protocol: "ospf", Transport: "network", Domain: evidence.Ref.Domain,
		Timestamp: ci.Timestamp, Source: network.NetworkFlow().Src().String(), Destination: network.NetworkFlow().Dst().String(),
		Rule: "ospfv3_observed", Entry: "OSPFv3ControlFields", Profile: "ospfv3-bounded-control", Admission: "observed-IPv6-next-header",
		Completeness: "message", SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...), nativeSessionMessage: true}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if len(wire) > limit || len(p.Data()) > limit {
		e.Length, e.Completeness = len(wire), "limited"
		a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "OSPFv3 carrier/control message exceeds parser limit"))
		a.messageBytes.Add(uint64(len(wire)))
		a.limited.Add(uint64(len(wire)))
		a.emit(e)
		return true
	}
	fields, err := stream_parser.DecodeOSPFv3Wire(wire, ospfv3Context(network), a.budget.MaxCollectionElements)
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
	}
	e.retainNativeOSPFv3Fields(wire)
	decodeErr := observedWireError(err)
	if ci.CaptureLength < ci.Length || p.Metadata().Truncated {
		decodeErr = protocolError(ErrNeedMore, "OSPFv3 capture is incomplete; only captured complete fields are available")
	}
	a.finishProtocolDatagram(e, wire, nil, decodeErr)
	if decodeErr == nil {
		e.Admission = "observed-IPv6-next-header-and-complete-control-grammar"
		e.Summary = "OSPFv3 " + fields["Packet Name"].(string)
	} else {
		e.Completeness = "limited"
	}
	a.emit(e)
	return true
}
