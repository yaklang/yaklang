package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// Byte-session framing retains its historical RADIUS wire probe. Native UDP
// admission also uses carrier hints after stronger protocol signatures.
// Passwords and authenticators remain opaque without verification keys.
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
	if err := f.reserveSession(s.storage()); err != nil {
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
	case 4:
		return "Accounting-Request"
	case 5:
		return "Accounting-Response"
	case 11:
		return "Access-Challenge"
	default:
		return fmt.Sprintf("Code %d", code)
	}
}

// This compatibility byte carrier is not RADIUS over TCP or RadSec. A fresh
// Code5 response has no sufficiently unique fingerprint. Only a complete Code4
// record with mandatory Accounting content establishes its observed family.
func probeRADIUSAccountingByte(w []byte, maxBytes int) ProbeResult {
	if len(w) < 4 || w[0] != 4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n <= 20 || n > 4096 || n > maxBytes {
		return ProbeResult{Verdict: ProbeReject}
	}
	nas, status, session := false, false, false
	for at := 20; at < n; {
		if len(w)-at < 2 {
			if len(w) < n {
				return probeNeed("radius", "accounting-byte", len(w), n)
			}
			return ProbeResult{Verdict: ProbeReject}
		}
		z := int(w[at+1])
		if z < 2 || z > n-at {
			return ProbeResult{Verdict: ProbeReject}
		}
		if z > len(w)-at {
			return probeNeed("radius", "accounting-byte", len(w), n)
		}
		switch w[at] {
		case 4:
			nas = nas || z == 6
		case 32:
			nas = nas || z > 2
		case 40:
			status = status || z == 6
		case 44:
			session = session || z > 2
		}
		at += z
	}
	if !nas || !status || !session {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("radius", "accounting-byte", 93)
}
