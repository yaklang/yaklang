package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binRADIUS is the M0 session state for RFC 2865 Access-Request/Accept/Reject.
// Port 1812 is never consulted. User-Password is opaque without keys.
type binRADIUS struct {
	pending map[uint8]string
}

func probeRADIUS(w []byte, limit int) ProbeResult {
	if len(w) < 4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// DHCP/BOOTP: op 1/2, htype 1, hlen 6.
	if (w[0] == 1 || w[0] == 2) && w[1] == 1 && w[2] == 6 {
		return ProbeResult{Verdict: ProbeReject}
	}
	switch w[0] {
	case 1, 2, 3, 11:
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 20 || n > 4096 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// TPKT/X.224 (RDP) is 0x03 0x00 + length. Access-Reject with Identifier 0
	// is not used as a probe fingerprint.
	if w[0] == 3 && w[1] == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("radius", "rfc2865", 92)
}

func (f *binFlow) frameRADIUS(w []byte) (int, *binSpec, error) {
	s := f.radius
	if s == nil {
		return 0, nil, sessionContext("RADIUS session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 20 {
		return 0, nil, fmt.Errorf("radius: length smaller than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("radius", "RADIUS"), nil
}

func radiusCodeName(code byte) string {
	switch code {
	case 1:
		return "Access-Request"
	case 2:
		return "Access-Accept"
	case 3:
		return "Access-Reject"
	case 11:
		return "Access-Challenge"
	default:
		return fmt.Sprintf("Code %d", code)
	}
}

func (s *binRADIUS) consume(raw []byte, max int) (map[string]any, error) {
	if len(raw) < 20 {
		return nil, fmt.Errorf("radius: truncated header")
	}
	n := int(binary.BigEndian.Uint16(raw[2:4]))
	if n != len(raw) {
		return nil, fmt.Errorf("radius: length %d does not match frame %d", n, len(raw))
	}
	code, id := raw[0], raw[1]
	out := map[string]any{
		"Packet Name": radiusCodeName(code), "Code": int(code), "Identifier": int(id),
	}
	if s.pending == nil {
		s.pending = map[uint8]string{}
	}
	at := 20
	attrs := 0
	for at < n {
		if at+2 > n {
			return nil, fmt.Errorf("radius: truncated attribute header")
		}
		if attrs >= max {
			return nil, protocolError(ErrResourceExceeded, "RADIUS attribute budget exceeded")
		}
		typ, alen := raw[at], int(raw[at+1])
		if alen < 2 || at+alen > n {
			return nil, fmt.Errorf("radius: invalid attribute length")
		}
		val := raw[at+2 : at+alen]
		switch typ {
		case 1:
			out["User-Name"] = string(val)
		case 2:
			out["Encrypted"] = true
			out["User-Password"] = "opaque"
			out["User-Password Bytes"] = len(val)
		case 4:
			if len(val) == 4 {
				out["NAS-IP-Address"] = fmt.Sprintf("%d.%d.%d.%d", val[0], val[1], val[2], val[3])
			}
		case 32:
			out["NAS-Identifier"] = string(val)
		}
		attrs++
		at += alen
	}
	switch code {
	case 1:
		s.pending[id] = "Access-Request"
		out["Role"] = "request"
	case 2, 3, 11:
		out["Role"] = "response"
		if req, ok := s.pending[id]; ok {
			out["In Reply To"] = req
			delete(s.pending, id)
		} else {
			out["Association"] = "missing-request"
		}
	default:
		return nil, fmt.Errorf("radius: unsupported code %d", code)
	}
	out["Attribute Count"] = attrs
	return out, nil
}
