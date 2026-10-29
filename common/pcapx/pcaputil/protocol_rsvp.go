package pcaputil

import (
	"errors"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

var captureRSVPType = gopacket.RegisterLayerType(200006, gopacket.LayerTypeMetadata{Name: "RSVP", Decoder: captureRSVPDecoder{}})

type captureRSVPDecoder struct{}

// RSVPObservedWireError distinguishes this terminal observer from an invalid
// enclosing carrier; native inspection still owns its bounded partial fields.
type RSVPObservedWireError struct{ Cause error }

func (e *RSVPObservedWireError) Error() string { return e.Cause.Error() }
func (e *RSVPObservedWireError) Unwrap() error { return e.Cause }

type captureRSVP struct {
	layers.BaseLayer
	Fields map[string]any
}

func (*captureRSVP) LayerType() gopacket.LayerType { return captureRSVPType }
func (captureRSVPDecoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	fields, err := stream_parser.DecodeRSVPWire(w, 4096)
	b.AddLayer(&captureRSVP{BaseLayer: layers.BaseLayer{Contents: w[:min(8, len(w))], Payload: w[min(8, len(w)):]}, Fields: fields})
	if len(w) < 8 {
		b.SetTruncated()
	}
	if errors.Is(err, stream_parser.ErrObservedWireContext) || errors.Is(err, stream_parser.ErrObservedWireBudget) {
		return nil
	}
	if err != nil {
		return &RSVPObservedWireError{Cause: err}
	}
	return nil
}

// Select only a literal next-header46, before a quote or application decoder.
func rsvpNextLayer(layer gopacket.Layer, network gopacket.NetworkLayer) bool {
	_, ipv6Parent := network.(*layers.IPv6)
	switch l := layer.(type) {
	case *layers.IPv4:
		return l.Protocol == 46 && l.FragOffset == 0 && l.Flags&layers.IPv4MoreFragments == 0
	case *layers.IPv6:
		return l.NextHeader == 46
	case *layers.IPv6HopByHop:
		return ipv6Parent && l.NextHeader == 46
	case *layers.IPv6Destination:
		return ipv6Parent && l.NextHeader == 46
	case *layers.IPv6Routing:
		return ipv6Parent && l.NextHeader == 46
	case *layers.IPSecAH:
		return l.NextHeader == 46
	}
	return false
}
func declaredRSVPWire(p gopacket.Packet) ([]byte, gopacket.NetworkLayer, bool) {
	if p.ErrorLayer() != nil {
		return nil, nil, false
	}
	var network gopacket.NetworkLayer
	for _, layer := range p.Layers() {
		switch l := layer.(type) {
		case *layers.IPv4:
			if network != nil {
				return nil, nil, false
			}
			network = l
		case *layers.IPv6:
			if network != nil {
				return nil, nil, false
			}
			network = l
		case *layers.IPv6HopByHop, *layers.IPv6Destination, *layers.IPv6Routing, *layers.IPSecAH:
		case *layers.IPv6Fragment, *layers.UDP, *layers.TCP, *layers.ICMPv4, *layers.ICMPv6:
			return nil, nil, false
		default:
			continue
		}
		if network != nil && rsvpNextLayer(layer, network) {
			return layer.LayerPayload(), network, true
		}
	}
	return nil, nil, false
}

func (a *binParser) observeRSVP(p gopacket.Packet, evidence captureEvidence) bool {
	wire, network, selected := declaredRSVPWire(p)
	if !selected {
		return false
	}
	ci := p.Metadata().CaptureInfo
	short := ci.CaptureLength < ci.Length
	if !espParentLengthBounded(network, short) {
		a.malformed.Add(1)
		a.networkHeaderRefusal(p, evidence, ci, "RSVPParentLengthInvalid", "RSVP parent extent invalid; control body not admitted")
		return true
	}
	source, destination := "", ""
	switch ip := network.(type) {
	case *layers.IPv4:
		source, destination = ip.SrcIP.String(), ip.DstIP.String()
	case *layers.IPv6:
		source, destination = ip.SrcIP.String(), ip.DstIP.String()
	}
	e := &ProtocolEvent{Protocol: "rsvp", Transport: "network", Domain: evidence.Ref.Domain, Timestamp: ci.Timestamp, Source: source, Destination: destination, Rule: "rsvp_observed", Entry: "RSVPControlFields", Profile: "rsvp-bounded-control", Admission: "observed-IP-next-header46", Completeness: "message", SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...), nativeSessionMessage: true}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if len(wire) > limit || len(p.Data()) > limit {
		e.Length, e.Completeness = len(wire), "limited"
		a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "RSVP carrier/control message exceeds parser limit"))
		a.messageBytes.Add(uint64(len(wire)))
		a.limited.Add(uint64(len(wire)))
		a.emit(e)
		return true
	}
	fields, err := stream_parser.DecodeRSVPWire(wire, a.budget.MaxCollectionElements)
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
		e.nativeFieldsIncomplete = fields["Field Layouts Complete"] != true
	}
	e.retainNativeRSVPFields(wire)
	decodeErr := observedWireError(err)
	required, _ := fields["Required Captured Bytes"].(uint64)
	header, _ := fields["Header"].(map[string]any)
	if !short && header["Version"] == uint64(1) && required > uint64(len(wire)) {
		// RFC2205: one message occupies one IP datagram. A complete parent
		// proves wrong length independently of object budget or an unselected
		// message type; a bare wire decoder cannot make that ingress claim.
		decodeErr = protocolError(ErrMalformedMessage, "complete IP payload has %d RSVP bytes; selected v1 extent requires %d: %v", len(wire), required, err)
	} else if short && required > uint64(len(wire)) && !errors.Is(err, stream_parser.ErrRSVPWireIntegrity) && !errors.Is(err, stream_parser.ErrRSVPWireFormat) && !errors.Is(err, stream_parser.ErrObservedWireBudget) {
		decodeErr = protocolError(ErrNeedMore, "RSVP capture incomplete; only complete captured primitives available")
	}
	a.finishProtocolDatagram(e, wire, nil, decodeErr)
	if decodeErr == nil {
		e.Admission = "observed-IP-next-header46-and-complete-base-wire"
		e.Summary = "RSVP " + fields["Message Name"].(string) + " · advertised selectors; reservation/authentication unverified"
	} else {
		e.Completeness = "limited"
	}
	a.emit(e)
	return true
}
