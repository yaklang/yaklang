package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binModbus is the M0 session state for Modbus TCP MBAP + FC 1–6/15/16.
// Port 502 is never consulted. RTU/ASCII and TLS Modbus are out of scope.
type binModbus struct {
	pending map[uint16]string
}

func probeModbus(w []byte, limit int) ProbeResult {
	if len(w) < 8 {
		if len(w) >= 4 && binary.BigEndian.Uint16(w[2:4]) == 0 {
			return probeNeed("modbus", "tcp", len(w), 8)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if binary.BigEndian.Uint16(w[2:4]) != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// RFC 9000 long headers put Version in bytes 1–4. Transaction ID 0x8001
	// is 0x80 0x01 and is still a valid MBAP TID.
	if len(w) >= 5 && w[0]&0xc0 == 0xc0 {
		switch binary.BigEndian.Uint32(w[1:5]) {
		case 0, 1:
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	n := int(binary.BigEndian.Uint16(w[4:6]))
	if n < 2 || n > 260 {
		return ProbeResult{Verdict: ProbeReject}
	}
	fc := w[7] & 0x7f
	switch fc {
	case 1, 2, 3, 4, 5, 6, 15, 16:
		_ = limit
		return probeAccept("modbus", "tcp", 91)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func (f *binFlow) frameModbus(w []byte) (int, *binSpec, error) {
	s := f.modbus
	if s == nil {
		return 0, nil, sessionContext("Modbus session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	if len(w) < 6 {
		return 0, nil, nil
	}
	if binary.BigEndian.Uint16(w[2:4]) != 0 {
		return 0, nil, fmt.Errorf("modbus: protocol id must be zero")
	}
	n := 6 + int(binary.BigEndian.Uint16(w[4:6]))
	if n < 8 {
		return 0, nil, fmt.Errorf("modbus: length shorter than unit+function")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("extended_protocols", "ModbusTCP"), nil
}

func modbusFCName(fc byte) string {
	switch fc {
	case 1:
		return "Read Coils"
	case 2:
		return "Read Discrete Inputs"
	case 3:
		return "Read Holding Registers"
	case 4:
		return "Read Input Registers"
	case 5:
		return "Write Single Coil"
	case 6:
		return "Write Single Register"
	case 15:
		return "Write Multiple Coils"
	case 16:
		return "Write Multiple Registers"
	default:
		return fmt.Sprintf("FC %d", fc)
	}
}

func (s *binModbus) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("modbus: truncated MBAP")
	}
	if binary.BigEndian.Uint16(raw[2:4]) != 0 {
		return nil, fmt.Errorf("modbus: protocol id must be zero")
	}
	n := 6 + int(binary.BigEndian.Uint16(raw[4:6]))
	if n != len(raw) {
		return nil, fmt.Errorf("modbus: length does not match frame")
	}
	tid := binary.BigEndian.Uint16(raw[0:2])
	unit, fc := raw[6], raw[7]
	exc := fc&0x80 != 0
	base := fc & 0x7f
	switch base {
	case 1, 2, 3, 4, 5, 6, 15, 16:
	default:
		return nil, fmt.Errorf("modbus: unsupported function %d", fc)
	}
	name := modbusFCName(base)
	out := map[string]any{
		"Packet Name": name, "Transaction ID": int(tid), "Unit ID": int(unit),
		"Function Code": int(fc),
	}
	if s.pending == nil {
		s.pending = map[uint16]string{}
	}
	if exc {
		out["Exception"] = true
		out["Packet Name"] = name + " Exception"
		if len(raw) >= 9 {
			out["Exception Code"] = int(raw[8])
		}
		out["Role"] = "response"
		if req, ok := s.pending[tid]; ok {
			out["In Reply To"] = req
			delete(s.pending, tid)
		} else {
			out["Association"] = "missing-request"
		}
		return out, nil
	}
	if _, ok := s.pending[tid]; ok {
		out["Role"] = "response"
		out["In Reply To"] = s.pending[tid]
		delete(s.pending, tid)
	} else {
		s.pending[tid] = name
		out["Role"] = "request"
	}
	return out, nil
}
