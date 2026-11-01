package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

// DecodeICMPv4HeaderWire decodes observed type-specific fields, not routing,
// neighbor trust, checksum validity or the semantics of a quoted datagram.
// Incomplete inputs return only whole available fields together with an error.
func DecodeICMPv4HeaderWire(w []byte, maxBytes int) (map[string]any, error) {
	if maxBytes <= 0 || len(w) > maxBytes {
		return nil, ErrObservedWireBudget
	}
	if len(w) < 8 {
		return nil, fmt.Errorf("icmp: generic header requires 8 bytes, got %d", len(w))
	}
	typ, code := w[0], w[1]
	m := map[string]any{"Type": typ, "Code": code}
	switch typ {
	case 0, 8, 13, 14, 15, 16, 17, 18:
		m["Identifier"], m["Sequence"] = binary.BigEndian.Uint16(w[4:6]), binary.BigEndian.Uint16(w[6:8])
	default:
		// RFC 4884 can use bits formerly called unused. Preserve them without
		// inventing echo identity or asserting a selected extension layout.
		m["Rest of Header"] = bytes.Clone(w[4:8])
	}
	switch typ {
	case 3:
		if code == 4 {
			m["MTU"] = binary.BigEndian.Uint16(w[6:8])
		}
	case 5:
		m["Gateway"] = net.IP(w[4:8]).String()
	case 12:
		m["Pointer"] = w[4]
	}
	if typ == 3 || typ == 4 || typ == 5 || typ == 11 || typ == 12 {
		m["Quoted Packet"] = bytes.Clone(w[8:])
	}
	required := 8
	if typ == 13 || typ == 14 {
		required = 20
		for i, name := range []string{"Originate Timestamp", "Receive Timestamp", "Transmit Timestamp"} {
			end := 12 + i*4
			if len(w) >= end {
				m[name] = binary.BigEndian.Uint32(w[end-4 : end])
			}
		}
	} else if typ == 17 || typ == 18 {
		required = 12
		if len(w) >= 12 {
			m["Address Mask"] = net.IP(w[8:12]).String()
		}
	}
	if len(w) < required {
		m["Available ICMP Bytes"], m["Required ICMP Header Bytes"] = len(w), required
		return m, fmt.Errorf("icmp: type %d fixed header requires %d bytes, got %d", typ, required, len(w))
	}
	return m, nil
}
