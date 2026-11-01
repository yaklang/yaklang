package stream_parser

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// LDPWireLength frames one complete PDU, never a guessed message boundary after
// missing TCP bytes. Length excludes the first four octets (RFC5036 section 3.1).
func LDPWireLength(w []byte) (int, error) {
	if len(w) > 0 && w[0] != 0 || len(w) > 1 && w[1] != 1 {
		return 0, fmt.Errorf("ldp: unsupported PDU version")
	}
	if len(w) < 4 {
		return 0, nil
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 14 {
		return 0, fmt.Errorf("ldp: PDU cannot contain a complete message")
	}
	return n + 4, nil
}

func LDPMessageName(typ uint64) string {
	switch typ {
	case 1:
		return "Notification"
	case 0x100:
		return "Hello"
	case 0x200:
		return "Initialization"
	case 0x201:
		return "KeepAlive"
	case 0x300:
		return "Address"
	case 0x301:
		return "Address Withdraw"
	case 0x400:
		return "Label Mapping"
	case 0x401:
		return "Label Request"
	case 0x402:
		return "Label Withdraw"
	case 0x403:
		return "Label Release"
	case 0x404:
		return "Label Abort Request"
	}
	return "Unknown"
}

func ldpCharge(r *controlReader, remaining *int) bool {
	if *remaining <= 0 {
		r.err = ErrObservedWireBudget
		return false
	}
	*remaining--
	return true
}

// DecodeLDPSessionPDU is the shared complete-wire TCP profile. It reports
// advertisements, not an installed label database or authenticated adjacency.
// Negotiated limits and observed TCP generations belong to the native receiver.
func DecodeLDPSessionPDU(w []byte, limit int) (map[string]any, error) {
	n, err := LDPWireLength(w)
	if err != nil || n == 0 || n != len(w) {
		return nil, fmt.Errorf("ldp: complete version1 PDU required: %v", err)
	}
	if limit <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{}}
	r.num("Version", 2, false)
	r.num("PDU Length", 2, false)
	routingAddress(r, "LSR ID", 4)
	r.num("Label Space ID", 2, false)
	var messages []map[string]any
	for r.err == nil && r.at < len(w) {
		if !ldpCharge(r, &limit) {
			break
		}
		m := &controlReader{w: w, at: r.at, info: map[string]any{}}
		header := m.num("Message Header", 2, false)
		typ := header & 0x7fff
		length := m.num("Message Length", 2, false)
		m.need(length >= 4 && length <= uint64(len(w)-m.at), "LDP message crosses PDU boundary")
		if m.err != nil {
			return nil, m.err
		}
		end := m.at + int(length)
		m.w = w[:end]
		m.num("Message ID", 4, false)
		m.info["Message Type"], m.info["Unknown Message"], m.info["Packet Name"] = typ, header&0x8000 != 0, LDPMessageName(typ)
		m.need(typ != 0x100, "Hello is a UDP discovery message, not a TCP session message")
		if LDPMessageName(typ) == "Unknown" {
			m.rest("Unknown Message Value")
			m.info["Message Decoded"] = false
		} else {
			m.need(header&0x8000 == 0, "known base message has unexpected U bit")
			var tlvs []map[string]any
			for m.err == nil && m.at < end {
				if !ldpCharge(m, &limit) {
					break
				}
				start := m.at
				t := &controlReader{w: w[:end], at: start, info: map[string]any{}}
				flags := t.num("TLV Header", 2, false)
				tt := flags & 0x3fff
				size := t.num("TLV Length", 2, false)
				t.need(size <= uint64(len(t.w)-t.at), "LDP TLV crosses message boundary")
				if t.err != nil {
					return nil, t.err
				}
				valueEnd := t.at + int(size)
				t.w = w[:valueEnd]
				t.info["TLV Type"], t.info["Unknown Ignore"], t.info["Unknown Forward"] = tt, flags&0x8000 != 0, flags&0x4000 != 0
				decodeLDPSessionTLV(t, tt, typ, &limit)
				t.need(t.at == valueEnd, "LDP TLV trailing bytes")
				if t.err != nil {
					return nil, t.err
				}
				ranges := routingFieldRanges(t.fields)
				ranges["TLV Type"] = []any{uint64(start), uint64(start + 2)}
				if tt == 0x200 {
					ranges["Generic Label"], ranges["Label Reserved"] = ranges["Label Word"], ranges["Label Word"]
				}
				if tt == 0x300 {
					for _, key := range []string{"Status Code", "Fatal", "Forward Status"} {
						ranges[key] = ranges["Status Word"]
					}
				}
				if tt == 0x500 {
					for _, key := range []string{"On Demand Advertisement", "Loop Detection", "Session Reserved"} {
						ranges[key] = ranges["Session Flags"]
					}
				}
				t.info["Wire Field Ranges"] = ranges
				if t.info["Value Decoded"] == false && flags&0x8000 == 0 {
					m.info["Unknown Mandatory Parameters"] = true
				}
				if t.info["FEC Decoded"] == false || t.info["Addresses Decoded"] == false {
					m.info["Unsupported Parameter Value"] = true
				}
				tlvs = append(tlvs, t.info)
				m.at = valueEnd
			}
			if m.err != nil {
				return nil, m.err
			}
			m.info["TLVs"] = tlvs
			if err := validateLDPMandatory(typ, tlvs); err != nil {
				return nil, err
			}
		}
		if m.err != nil {
			return nil, m.err
		}
		ranges := routingFieldRanges(m.fields)
		ranges["Message Type"] = []any{uint64(r.at), uint64(r.at + 2)}
		ranges["Unknown Message"] = []any{uint64(r.at), uint64(r.at + 2)}
		m.info["Wire Field Ranges"] = ranges
		messages = append(messages, m.info)
		r.at = end
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["Messages"], r.info["Wire Field Ranges"] = messages, routingFieldRanges(r.fields)
	r.info["Authentication Verified"] = false
	r.info["Observation Scope"] = "complete TCP PDU; advertisements and observed exchange only; no installed label database or authenticated peer claim"
	return r.info, nil
}

func validateLDPMandatory(typ uint64, tlvs []map[string]any) error {
	mandatory := []uint64{}
	switch typ {
	case 1:
		mandatory = []uint64{0x300}
	case 0x200:
		mandatory = []uint64{0x500}
	case 0x300, 0x301:
		mandatory = []uint64{0x101}
	case 0x400:
		mandatory = []uint64{0x100, 0x200}
	case 0x401, 0x402, 0x403:
		mandatory = []uint64{0x100}
	case 0x404:
		mandatory = []uint64{0x100, 0x600}
	}
	for i, want := range mandatory {
		if i >= len(tlvs) {
			return fmt.Errorf("ldp: missing mandatory %04x in %s", want, LDPMessageName(typ))
		}
		got := tlvs[i]["TLV Type"].(uint64)
		if want == 0x200 && got >= 0x200 && got <= 0x202 {
			continue
		}
		if got != want {
			return fmt.Errorf("ldp: mandatory TLV order in %s", LDPMessageName(typ))
		}
	}
	seen := map[uint64]bool{}
	for _, t := range tlvs {
		tt := t["TLV Type"].(uint64)
		if tt == 0x100 || tt == 0x101 || tt == 0x500 || tt == 0x600 || tt >= 0x200 && tt <= 0x202 {
			key := tt
			if tt >= 0x200 && tt <= 0x202 {
				key = 0x200
			}
			if seen[key] {
				return fmt.Errorf("ldp: duplicate singleton TLV %04x", tt)
			}
			seen[key] = true
		}
	}
	return nil
}

func decodeLDPSessionTLV(r *controlReader, typ, msg uint64, remaining *int) {
	n := len(r.w) - r.at
	exact := func(want int) { r.need(n == want, "LDP known TLV has incorrect length") }
	switch typ {
	case 0x100:
		r.info["Packet Name"] = "FEC"
		decodeLDPFEC(r, msg, remaining)
	case 0x101:
		r.info["Packet Name"] = "Address List"
		afi := r.num("Address Family", 2, false)
		size := 4
		if afi == 2 {
			size = 16
		} else if afi != 1 {
			r.rest("Opaque Addresses")
			r.info["Addresses Decoded"] = false
			return
		}
		r.need((len(r.w)-r.at)%size == 0, "LDP address list width")
		var rows []map[string]any
		for r.err == nil && r.at < len(r.w) {
			if !ldpCharge(r, remaining) {
				break
			}
			start := r.at
			row := &controlReader{w: r.w, at: start, info: map[string]any{}}
			routingAddress(row, "Address", size)
			row.info["Wire Field Ranges"] = routingFieldRanges(row.fields)
			rows = append(rows, row.info)
			r.at = row.at
			if row.err != nil {
				r.err = row.err
			}
		}
		r.info["Addresses"] = rows
	case 0x103:
		exact(1)
		r.num("Hop Count", 1, false)
		r.info["Packet Name"] = "Hop Count"
	case 0x104:
		r.need(n%4 == 0, "LDP path vector width")
		var rows []map[string]any
		for r.err == nil && r.at < len(r.w) {
			if !ldpCharge(r, remaining) {
				break
			}
			row := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
			routingAddress(row, "LSR ID", 4)
			row.info["Wire Field Ranges"] = routingFieldRanges(row.fields)
			rows = append(rows, row.info)
			r.at = row.at
		}
		r.info["Path Vector"] = rows
		r.info["Packet Name"] = "Path Vector"
	case 0x200:
		exact(4)
		label := r.num("Label Word", 4, false)
		r.info["Generic Label"] = label & 0xfffff
		r.info["Label Reserved"] = label >> 20
		r.info["Packet Name"] = "Generic Label"
	case 0x201, 0x202:
		exact(4)
		r.rest("Opaque Link Label")
		r.info["Value Decoded"] = false
		r.info["Packet Name"] = "ATM/Frame Relay Label (unselected)"
	case 0x300:
		exact(10)
		code := r.num("Status Word", 4, false)
		r.info["Status Code"], r.info["Fatal"], r.info["Forward Status"] = code&0x3fffffff, code&0x80000000 != 0, code&0x40000000 != 0
		r.num("Referenced Message ID", 4, false)
		r.num("Referenced Message Type", 2, false)
		r.info["Packet Name"] = "Status"
	case 0x301:
		exact(4)
		r.num("Extended Status", 4, false)
		r.info["Packet Name"] = "Extended Status"
	case 0x302, 0x303:
		r.rest("Returned Wire Bytes")
		r.info["Packet Name"] = "Returned PDU/Message (quoted bytes)"
	case 0x500:
		exact(14)
		v := r.num("Session Version", 2, false)
		keep := r.num("KeepAlive Time", 2, false)
		flags := r.num("Session Flags", 1, false)
		pv := r.num("Path Vector Limit", 1, false)
		r.num("Max PDU Length", 2, false)
		routingAddress(r, "Receiver LSR ID", 4)
		r.num("Receiver Label Space ID", 2, false)
		r.info["On Demand Advertisement"], r.info["Loop Detection"], r.info["Session Reserved"] = flags&0x80 != 0, flags&0x40 != 0, flags&0x3f
		r.need(v == 1 && keep != 0, "LDP session version or zero KeepAlive Time")
		r.need(flags&0x40 != 0 || pv == 0, "LDP path vector limit without loop detection")
		r.info["Packet Name"] = "Common Session Parameters"
	case 0x600:
		exact(4)
		r.num("Label Request Message ID", 4, false)
		r.info["Packet Name"] = "Label Request Message ID"
	default:
		r.rest("Unknown Value")
		r.info["Value Decoded"] = false
		r.info["Packet Name"] = fmt.Sprintf("Unknown TLV 0x%04x", typ)
	}
}

func decodeLDPFEC(r *controlReader, msg uint64, remaining *int) {
	var rows []map[string]any
	for r.err == nil && r.at < len(r.w) {
		if !ldpCharge(r, remaining) {
			break
		}
		f := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		typ := f.num("FEC Type", 1, false)
		if typ == 1 {
			f.need((msg == 0x402 || msg == 0x403) && len(rows) == 0 && f.at == len(f.w), "LDP wildcard FEC scope")
			f.info["Packet Name"] = "Wildcard"
		} else if typ == 2 {
			afi := f.num("Address Family", 2, false)
			bits := f.num("Prefix Length", 1, false)
			size := 4
			if afi == 2 {
				size = 16
			} else if afi != 1 {
				f.rest("Opaque FEC Elements")
				r.info["FEC Decoded"] = false
			}
			if afi == 1 || afi == 2 {
				f.need(bits <= uint64(size*8), "LDP FEC prefix length")
				wire := f.take("Prefix Bytes", int((bits+7)/8))
				if f.err == nil {
					var b [16]byte
					copy(b[:], wire)
					addr := netip.AddrFrom16(b)
					if size == 4 {
						addr = netip.AddrFrom4([4]byte(b[:4]))
					}
					prefix := netip.PrefixFrom(addr, int(bits)).Masked()
					f.info["Prefix"], f.info["Prefix Address"] = prefix.String(), prefix.Addr().String()
				}
			}
			f.info["Packet Name"] = "Prefix"
		} else {
			f.rest("Opaque FEC Elements")
			r.info["FEC Decoded"] = false
		}
		if f.err != nil {
			r.err = f.err
			return
		}
		f.info["Wire Field Ranges"] = routingFieldRanges(f.fields)
		if ranges := f.info["Wire Field Ranges"].(map[string]any); ranges["Prefix Bytes"] != nil {
			ranges["Prefix"] = ranges["Prefix Bytes"]
			ranges["Prefix Address"] = ranges["Prefix Bytes"]
		}
		rows = append(rows, f.info)
		r.at = f.at
	}
	r.need(len(rows) > 0, "empty FEC TLV")
	r.need(msg == 0x400 || len(rows) == 1, "multiple FEC elements outside Label Mapping")
	r.info["FEC Elements"] = rows
}
