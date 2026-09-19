package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binC37118 is the M0 session state for IEEE C37.118 synchrophasor frames.
// Port 4712 is never consulted. DATA without an observed CFG-2 is ContextRequired.
type binC37118 struct {
	cfgID   uint16
	haveCFG bool
}

func probeC37118(w []byte, limit int) ProbeResult {
	if len(w) < 4 || w[0] != 0xaa || w[1]&0x80 != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ver := w[1] & 0x0f
	kind := w[1] >> 4
	if ver != 1 && ver != 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if kind > 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 16 || n > 65535 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("c37118", "ieee", 90)
}

func c37118CRC(wire []byte) uint16 {
	crc := uint16(0xffff)
	for _, b := range wire {
		crc ^= uint16(b) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func (f *binFlow) frameC37118(w []byte) (int, *binSpec, error) {
	s := f.c37118
	if s == nil {
		return 0, nil, sessionContext("C37.118 session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	if w[0] != 0xaa {
		return 0, nil, fmt.Errorf("c37118: invalid sync word")
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 16 {
		return 0, nil, fmt.Errorf("c37118: invalid frame size")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("c37118", "C37118"), nil
}

func c37118TypeName(kind byte) string {
	switch kind {
	case 0:
		return "DATA"
	case 1:
		return "HEADER"
	case 2:
		return "CFG-1"
	case 3:
		return "CFG-2"
	case 4:
		return "CMD"
	case 5:
		return "CFG-3"
	default:
		return fmt.Sprintf("Type %d", kind)
	}
}

func (s *binC37118) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 16 {
		return nil, fmt.Errorf("c37118: truncated frame")
	}
	if raw[0] != 0xaa || raw[1]&0x80 != 0 {
		return nil, fmt.Errorf("c37118: invalid sync word")
	}
	n := int(binary.BigEndian.Uint16(raw[2:4]))
	if n != len(raw) {
		return nil, fmt.Errorf("c37118: frame size differs from message boundary")
	}
	if c37118CRC(raw[:n-2]) != binary.BigEndian.Uint16(raw[n-2:]) {
		return nil, fmt.Errorf("c37118: CRC mismatch")
	}
	kind := raw[1] >> 4
	id := binary.BigEndian.Uint16(raw[4:6])
	out := map[string]any{
		"Packet Name": c37118TypeName(kind), "Frame Type": int(kind),
		"Version": int(raw[1] & 0x0f), "ID Code": int(id), "Frame Size": n,
	}
	switch kind {
	case 4:
		if n < 18 {
			return nil, fmt.Errorf("c37118: truncated command")
		}
		out["Command"] = int(binary.BigEndian.Uint16(raw[14:16]))
		out["Role"] = "command"
	case 3:
		s.haveCFG, s.cfgID = true, id
		out["Role"] = "configuration"
	case 0:
		out["Role"] = "data"
		if !s.haveCFG || s.cfgID != id {
			out["Configuration Required"] = true
			return out, protocolError(ErrContextRequired, "C37.118 DATA without observed CFG-2")
		}
	}
	return out, nil
}
