package stream_parser

import (
	"fmt"
)

// DecodeLDPDiscovery validates a complete RFC5036 UDP discovery PDU, including
// every message and TLV envelope. A Hello is an advertisement, not an observed
// TCP session, negotiated hold time, accepted adjacency or authenticated LSR.
// Unknown extension TLVs retain their U/F bits and owned bytes, without guessing
// their meaning. TCP session and label distribution PDUs require another entry.
func DecodeLDPDiscovery(w []byte, limit int) (map[string]any, error) {
	if len(w) < 26 || len(w) > 4100 {
		return nil, fmt.Errorf("ldp discovery: complete PDU outside initial length bound")
	}
	if limit <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{}}
	version := r.num("Version", 2, false)
	length := r.num("PDU Length", 2, false)
	r.need(version == 1 && length+4 == uint64(len(w)), "LDP version or complete PDU length")
	routingAddress(r, "LSR ID", 4)
	r.num("Label Space ID", 2, false)
	var messages []map[string]any
	for r.err == nil && r.at < len(w) {
		if limit == 0 {
			return nil, ErrObservedWireBudget
		}
		limit--
		m := &controlReader{w: w, at: r.at, info: map[string]any{}}
		typ := m.num("Message Type", 2, false)
		n := m.num("Message Length", 2, false)
		m.need(typ == 0x0100 && n >= 12 && n <= uint64(len(w)-m.at), "complete Hello message required")
		if m.err != nil {
			return nil, m.err
		}
		end := m.at + int(n)
		m.w = w[:end]
		m.num("Message ID", 4, false)
		m.info["Unknown Message"] = false
		m.info["Packet Name"] = "Hello"
		var tlvs []map[string]any
		for m.err == nil && m.at < end {
			if limit == 0 {
				return nil, ErrObservedWireBudget
			}
			limit--
			t := &controlReader{w: w[:end], at: m.at, info: map[string]any{}}
			flags := t.num("TLV Header", 2, false)
			typ := flags & 0x3fff
			n := t.num("TLV Length", 2, false)
			t.need(n <= uint64(len(t.w)-t.at), "LDP TLV crosses message boundary")
			if t.err != nil {
				return nil, t.err
			}
			t.info["TLV Type"], t.info["Unknown Ignore"], t.info["Unknown Forward"] = typ, flags&0x8000 != 0, flags&0x4000 != 0
			valueEnd := t.at + int(n)
			t.w = w[:valueEnd]
			if len(tlvs) == 0 {
				t.need(typ == 0x0400 && flags == typ && n == 4, "Hello requires leading Common Hello Parameters")
			} else {
				t.need(typ != 0x0400, "duplicate Common Hello Parameters")
			}
			switch typ {
			case 0x0400:
				t.need(n == 4, "Common Hello Parameters length")
				t.num("Hold Time", 2, false)
				bits := t.num("Hello Flags", 2, false)
				t.info["Targeted"], t.info["Requested"], t.info["GTSM"] = bits&0x8000 != 0, bits&0x4000 != 0, bits&0x2000 != 0
				t.info["Reserved"] = bits & 0x1fff
				if t.info["Hold Time"] == uint64(0) {
					t.info["Hold Time Meaning"] = "advertised default: link 15 s, targeted 45 s; negotiated value unobserved"
				} else if t.info["Hold Time"] == uint64(65535) {
					t.info["Hold Time Meaning"] = "advertised infinite; negotiated value unobserved"
				}
				if bits&0xa000 == 0xa000 {
					t.info["GTSM Meaning"] = "flag ignored for targeted Hello; enforcement unobserved"
				}
				t.info["Packet Name"] = "Common Hello Parameters"
			case 0x0401:
				t.need(n == 4, "IPv4 Transport Address length")
				routingAddress(t, "IPv4 Transport Address", 4)
				t.info["Packet Name"] = "IPv4 Transport Address"
			case 0x0402:
				t.need(n == 4, "Configuration Sequence Number length")
				t.num("Configuration Sequence Number", 4, false)
				t.info["Packet Name"] = "Configuration Sequence Number"
			case 0x0403:
				t.need(n == 16, "IPv6 Transport Address length")
				routingAddress(t, "IPv6 Transport Address", 16)
				t.info["Packet Name"] = "IPv6 Transport Address"
			default:
				t.rest("Unknown Value")
				t.info["Packet Name"] = fmt.Sprintf("Unknown TLV 0x%04x", typ)
				t.info["Extension Decoded"] = false
				if flags&0x8000 == 0 {
					t.info["Receiver Processing"] = "unknown mandatory TLV; acceptance not established"
				}
			}
			t.need(t.at == valueEnd, "LDP TLV trailing bytes")
			if t.err != nil {
				return nil, t.err
			}
			// All ranges use the PDU's coordinate space, including nested TLVs.
			ranges := routingFieldRanges(t.fields)
			ranges["TLV Type"] = []any{uint64(m.at), uint64(m.at + 2)}
			if typ == 0x0400 {
				for _, key := range []string{"Targeted", "Requested", "GTSM", "Reserved"} {
					ranges[key] = []any{uint64(m.at + 6), uint64(m.at + 8)}
				}
			}
			t.info["Wire Field Ranges"] = ranges
			tlvs = append(tlvs, t.info)
			m.at = valueEnd
		}
		if m.err != nil {
			return nil, m.err
		}
		messageRanges := routingFieldRanges(m.fields)
		messageRanges["Unknown Message"] = messageRanges["Message Type"]
		m.info["TLVs"], m.info["Wire Field Ranges"] = tlvs, messageRanges
		messages = append(messages, m.info)
		r.at = end
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["Messages"] = messages
	r.info["Packet Name"] = "LDP Hello"
	r.info["Authentication Verified"] = false
	r.info["Observation Scope"] = "UDP discovery advertisement; TCP session, adjacency and negotiated parameters unobserved"
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	return r.info, nil
}
