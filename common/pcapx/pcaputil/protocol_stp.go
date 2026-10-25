package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// The 802.3 length, not the capture's padding or a previous advertisement,
// bounds the BPDU. Only a complete canonical SAP pair/UI or Cisco SNAP PID
// selects STP. An Ethernet multicast destination alone is not identity proof.
func stpEthernetCarrier(eth *layers.Ethernet) (wire []byte, pvst, selected bool, err error) {
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
	if n > 1500 || len(p) < 3 {
		return
	}
	offset := 0
	if p[0] == 0x42 && p[1] == 0x42 && p[2] == 3 {
		offset = 3
	} else if len(p) >= 8 && p[0] == 0xaa && p[1] == 0xaa && p[2] == 3 && p[3] == 0 && p[4] == 0 && p[5] == 12 && p[6] == 1 && p[7] == 11 {
		offset, pvst = 8, true
	} else {
		return
	}
	selected = true
	if n < offset {
		return nil, pvst, true, protocolError(ErrMalformedMessage, "STP carrier length excludes its LLC/SNAP header")
	}
	if n > len(p) {
		return p[offset:], pvst, true, protocolError(ErrNeedMore, "STP declared 802.3 carrier is not fully captured")
	}
	return p[offset:n], pvst, true, nil
}

func (a *binParser) observeSTP(p gopacket.Packet, evidence captureEvidence) bool {
	var eth *layers.Ethernet
	// A tunnel may carry another Ethernet frame. Associate LLC with its
	// immediately preceding Ethernet layer, never the outer tunnel's peers.
	for _, layer := range p.Layers() {
		if v, ok := layer.(*layers.Ethernet); ok {
			eth = v
		}
		if _, ok := layer.(*layers.LLC); !ok || eth == nil {
			continue
		}
		w, pvst, selected, err := stpEthernetCarrier(eth)
		if !selected {
			continue
		}
		ci := p.Metadata().CaptureInfo
		e := &ProtocolEvent{Protocol: "stp", Transport: "ethernet", Domain: evidence.Ref.Domain,
			Timestamp: ci.Timestamp, Source: eth.SrcMAC.String(), Destination: eth.DstMAC.String(),
			Rule: "stp", Entry: "STPFields", Profile: "stp-bounded-bpdu-advertisement", Admission: "observed-802.3-LLC-carrier",
			Completeness: "message", SourceBytes: networkObservationSource(evidence), nativeSessionMessage: true}
		if pvst {
			e.Entry = "PVSTFields"
			e.Admission = "observed-802.3-Cisco-SNAP-carrier"
		}
		if err == nil && ci.CaptureLength < ci.Length {
			err = protocolError(ErrNeedMore, "STP capture is truncated; complete carrier is not proved")
		}
		wireLength := len(w)
		if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
			err = protocolError(ErrResourceExceeded, "STP BPDU exceeds parser byte budget")
			w = nil
		}
		var fields map[string]any
		if err == nil {
			fields, err = stream_parser.DecodeSTPWire(w, pvst, a.budget.MaxCollectionElements)
			err = observedWireError(err)
		}
		if fields != nil {
			e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
		}
		a.finishProtocolDatagram(e, w, nil, err)
		if wireLength > 0 && len(e.Raw) == 0 && e.Status == "limited" {
			e.Length = wireLength
			a.messageBytes.Add(uint64(wireLength))
			a.limited.Add(uint64(wireLength))
		}
		if err != nil {
			e.Completeness = e.Status
		} else {
			if fields["Unselected Extensions"] == true {
				e.Completeness = "limited"
			}
			e.Admission += "-and-complete-BPDU-grammar"
			e.Summary = fmt.Sprintf("%s %s", fields["Protocol Name"], fields["Packet Name"])
			if root, ok := fields["Root MAC"]; ok {
				e.Summary += fmt.Sprintf(" · advertised root %v/%v/%v · cost=%v port=0x%04x", fields["Root Priority"], fields["Root System ID Extension"], root, fields["Root Path Cost"], routingNumber(fields, "Port ID"))
			}
		}
		a.emit(e)
		return true
	}
	return false
}
