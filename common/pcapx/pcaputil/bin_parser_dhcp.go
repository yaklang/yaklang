package pcaputil

import (
	"encoding/binary"
	"fmt"
)

const dhcpCookie = 0x63825363

// binDHCP is the M0 session state for RFC 2131 Discover/Offer/Request/Ack.
// Ports 67/68 are never consulted. DHCPv6 is out of scope.
type binDHCP struct {
	pending map[uint32]string
}

func probeDHCP(w []byte, limit int) ProbeResult {
	if len(w) < 4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] != 1 && w[0] != 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[1] != 1 || w[2] != 6 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 240 && binary.BigEndian.Uint32(w[236:240]) != dhcpCookie {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("dhcp", "rfc2131", 94)
}

func (f *binFlow) frameDHCP(w []byte) (int, *binSpec, error) {
	s := f.dhcp
	if s == nil {
		return 0, nil, sessionContext("DHCP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	if len(w) < 240 {
		return 0, nil, nil
	}
	if binary.BigEndian.Uint32(w[236:240]) != dhcpCookie {
		return 0, nil, fmt.Errorf("dhcp: invalid magic cookie")
	}
	n, err := dhcpFrameLength(w)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	return n, f.spec("dhcp", "DHCP"), nil
}

func dhcpFrameLength(w []byte) (int, error) {
	at := 240
	for at < len(w) {
		code := w[at]
		if code == 255 {
			return at + 1, nil
		}
		if code == 0 {
			at++
			continue
		}
		if at+1 >= len(w) {
			return 0, nil
		}
		ln := int(w[at+1])
		if at+2+ln > len(w) {
			return 0, nil
		}
		at += 2 + ln
	}
	return 0, nil
}

func dhcpMsgName(t byte) string {
	switch t {
	case 1:
		return "Discover"
	case 2:
		return "Offer"
	case 3:
		return "Request"
	case 5:
		return "Ack"
	case 6:
		return "Nak"
	default:
		return fmt.Sprintf("Type %d", t)
	}
}

func (s *binDHCP) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 240 {
		return nil, fmt.Errorf("dhcp: truncated BOOTP header")
	}
	if binary.BigEndian.Uint32(raw[236:240]) != dhcpCookie {
		return nil, fmt.Errorf("dhcp: invalid magic cookie")
	}
	op := raw[0]
	xid := binary.BigEndian.Uint32(raw[4:8])
	chaddr := append([]byte(nil), raw[28:34]...)
	msgType := byte(0)
	at := 240
	for at < len(raw) {
		code := raw[at]
		if code == 255 {
			break
		}
		if code == 0 {
			at++
			continue
		}
		if at+1 >= len(raw) {
			return nil, fmt.Errorf("dhcp: truncated option header")
		}
		ln := int(raw[at+1])
		if at+2+ln > len(raw) {
			return nil, fmt.Errorf("dhcp: truncated option")
		}
		if code == 53 && ln == 1 {
			msgType = raw[at+2]
		}
		at += 2 + ln
	}
	if msgType == 0 {
		return nil, fmt.Errorf("dhcp: missing message-type option")
	}
	name := dhcpMsgName(msgType)
	out := map[string]any{
		"Packet Name": name, "Operation": int(op), "Xid": xid, "CHADDR": chaddr,
		"Magic Cookie": "99.130.83.99", "Message Type": int(msgType),
	}
	if s.pending == nil {
		s.pending = map[uint32]string{}
	}
	switch msgType {
	case 1, 3:
		s.pending[xid] = name
		out["Role"] = "request"
	case 2, 5, 6:
		out["Role"] = "response"
		if req, ok := s.pending[xid]; ok {
			out["In Reply To"] = req
			if msgType == 5 || msgType == 6 {
				delete(s.pending, xid)
			}
		} else {
			out["Association"] = "missing-request"
		}
	}
	return out, nil
}
