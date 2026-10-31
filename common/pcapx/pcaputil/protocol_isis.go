package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func isisEthernetCarrier(eth *layers.Ethernet) (wire []byte, selected bool, err error) {
	if len(eth.Contents) < 14 {
		return
	}
	n := int(binary.BigEndian.Uint16(eth.Contents[12:14]))
	p := eth.Payload
	for depth := 0; n == 0x8100 || n == 0x88a8; depth++ {
		if depth >= 4 || len(p) < 4 {
			return
		}
		n, p = int(binary.BigEndian.Uint16(p[2:4])), p[4:]
	}
	if n > 1500 || len(p) < 4 || p[0] != 0xfe || p[1] != 0xfe || p[2] != 3 && p[2] != 0x13 || p[3] != 0x83 {
		return
	}
	selected = true
	if n < 4 {
		return nil, true, protocolError(ErrMalformedMessage, "IS-IS carrier excludes its complete LLC/NLPID")
	}
	if n > len(p) {
		return p[3:], true, protocolError(ErrNeedMore, "IS-IS declared IEEE802.3 carrier is not fully captured")
	}
	return p[3:n], true, nil
}

func (a *binParser) observeISIS(p gopacket.Packet, evidence captureEvidence) bool {
	for _, layer := range p.Layers() {
		if _, ok := layer.(gopacket.NetworkLayer); ok {
			return false
		}
		eth, ok := layer.(*layers.Ethernet)
		if !ok {
			continue
		}
		wire, selected, err := isisEthernetCarrier(eth)
		if !selected {
			return false // first Ethernet is the root; an inner pseudowire needs its own admission proof
		}
		ci := p.Metadata().CaptureInfo
		e := &ProtocolEvent{Protocol: "isis", Transport: "ethernet", Domain: evidence.Ref.Domain,
			Timestamp: ci.Timestamp, Source: eth.SrcMAC.String(), Destination: eth.DstMAC.String(),
			Rule: "isis", Entry: "ISISFields", Profile: "isis-six-octet-independent-advertisement", Admission: "observed-IEEE802.3-LLC-NLPID",
			Completeness: "message", SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true}
		if err == nil && ci.CaptureLength < ci.Length {
			err = protocolError(ErrNeedMore, "IS-IS capture is truncated; complete carrier is not proved")
		}
		wireLength := len(wire)
		if len(wire) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
			err = protocolError(ErrResourceExceeded, "IS-IS PDU exceeds parser byte budget")
			wire = nil
		}
		var fields map[string]any
		if err == nil {
			fields, err = stream_parser.DecodeISISWire(wire, a.budget.MaxCollectionElements)
			err = observedWireError(err)
		}
		if fields != nil {
			e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
		}
		a.finishProtocolDatagram(e, wire, nil, err)
		if wireLength > 0 && len(e.Raw) == 0 && e.Status == "limited" {
			e.Length = wireLength
			a.messageBytes.Add(uint64(wireLength))
			a.limited.Add(uint64(wireLength))
		}
		if err != nil || fields["Unselected Extensions"] == true {
			e.Completeness = "limited"
		}
		if fields != nil {
			e.Summary = fmt.Sprintf("IS-IS %v", fields["Packet Name"])
			if id, ok := fields["Source System ID"]; ok && fields["Holding Timer Seconds"] != nil {
				e.Summary += fmt.Sprintf(" · advertised system=%v · hold=%v s", id, fields["Holding Timer Seconds"])
			}
			if id, ok := fields["LSP ID"]; ok {
				e.Summary += fmt.Sprintf(" · LSP=%v seq=0x%08x lifetime=%v s · %v", id, routingNumber(fields, "Sequence Number"), fields["Remaining Lifetime Seconds"], fields["Checksum Observation"])
			}
			if id, ok := fields["Source Node ID"]; ok {
				e.Summary += fmt.Sprintf(" · advertised source=%v", id)
			}
			if err != nil {
				e.Summary += " · " + err.Error()
			}
		}
		a.emit(e)
		return true
	}
	return false
}
