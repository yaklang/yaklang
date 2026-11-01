package pcaputil

import (
	"encoding/binary"
	"errors"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type captureOSPFv2Decoder struct{}

type captureOSPFv2 struct {
	layers.BaseLayer
	Fields map[string]any
}

func (*captureOSPFv2) LayerType() gopacket.LayerType { return layers.LayerTypeOSPF }

func (captureOSPFv2Decoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	if len(w) > 0 && w[0] != 2 {
		// RFC7949 also defines OSPFv3 over IPv4. An IPv4 carrier alone must
		// not turn that unselected version into a malformed OSPFv2 message.
		return layers.LayerTypeOSPF.Decode(w, b)
	}
	fields, err := stream_parser.DecodeOSPFv2Wire(w, 4096)
	b.AddLayer(&captureOSPFv2{BaseLayer: layers.BaseLayer{Contents: w[:min(24, len(w))], Payload: w[min(24, len(w)):]}, Fields: fields})
	if len(w) < 24 || len(w) >= 4 && int(binary.BigEndian.Uint16(w[2:4])) > len(w) {
		b.SetTruncated()
	}
	if errors.Is(err, stream_parser.ErrOSPFv2WireIntegrity) || errors.Is(err, stream_parser.ErrOSPFv2WireFormat) {
		return err
	}
	if errors.Is(err, stream_parser.ErrObservedWireContext) || errors.Is(err, stream_parser.ErrObservedWireBudget) {
		return nil
	}
	return err
}

func declaredOSPFv2Wire(p gopacket.Packet) ([]byte, *layers.IPv4, bool) {
	if p.ErrorLayer() != nil {
		return nil, nil, false
	}
	for _, layer := range p.Layers() {
		switch v := layer.(type) {
		case *layers.IPv4:
			if v.Protocol != layers.IPProtocolOSPF || v.FragOffset != 0 || v.Flags&layers.IPv4MoreFragments != 0 {
				continue
			}
			w := v.LayerPayload()
			if len(w) != 0 && w[0] == 2 {
				return w, v, true
			}
			return nil, nil, false
		case *layers.IPv6:
			continue
		case *layers.UDP, *layers.TCP, *layers.ICMPv4, *layers.ICMPv6, *layers.IPv6Fragment:
			return nil, nil, false
		}
	}
	return nil, nil, false
}

func (a *binParser) observeOSPFv2(p gopacket.Packet, evidence captureEvidence) bool {
	wire, ip, selected := declaredOSPFv2Wire(p)
	if !selected {
		return false
	}
	ci := p.Metadata().CaptureInfo
	captureTruncated := ci.CaptureLength < ci.Length
	if !espParentLengthBounded(ip, captureTruncated) {
		a.malformed.Add(1)
		a.networkHeaderRefusal(p, evidence, ci, "OSPFv2ParentLengthInvalid", "OSPFv2 parent IPv4 extent is invalid; control body not admitted")
		return true
	}
	e := &ProtocolEvent{Protocol: "ospf", Transport: "network", Domain: evidence.Ref.Domain,
		Timestamp: ci.Timestamp, Source: ip.SrcIP.String(), Destination: ip.DstIP.String(),
		Rule: "ospfv2_observed", Entry: "OSPFv2ControlFields", Profile: "ospfv2-bounded-control", Admission: "observed-IPv4-protocol-and-version",
		Completeness: "message", SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...), nativeSessionMessage: true}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if len(wire) > limit || len(p.Data()) > limit {
		e.Length, e.Completeness = len(wire), "limited"
		a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "OSPFv2 carrier/control message exceeds parser limit"))
		a.messageBytes.Add(uint64(len(wire)))
		a.limited.Add(uint64(len(wire)))
		a.emit(e)
		return true
	}
	fields, err := stream_parser.DecodeOSPFv2Wire(wire, a.budget.MaxCollectionElements)
	if fields != nil {
		e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
	}
	e.retainNativeOSPFv2Fields(wire)
	decodeErr := observedWireError(err)
	// Only a missing declared wire extent can be attributed to snaplen.
	// A complete body with impossible counts or a proven bad checksum remains
	// malformed even if unrelated later IP bytes were not captured.
	required, _ := fields["Required Captured Bytes"].(uint64)
	if captureTruncated && required > uint64(len(wire)) && !errors.Is(err, stream_parser.ErrOSPFv2WireIntegrity) && !errors.Is(err, stream_parser.ErrOSPFv2WireFormat) && !errors.Is(err, stream_parser.ErrObservedWireBudget) {
		decodeErr = protocolError(ErrNeedMore, "OSPFv2 capture is incomplete; only captured complete fields are available")
	}
	a.finishProtocolDatagram(e, wire, nil, decodeErr)
	if decodeErr == nil {
		e.Admission = "observed-IPv4-protocol-and-complete-control-grammar"
		e.Summary = "OSPFv2 " + fields["Packet Name"].(string)
	} else {
		e.Completeness = "limited"
	}
	a.emit(e)
	return true
}
