package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// IP protocol115 supplies an explicit L2TPv3 carrier identity. UDP service
// routing alone is insufficient for an opaque v3 data header: automatic UDP
// admission requires the complete control grammar, including its first AVP.
// Fragment assembly and supported outer-tunnel normalization precede this call.
func (a *binParser) observeL2TPv3(p gopacket.Packet, evidence captureEvidence) bool {
	network := p.NetworkLayer()
	if network == nil {
		return false
	}
	overUDP, selected, explicitUDP := false, false, false
	var carrierErr error
	switch ip := network.(type) {
	case *layers.IPv4:
		selected = ip.Protocol == 115
		// gopacket replaces wire TotalLength0 with capture-rest for offload
		// tolerance. That inferred length cannot establish a bounded v3 PDU.
		if selected && (len(ip.Contents) < 20 || ip.Version != 4 || ip.IHL < 5 || int(binary.BigEndian.Uint16(ip.Contents[2:4])) < int(ip.IHL)*4 || int(binary.BigEndian.Uint16(ip.Contents[2:4])) != len(ip.Contents)+len(ip.Payload)) {
			carrierErr = protocolError(ErrMalformedMessage, "L2TPv3 requires a complete bounded IPv4 total length")
		}
	case *layers.IPv6:
		selected = ip.NextHeader == 115
		if selected && (len(ip.Contents) < 40 || ip.Version != 6 || int(binary.BigEndian.Uint16(ip.Contents[4:6])) != len(ip.Payload)) {
			carrierErr = protocolError(ErrMalformedMessage, "L2TPv3 direct IPv6 payload length mismatch")
		}
	}
	wire := network.LayerPayload()
	source, destination := network.NetworkFlow().Src().String(), network.NetworkFlow().Dst().String()
	if !selected {
		udp, ok := p.TransportLayer().(*layers.UDP)
		if !ok || len(udp.Payload) < 2 {
			return false
		}
		src, dst := uint16(udp.SrcPort), uint16(udp.DstPort)
		explicit := a.datagramDecodeAs[src] == "l2tp" || a.datagramDecodeAs[dst] == "l2tp"
		explicitUDP = explicit
		if !explicit && src != 1701 && dst != 1701 {
			return false
		}
		flags := binary.BigEndian.Uint16(udp.Payload)
		if flags&15 != 3 || !explicit && flags&0x8000 == 0 {
			return false
		}
		wire, overUDP, selected = udp.Payload, true, true
		source = net.JoinHostPort(source, strconv.Itoa(int(src)))
		destination = net.JoinHostPort(destination, strconv.Itoa(int(dst)))
		if udp.Length < 8 || int(udp.Length) != len(wire)+8 {
			return false
		}
	}
	if !selected {
		return false
	}
	ci := p.Metadata().CaptureInfo
	e := &ProtocolEvent{Protocol: "l2tp", Transport: "network", Domain: evidence.Ref.Domain, Timestamp: ci.Timestamp,
		Source: source, Destination: destination, Rule: "l2tp3", Entry: "L2TPv3IPFields", Profile: "l2tp3-bounded-wire-envelope",
		Admission: "observed-IP-protocol", Completeness: "message", SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true}
	if overUDP {
		e.Transport, e.Entry, e.Admission = "udp", "L2TPv3UDPFields", "service-hint"
		if explicitUDP {
			e.Admission = "explicit-decode-as"
		}
	}
	var m map[string]any
	var err error
	if ci.CaptureLength < ci.Length || p.Metadata().Truncated {
		err = protocolError(ErrNeedMore, "L2TPv3 carrier capture is truncated")
	} else if carrierErr != nil {
		err = carrierErr
	} else if len(wire) > min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes) {
		err = protocolError(ErrResourceExceeded, "L2TPv3 wire exceeds parser limit")
	} else {
		m, err = stream_parser.DecodeL2TPv3Wire(wire, overUDP, a.budget.MaxCollectionElements)
		err = observedWireError(err)
	}
	if m != nil {
		if m["Unparsed Parameters"] == true || m["Unparsed Message Type"] == true || m["Ambiguous Parameters"] == true {
			e.Completeness = "limited"
		}
		if m["Control"] != true {
			m["Opaque Payload Bytes"] = uint64(len(wire) - int(m["Payload Offset"].(uint64)))
			m["Connection Context"] = "not observed; cookie, sublayer and pseudowire are unconfirmed"
			e.Completeness = "limited"
		}
		e.Session = m
		e.semanticFields, e.semanticSession = cloneSession(m), cloneSession(m)
	}
	if len(wire) > min(a.config.MaxMessageBytes, a.budget.MaxFrameBytes) {
		a.finishProtocolDatagram(e, nil, nil, err)
		e.Length = len(wire)
		a.messageBytes.Add(uint64(len(wire)))
		a.limited.Add(uint64(len(wire)))
	} else {
		a.finishProtocolDatagram(e, wire, nil, err)
	}
	if err != nil {
		e.Completeness = "limited"
	} else {
		e.Length = len(wire)
		if overUDP && !explicitUDP {
			e.Admission = "complete-control-grammar-and-service-hint"
		}
		e.Summary = fmt.Sprintf("L2TPv3 %s", m["Packet Name"])
		if m["Control"] != true {
			e.Summary += fmt.Sprintf(" · session=0x%08x · receiver context unavailable; payload opaque", m["Session ID"])
		}
	}
	a.emit(e)
	return true
}
