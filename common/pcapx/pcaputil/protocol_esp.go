package pcaputil

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Selection is an explicit network/extension Next Header, never a UDP service
// port or a two-word payload heuristic. ICMP quotes are not live ESP traffic.
func declaredESPWire(p gopacket.Packet) ([]byte, gopacket.NetworkLayer, bool) {
	if failure := p.ErrorLayer(); failure != nil {
		var short *espHeaderTruncatedError
		if !errors.As(failure.Error(), &short) {
			// Some extension decoders add their partial layer before returning
			// an option/length error. Its NextHeader cannot admit a child PDU.
			return nil, nil, false
		}
	}
	var network gopacket.NetworkLayer
	for _, l := range p.Layers() {
		var next layers.IPProtocol
		switch v := l.(type) {
		case *layers.IPv4:
			network, next = v, v.Protocol
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
		case *layers.UDP, *layers.TCP, *layers.ICMPv4, *layers.ICMPv6:
			return nil, nil, false
		default:
			continue
		}
		if next == layers.IPProtocolESP && network != nil {
			return l.LayerPayload(), network, true
		}
	}
	return nil, nil, false
}

func (a *binParser) observeESP(p gopacket.Packet, evidence captureEvidence) bool {
	wire, network, selected := declaredESPWire(p)
	if !selected {
		return false
	}
	ci := p.Metadata().CaptureInfo
	if !espParentLengthBounded(network, p.Metadata().Truncated) {
		a.malformed.Add(1)
		a.networkHeaderRefusal(p, evidence, ci, "ESPParentLengthInvalid", "ESP parent IP length is invalid; clear ESP header not admitted")
		return true
	}
	e := &ProtocolEvent{
		Timestamp: ci.Timestamp, Transport: "network", Protocol: "esp", Rule: "ipsec_esp", Entry: "ESPHeaderFields",
		Profile: "esp-observed-header", Admission: "observed-IP-next-header", Domain: evidence.Ref.Domain,
		Source: network.NetworkFlow().Src().String(), Destination: network.NetworkFlow().Dst().String(),
		Length: len(wire), Status: "limited", Completeness: "headers", nativeESPObservation: true,
		SourceBytes: networkObservationSource(evidence), CarrierLayers: append([]string(nil), evidence.carrierLayers...),
	}
	limit := min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes)
	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(wire)))
	if len(p.Data()) > limit || len(wire) > limit {
		_, e.sessionError = classifySessionError(protocolError(ErrResourceExceeded, "ESP carrier/header observation exceeds parser limit"))
		e.Status, e.Completeness = "limited", "limited"
		e.ExpertCode, e.Error = string(ErrResourceExceeded), e.sessionError.Error()
		e.Summary = "ESP · observation omitted by resource limit"
		a.limited.Add(uint64(len(wire)))
		a.emit(e)
		return true
	}
	fields, err := stream_parser.DecodeESPHeaderWire(wire, limit)
	if len(wire) < 8 || ci.CaptureLength < ci.Length || p.Metadata().Truncated {
		err = protocolError(ErrNeedMore, "ESP capture is incomplete; only captured clear-header facts are available")
	} else {
		err = observedWireError(err)
	}
	e.Raw = append([]byte(nil), wire...)
	e.Session, e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields), cloneSession(fields)
	e.Summary = "ESP · clear header · opaque body; SA context unavailable"
	if len(wire) >= 8 {
		e.Summary = fmt.Sprintf("ESP · SPI=0x%08x · seq=%d · opaque=%d bytes; SA context unavailable", binary.BigEndian.Uint32(wire[:4]), binary.BigEndian.Uint32(wire[4:8]), len(wire)-8)
	}
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.ExpertCode, e.Error = string(e.sessionError.Kind), err.Error()
		e.Completeness = e.Status
		if e.Status == "incomplete" {
			a.incomplete.Add(1)
		} else {
			a.malformed.Add(1)
		}
	} else {
		a.limited.Add(uint64(len(wire)))
	}
	a.emit(e)
	return true
}

func espParentLengthBounded(network gopacket.NetworkLayer, truncated bool) bool {
	var declared, captured int
	switch ip := network.(type) {
	case *layers.IPv4:
		if len(ip.Contents) < 20 || ip.Version != 4 || ip.IHL < 5 || len(ip.Contents) != int(ip.IHL)*4 {
			return false
		}
		declared = int(binary.BigEndian.Uint16(ip.Contents[2:4]))
		captured = len(ip.Contents) + len(ip.Payload)
		if declared < len(ip.Contents) {
			return false
		}
	case *layers.IPv6:
		if len(ip.Contents) < 40 || ip.Version != 6 {
			return false
		}
		declared = 40 + int(binary.BigEndian.Uint16(ip.Contents[4:6]))
		captured = 40 + len(ip.Payload)
		if ip.HopByHop != nil {
			captured += ip.HopByHop.ActualLength
		}
		if declared == 40 {
			return false
		}
	default:
		return false
	}
	return captured == declared || truncated && captured < declared
}

func espDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, item := range []struct{ name, key string }{{"esp.spi", "SPI"}, {"esp.sequence", "Sequence"}} {
		key := item.key
		b := displayCustom("esp", item.name, key+" (observed clear header)", DisplayUnsigned, 32, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if e.nativeESPObservation {
				v, ok := e.semanticSession[key].(uint32)
				offset := 0
				if key == "Sequence" {
					offset = 4
				}
				if ok && len(e.Raw) >= offset+4 && binary.BigEndian.Uint32(e.Raw[offset:offset+4]) == v {
					emit(v)
				}
			}
		})
		b.rangeKey = key
		b.descriptor.Description = "RFC4303 clear-header observation; no SA identity, ESN reconstruction, authentication or replay verdict"
		out = append(out, b)
	}
	return out
}
