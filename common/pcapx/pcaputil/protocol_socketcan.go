package pcaputil

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// LINKTYPE_CAN_SOCKETCAN carries a network-order identifier, independently of
// pcap container byte order. This local adapter preserves the complete record
// for the bounded native codec without registering global gopacket layer types.
type socketCANLayer struct{ wire []byte }

func (l *socketCANLayer) LayerType() gopacket.LayerType { return gopacket.LayerTypePayload }
func (l *socketCANLayer) LayerContents() []byte         { return l.wire }
func (l *socketCANLayer) LayerPayload() []byte          { return nil }

type socketCANDecoder struct{}

func (socketCANDecoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	b.AddLayer(&socketCANLayer{wire: w})
	return nil
}

func (c *CaptureConfig) packetHandlerWithLink(ctx context.Context, packet gopacket.Packet, link layers.LinkType) {
	// Empty capture records have no gopacket layers. The known carrier still
	// permits a precise missing-header diagnostic; original observers receive
	// their unchanged packet through the regular handler below.
	if c.binParser != nil && link == 227 && packet != nil && len(packet.Data()) == 0 {
		c.binParser.decodeCANRecord(packet.Data(), packetEvidence(packet), packet.Metadata().CaptureInfo)
	}
	c.packetHandler(ctx, packet)
}

// CAN IDs name arbitration messages, not authenticated peers or transactions.
// FD supports the canonical 72-byte Linux record; classical records may omit
// unused data padding. Legacy classical header padding stays opaque.
func decodeSocketCAN(w []byte) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrMalformedMessage, "SocketCAN "+why)
	}
	unsupported := func(why string) (map[string]any, error) {
		return nil, protocolError(ErrUnsupportedFeature, "SocketCAN "+why)
	}
	if len(w) < 8 {
		return bad("header is incomplete")
	}
	if w[4]&0x80 != 0 {
		return unsupported("CAN XL record")
	}
	fd := len(w) == 72
	if !fd && len(w) > 16 {
		return unsupported("record size outside classical/canonical FD profile")
	}
	if !fd && w[5]&4 != 0 && w[5]&^byte(7) == 0 && w[6] == 0 && w[7] == 0 {
		return unsupported("short dual-use FD record requires canonical FD capture")
	}
	id, n := binary.BigEndian.Uint32(w), int(w[4])
	extended, rtr, notification := id&0x80000000 != 0, id&0x40000000 != 0, id&0x20000000 != 0
	identifier := id & 0x1fffffff
	if notification && (extended || rtr) {
		return bad("controller error cannot carry EFF/RTR")
	}
	if !notification && !extended && identifier > 0x7ff {
		return bad("standard identifier exceeds 11 bits")
	}
	if fd {
		if rtr || notification || n > 64 {
			return bad("FD flags or payload length is invalid")
		}
		if w[5]&^byte(7) != 0 {
			return unsupported("unknown FD flag bits")
		}
	} else if n > 8 || notification && n != 8 {
		return bad("classical/controller-error payload length is invalid")
	}
	if !rtr && n > len(w)-8 {
		return bad("payload length exceeds captured record")
	}
	data, padding := w[8:8], w[8:]
	if !rtr {
		data, padding = w[8:8+n], w[8+n:]
	}
	format := "classic"
	if fd {
		format = "fd"
	}
	f := map[string]any{"Observation": "captured-controller-record", "Frame Format": format, "Identifier": identifier, "Extended": extended, "RTR": rtr, "Error Frame": notification, "Data Length": n, "Header Reserved": bytes.Clone(w[5:8]), "Data": bytes.Clone(data), "Padding": bytes.Clone(padding)}
	if fd {
		f["FD Flags"] = map[string]any{"BRS": w[5]&1 != 0, "ESI": w[5]&2 != 0, "FDF": w[5]&4 != 0}
	} else if n == 8 && w[7] >= 9 && w[7] <= 15 {
		f["Raw DLC"] = w[7]
	}
	if notification {
		e := map[string]any{"Class Mask": identifier, "Unknown Class Bits": identifier & ^uint32(1023), "TX Timeout": id&1 != 0, "Lost Arbitration": id&2 != 0, "Controller Problem": id&4 != 0, "Protocol Violation": id&8 != 0, "Transceiver Status": id&16 != 0, "No ACK": id&32 != 0, "Bus Off": id&64 != 0, "Bus Error": id&128 != 0, "Restarted": id&256 != 0, "Counters Present": id&512 != 0}
		if id&2 != 0 {
			e["Arbitration Bit"] = data[0]
		}
		if id&4 != 0 {
			e["Controller Flags"] = data[1]
		}
		if id&8 != 0 {
			e["Protocol Flags"], e["Protocol Location"] = data[2], data[3]
		}
		if id&16 != 0 {
			e["Transceiver Detail"] = data[4]
		}
		if id&512 != 0 {
			e["TX Counter"], e["RX Counter"] = data[6], data[7]
		}
		f["Controller Error"] = e
	}
	return f, nil
}

func decodeJ1939RequestName(identifier uint32, data []byte) (map[string]any, error) {
	pf, ps := byte(identifier>>16), byte(identifier>>8)
	pgn := identifier >> 8 & 0x3ffff
	f := map[string]any{"Priority": identifier >> 26 & 7, "Extended Data Page": identifier >> 25 & 1, "Data Page": identifier >> 24 & 1, "PDU Format": pf, "PDU Specific": ps, "Source Address": byte(identifier), "Content": "opaque-parameter-group"}
	if pf < 240 {
		pgn &= 0x3ff00
		f["Destination Address"] = ps
	} else {
		f["Group Extension"] = ps
	}
	f["PGN"] = pgn
	switch pgn {
	case 59904:
		if len(data) < 3 {
			return nil, protocolError(ErrMalformedMessage, "J1939 Request PGN is incomplete")
		}
		requested := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
		if requested > 0x3ffff || byte(requested>>8) < 240 && byte(requested) != 0 {
			return nil, protocolError(ErrMalformedMessage, "J1939 requested PGN is not canonical")
		}
		f["Content"], f["Requested PGN"], f["Request Trailer"] = "parameter-group-request", requested, bytes.Clone(data[3:])
	case 60928:
		if len(data) != 8 {
			return nil, protocolError(ErrMalformedMessage, "J1939 Address Claim NAME must be eight octets")
		}
		name := binary.LittleEndian.Uint64(data)
		if name == 0 {
			return nil, protocolError(ErrMalformedMessage, "J1939 Address Claim NAME is zero")
		}
		f["Content"], f["NAME"] = "unverified-address-claim", fmt.Sprintf("%016x", name)
		f["NAME Fields"] = map[string]any{"Identity Number": name & 0x1fffff, "Manufacturer Code": name >> 21 & 2047, "ECU Instance": name >> 32 & 7, "Function Instance": name >> 35 & 31, "Function": name >> 40 & 255, "Reserved": name >> 48 & 1, "Vehicle System": name >> 49 & 127, "Vehicle System Instance": name >> 56 & 15, "Industry Group": name >> 60 & 7, "Arbitrary Address Capable": name >> 63 & 1}
	}
	return f, nil
}

func (a *binParser) decodeCANRecord(w []byte, evidence captureEvidence, ci gopacket.CaptureInfo) {
	e := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "can", Protocol: "socketcan", Profile: "socketcan-controller-record", Admission: "capture-linktype-and-validated-record", Domain: evidence.Ref.Domain, Length: len(w), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{evidence.Ref}}, Completeness: "message"}
	var fields map[string]any
	var err error
	switch {
	case ci.CaptureLength < ci.Length:
		err = protocolError(ErrNeedMore, "SocketCAN capture record is truncated")
	case len(w) > min(a.budget.MaxFrameBytes, a.budget.MaxMessageBytes):
		err = protocolError(ErrResourceExceeded, "SocketCAN record exceeds parser byte budget")
	default:
		e.Raw = bytes.Clone(w)
		a.messages.Add(1)
		a.messageBytes.Add(uint64(len(w)))
		fields, err = decodeSocketCAN(w)
		selection := a.canDecodeAs[e.Domain.Interface]
		if err == nil && (selection == "j1939" || selection == "dronecan") && fields["Extended"] == true && fields["RTR"] == false && fields["Error Frame"] == false && fields["Frame Format"] == "classic" {
			e.Protocol, e.Profile, e.Admission = "j1939", "j1939-classic-request-name", "explicit-interface-decode-as"
			var jf map[string]any
			if a.canDecodeAs[e.Domain.Interface] == "dronecan" {
				e.Protocol, e.Profile = "dronecan", "dronecan-v0-node-status"
				jf, err = decodeDroneCANNodeStatus(fields["Identifier"].(uint32), fields["Data"].([]byte))
			} else {
				jf, err = decodeJ1939RequestName(fields["Identifier"].(uint32), fields["Data"].([]byte))
			}
			if err == nil {
				key := "J1939"
				if e.Protocol == "dronecan" {
					key = "DroneCAN"
				}
				fields[key] = jf
			}
		}
	}
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error, e.Completeness = err.Error(), e.Status
		switch e.Status {
		case "limited":
			a.limited.Add(uint64(len(w)))
		case "incomplete":
			a.incomplete.Add(1)
		case "context-required":
			a.contextRequired.Add(1)
		default:
			a.malformed.Add(1)
		}
		a.emit(e)
		return
	}
	e.Session, e.semanticFields = fields, cloneSession(fields)
	e.Structured = map[string]any{"fields": cloneSession(fields)}
	e.Status = "decoded"
	if a.config.Deferred {
		e.Status = "deferred"
		a.deferred.Add(1)
	} else {
		a.decoded.Add(1)
	}
	a.emit(e)
}
