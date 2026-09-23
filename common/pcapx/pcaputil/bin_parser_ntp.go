package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binNTP is the M0 session state for RFC 5905 NTPv4 client/server (mode 3/4).
// Port 123 is never consulted. NTS and control/private modes are out of scope.
type binNTP struct {
	origin []byte
}

func probeNTP(w []byte, limit int) ProbeResult {
	if len(w) < 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	vn := (w[0] >> 3) & 7
	mode := w[0] & 7
	if vn != 4 || (mode != 3 && mode != 4) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 48 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// Stratum is an unsigned byte in the NTP header: 0 is unsynchronized or
	// unspecified, 1-15 are synchronized strata, and 16 is unsynchronized. Values
	// above 16 are invalid and commonly occur in unrelated text protocols. For
	// example, bencoded KRPC starts with "d1:"; its leading 'd' resembles an
	// NTPv4 server header unless this field is checked before admission.
	if w[1] > 16 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("ntp", "4", 90)
}

func (f *binFlow) frameNTP(w []byte) (int, *binSpec, error) {
	s := f.ntp
	if s == nil {
		return 0, nil, sessionContext("NTP session was not observed")
	}
	if err := f.reserveSession(128); err != nil {
		return 0, nil, err
	}
	if len(w) < 48 {
		return 0, nil, nil
	}
	vn := (w[0] >> 3) & 7
	if vn != 4 {
		return 0, nil, fmt.Errorf("ntp: version must be 4")
	}
	return 48, f.spec("ntp", "NTP"), nil
}

func (s *binNTP) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 48 {
		return nil, fmt.Errorf("ntp: truncated header")
	}
	vn := (raw[0] >> 3) & 7
	mode := raw[0] & 7
	if vn != 4 {
		return nil, fmt.Errorf("ntp: version must be 4")
	}
	if mode != 3 && mode != 4 {
		return nil, fmt.Errorf("ntp: first-version profile is mode 3/4")
	}
	name := "Client"
	if mode == 4 {
		name = "Server"
	}
	origin := append([]byte(nil), raw[24:32]...)
	recv := append([]byte(nil), raw[32:40]...)
	xmit := append([]byte(nil), raw[40:48]...)
	out := map[string]any{
		"Packet Name": name, "Version": int(vn), "Mode": int(mode), "Stratum": int(raw[1]),
		"Origin Timestamp": origin, "Receive Timestamp": recv, "Transmit Timestamp": xmit,
	}
	if mode == 3 {
		s.origin = append([]byte(nil), xmit...)
		out["Role"] = "request"
	} else {
		out["Role"] = "response"
		if len(s.origin) == 8 && binary.BigEndian.Uint64(origin) == binary.BigEndian.Uint64(s.origin) {
			out["In Reply To"] = "Client"
		} else if len(s.origin) == 0 {
			out["Association"] = "missing-request"
		}
	}
	return out, nil
}
