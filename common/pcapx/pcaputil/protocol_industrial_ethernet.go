package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// The profiles expose captured SV octets and EtherCAT datagrams. Neither
// dataset channels without SCL nor device operations from WKC are inferred.
func industrialLinkError(kind ProtocolErrorKind, why string) error {
	return protocolError(kind, "industrial Ethernet: %s", why)
}

func svUint(w []byte, maximum uint64) (uint64, error) {
	if len(w) < 1 || len(w) > 5 || berUint(w) > maximum {
		return 0, industrialLinkError(ErrMalformedMessage, "SV integer is outside its field range")
	}
	// Fixed unsigned publisher encodings, including ffff/ffffffff, are used by
	// libiec61850 1.5.1. Do not apply signed BER interpretation to those widths.
	return berUint(w), nil
}

func svTLV(w []byte, at int) (byte, []byte, int, error) {
	tag, value, next, err := berRead(w, at)
	if err != nil {
		return 0, nil, at, industrialLinkError(ErrMalformedMessage, "SV TLV exceeds its complete bounded container")
	}
	return tag, value, next, nil
}

func decodeSV(w []byte, budget ParserBudget) (map[string]any, error) {
	if budget.MaxRecursionDepth < 4 {
		return nil, industrialLinkError(ErrResourceExceeded, "SV nesting exceeds depth budget")
	}
	tag, pdu, next, err := svTLV(w, 8)
	if err != nil {
		return nil, err
	}
	if tag != 0x60 || next != len(w) {
		return nil, industrialLinkError(ErrMalformedMessage, "SV requires one exact savPdu")
	}
	tag, value, at, err := svTLV(pdu, 0)
	if err != nil {
		return nil, err
	}
	if tag != 0x80 {
		return nil, industrialLinkError(ErrMalformedMessage, "SV noASDU is missing")
	}
	count, err := svUint(value, 65535)
	if err != nil {
		return nil, err
	}
	if count > uint64(budget.MaxCollectionElements) {
		return nil, industrialLinkError(ErrResourceExceeded, "SV ASDU count exceeds collection budget")
	}
	tag, seq, end, err := svTLV(pdu, at)
	if err != nil {
		return nil, err
	}
	if tag != 0xa2 || end != len(pdu) {
		return nil, industrialLinkError(ErrMalformedMessage, "SV requires one exact seqASDU")
	}
	if count > uint64(len(seq)/2) {
		return nil, industrialLinkError(ErrMalformedMessage, "SV count exceeds the available ASDU envelopes")
	}
	asdus := make([]map[string]any, 0, int(count))
	pos := 0
	for i := uint64(0); i < count; i++ {
		tag, fields, next, err := svTLV(seq, pos)
		if err != nil {
			return nil, err
		}
		if tag != 0x30 {
			return nil, industrialLinkError(ErrMalformedMessage, "SV ASDU SEQUENCE is missing")
		}
		pos = next
		a := make(map[string]any)
		previous, seen := byte(0x7f), uint16(0)
		for at := 0; at < len(fields); {
			tag, value, next, err := svTLV(fields, at)
			if err != nil {
				return nil, err
			}
			at = next
			if tag <= previous {
				return nil, industrialLinkError(ErrMalformedMessage, "SV fields are duplicated or unordered")
			}
			if tag > 0x89 {
				return nil, industrialLinkError(ErrUnsupportedFeature, "SV ASDU extension is outside the selected profile")
			}
			previous = tag
			seen |= 1 << (tag - 0x80)
			name := [...]string{"SV ID", "Dataset", "Sample Counter", "Configuration Revision", "Reference Time", "Sample Synchronization", "Sample Rate", "Sample Data", "Sample Mode", "Grandmaster Identity"}[tag-0x80]
			switch tag {
			case 0x82, 0x83, 0x85, 0x86, 0x88:
				maximum := uint64(0xffffffff)
				if tag == 0x82 || tag == 0x86 {
					maximum = 65535
				}
				n, err := svUint(value, maximum)
				if err != nil {
					return nil, err
				}
				a[name] = n
			default:
				if (tag == 0x84 || tag == 0x89) && len(value) != 8 {
					return nil, industrialLinkError(ErrMalformedMessage, "SV time/identity octets must have length eight")
				}
				if tag == 0x80 || tag == 0x81 {
					for _, c := range value {
						if c < 0x20 || c > 0x7e {
							return nil, industrialLinkError(ErrMalformedMessage, "SV VisibleString contains a non-visible octet")
						}
					}
				}
				a[name] = bytes.Clone(value)
			}
		}
		// smpSynch is OPTIONAL in the fixed ASN; unknown named integer values
		// are observations rather than an invented enumeration constraint.
		if seen&141 != 141 {
			return nil, industrialLinkError(ErrMalformedMessage, "SV ASDU lacks a mandatory field")
		}
		asdus = append(asdus, a)
	}
	if pos != len(seq) {
		return nil, industrialLinkError(ErrMalformedMessage, "SV noASDU does not consume seqASDU")
	}
	r1, r2 := binary.BigEndian.Uint16(w[4:]), binary.BigEndian.Uint16(w[6:])
	return map[string]any{"APPID": binary.BigEndian.Uint16(w), "Length": len(w), "Reserved 1": r1, "Reserved 2": r2, "Simulation": r1&0x8000 != 0, "ASDU Count": count, "ASDUs": asdus}, nil
}

func decodeEtherCAT(w []byte, budget ParserBudget) (map[string]any, error) {
	if budget.MaxRecursionDepth < 2 {
		return nil, industrialLinkError(ErrResourceExceeded, "EtherCAT nesting exceeds depth budget")
	}
	header := binary.LittleEndian.Uint16(w)
	if header&0x0800 != 0 {
		return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT frame reserved bit is nonzero")
	}
	if header>>12 != 1 {
		return nil, industrialLinkError(ErrUnsupportedFeature, "EtherCAT frame type is outside datagram profile")
	}
	if len(w) < 14 {
		return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT datagram area is too short")
	}
	ds := make([]map[string]any, 0)
	for at := 2; at < len(w); {
		if len(ds) >= budget.MaxCollectionElements {
			return nil, industrialLinkError(ErrResourceExceeded, "EtherCAT datagram count exceeds collection budget")
		}
		if len(w)-at < 12 {
			return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT datagram header/counter is truncated")
		}
		b := w[at:]
		command, index := b[0], b[1]
		flags, irq := binary.LittleEndian.Uint16(b[6:]), binary.LittleEndian.Uint16(b[8:])
		length := int(flags & 0x07ff)
		if command > 14 {
			return nil, industrialLinkError(ErrUnsupportedFeature, "EtherCAT reserved/vendor command is outside the selected profile")
		}
		if flags&0x3800 != 0 {
			return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT datagram reserved bits are nonzero")
		}
		if length > 1486 || length+12 > len(b) {
			return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT datagram length exceeds its permitted boundary")
		}
		next := at + 12 + length
		more := flags&0x8000 != 0
		if more != (next < len(w)) {
			return nil, industrialLinkError(ErrMalformedMessage, "EtherCAT continuation flag disagrees with complete chain")
		}
		d := map[string]any{"Command": command, "Index": index, "Length": length, "Length Flags": flags, "Circulating": flags&0x4000 != 0, "More": more, "Interrupt": irq, "Data": bytes.Clone(b[10 : 10+length]), "Working Counter": binary.LittleEndian.Uint16(b[10+length:])}
		if command >= 10 && command <= 12 {
			d["Logical Address"] = binary.LittleEndian.Uint32(b[2:])
		} else {
			d["Station Address"], d["Register Offset"] = binary.LittleEndian.Uint16(b[2:]), binary.LittleEndian.Uint16(b[4:])
		}
		ds = append(ds, d)
		at = next
	}
	return map[string]any{"Type": 1, "Length": len(w) - 2, "Datagrams": ds}, nil
}

func (a *binParser) decodeIndustrialEthernet(eth *layers.Ethernet, kind layers.EthernetType, payload []byte, evidence captureEvidence, ci gopacket.CaptureInfo) {
	protocol, profile, rule, entry := "sv", "iec61850-sv-raw-asdu", "iec61850", "SampledValues"
	if kind == 0x88a4 {
		protocol, profile, rule, entry = "ethercat", "ethercat-datagram-chain", "ethercat", "EtherCAT"
	}
	e := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "ethernet", Protocol: protocol, Profile: profile, Admission: "ether-type", Source: eth.SrcMAC.String(), Destination: eth.DstMAC.String(), Domain: evidence.Ref.Domain, Rule: rule, Entry: entry, SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{evidence.Ref}}}
	var err error
	n, minimum := 0, 2
	if protocol == "sv" {
		minimum = 8
	}
	if len(payload) < minimum {
		err = industrialLinkError(ErrNeedMore, "link header is truncated")
	} else if protocol == "sv" {
		n = int(binary.BigEndian.Uint16(payload[2:]))
		if n < 10 {
			err = industrialLinkError(ErrMalformedMessage, "SV declared frame is too short")
		}
	} else {
		n = int(binary.LittleEndian.Uint16(payload)&0x07ff) + 2
	}
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if err == nil && (n > limit || len(payload) > limit) {
		err = industrialLinkError(ErrResourceExceeded, "link PDU or captured padding exceeds message byte budget")
	}
	if err == nil && n > len(payload) {
		err = industrialLinkError(ErrNeedMore, "declared link PDU exceeds this captured carrier")
	}
	f := &binFlow{a: a}
	defer f.closeSession()
	// Reserve before field maps, raw bytes, padding and event projections. No
	// session or capture-owned input survives this stateless frame decoder.
	if len(payload) <= limit && n <= limit {
		if reserveErr := f.reserveSession(512 + 32*int64(len(payload))); reserveErr != nil {
			err = reserveErr
		}
	}
	var wire []byte
	if err == nil {
		wire = payload[:n]
		if protocol == "sv" {
			e.Session, err = decodeSV(wire, a.budget)
		} else {
			e.Session, err = decodeEtherCAT(wire, a.budget)
		}
		if err == nil {
			e.Session["Link Padding"] = bytes.Clone(payload[n:])
			e.semanticFields = cloneSession(e.Session)
		}
	} else if f.sessionBytes > 0 {
		wire = payload
	}
	e.Length = n
	a.finishProtocolDatagram(e, wire, nil, err)
	if err != nil {
		e.Completeness = e.Status
	} else {
		e.Completeness = "message"
	}
	a.emit(e)
}
