package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binDNP3 is the M0 session state for IEEE 1815 DNP3 link+application.
// Port 20000 is never consulted. Secure Authentication is out of scope.
type binDNP3 struct {
	pending map[uint32]string
}

func probeDNP3(w []byte, limit int) ProbeResult {
	if len(w) < 2 || w[0] != 0x05 || w[1] != 0x64 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 10 {
		return probeNeed("dnp3", "ieee1815", len(w), 10)
	}
	if !dnp3HeaderCRCValid(w[:10]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("dnp3", "ieee1815", 94)
}

func dnp3CRC(data []byte) uint16 {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xA6BC
			} else {
				crc >>= 1
			}
		}
	}
	return crc ^ 0xFFFF
}

func dnp3HeaderCRCValid(hdr []byte) bool {
	if len(hdr) < 10 {
		return false
	}
	return dnp3CRC(hdr[:8]) == binary.LittleEndian.Uint16(hdr[8:10])
}

func dnp3FrameLength(w []byte) (int, error) {
	if len(w) < 10 {
		return 0, nil
	}
	if w[0] != 0x05 || w[1] != 0x64 {
		return 0, fmt.Errorf("dnp3: start bytes must be 05 64")
	}
	user := int(w[2]) - 5
	if user < 0 {
		return 0, fmt.Errorf("dnp3: invalid length")
	}
	n := 10
	for left := user; left > 0; {
		chunk := min(left, 16)
		n += chunk + 2
		left -= chunk
	}
	return n, nil
}

func (f *binFlow) frameDNP3(w []byte) (int, *binSpec, error) {
	s := f.dnp3
	if s == nil {
		return 0, nil, sessionContext("DNP3 session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	n, err := dnp3FrameLength(w)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("extended_protocols", "DNP3"), nil
}

func dnp3UserData(w []byte) ([]byte, error) {
	if !dnp3HeaderCRCValid(w[:10]) {
		return nil, fmt.Errorf("dnp3: header CRC mismatch")
	}
	user := int(w[2]) - 5
	if user < 0 {
		return nil, fmt.Errorf("dnp3: invalid length")
	}
	out := make([]byte, 0, user)
	at := 10
	for left := user; left > 0; {
		chunk := min(left, 16)
		if at+chunk+2 > len(w) {
			return nil, fmt.Errorf("dnp3: truncated user data")
		}
		block := w[at : at+chunk]
		if dnp3CRC(block) != binary.LittleEndian.Uint16(w[at+chunk:at+chunk+2]) {
			return nil, fmt.Errorf("dnp3: user data CRC mismatch")
		}
		out = append(out, block...)
		at += chunk + 2
		left -= chunk
	}
	return out, nil
}

func dnp3FuncName(fc byte) string {
	switch fc {
	case 0x00:
		return "CONFIRM"
	case 0x01:
		return "READ"
	case 0x02:
		return "WRITE"
	case 0x81:
		return "RESPONSE"
	case 0x82:
		return "UNSOLICITED"
	default:
		return fmt.Sprintf("FC %d", fc)
	}
}

func (s *binDNP3) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 10 {
		return nil, fmt.Errorf("dnp3: truncated header")
	}
	dest := binary.LittleEndian.Uint16(raw[4:6])
	src := binary.LittleEndian.Uint16(raw[6:8])
	out := map[string]any{
		"Packet Name": "Link Frame", "Destination": int(dest), "Source": int(src),
		"Control": int(raw[3]),
	}
	user, err := dnp3UserData(raw)
	if err != nil {
		return nil, err
	}
	if s.pending == nil {
		s.pending = map[uint32]string{}
	}
	if len(user) >= 2 {
		fc := user[1]
		out["Function"] = dnp3FuncName(fc)
		out["Packet Name"] = dnp3FuncName(fc)
		key := uint32(src)<<16 | uint32(dest)
		rev := uint32(dest)<<16 | uint32(src)
		if fc&0x80 == 0 {
			s.pending[key] = dnp3FuncName(fc)
			out["Role"] = "request"
		} else {
			out["Role"] = "response"
			if req, ok := s.pending[rev]; ok {
				out["In Reply To"] = req
				delete(s.pending, rev)
			} else {
				out["Association"] = "missing-request"
			}
		}
	}
	return out, nil
}
