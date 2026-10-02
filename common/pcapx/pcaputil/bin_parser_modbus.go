package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binModbus is the M0 session state for Modbus TCP MBAP + FC 1–6/15/16.
// Port 502 is never consulted. RTU/ASCII and TLS Modbus are out of scope.
type binModbus struct {
	pending    map[uint16]modbusRequest
	client     int
	hasClient  bool
	maxPending int
}

type modbusRequest struct {
	direction      int
	unit, function byte
	name           string
	addressValue   [4]byte
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
	if n < 2 || n > 254 {
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
	if err := f.reserveSession(256 + int64(len(s.pending)+1)*96); err != nil {
		return 0, nil, err
	}
	if len(w) < 6 {
		return 0, nil, nil
	}
	if binary.BigEndian.Uint16(w[2:4]) != 0 {
		return 0, nil, fmt.Errorf("modbus: protocol id must be zero")
	}
	n := 6 + int(binary.BigEndian.Uint16(w[4:6]))
	if n < 8 || n > 260 {
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

func (s *binModbus) consume(dir int, raw []byte) (map[string]any, error) {
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
		s.pending = map[uint16]modbusRequest{}
	}
	req, matched := s.pending[tid]
	if exc && s.hasClient && dir == s.client {
		return nil, fmt.Errorf("modbus: exception received from request direction")
	}
	response := exc || s.hasClient && dir != s.client || matched && req.direction != dir
	if response && matched && (req.unit != unit || req.function != base) {
		return nil, fmt.Errorf("modbus: response unit/function differs from request")
	}
	if err := validateModbusPDU(raw[7:], response); err != nil {
		return nil, err
	}
	if response && matched && !exc {
		if base <= 4 {
			quantity := int(binary.BigEndian.Uint16(req.addressValue[2:]))
			expected := (quantity + 7) / 8
			if base >= 3 {
				expected = quantity * 2
			}
			if int(raw[8]) != expected {
				return nil, fmt.Errorf("modbus: response byte count differs from requested quantity")
			}
		} else if [4]byte(raw[8:12]) != req.addressValue {
			return nil, fmt.Errorf("modbus: response address or quantity/value differs from request")
		}
	}
	if exc {
		if !s.hasClient {
			s.client, s.hasClient = 1-dir, true
		}
		out["Exception"] = true
		out["Packet Name"] = name + " Exception"
		if len(raw) >= 9 {
			out["Exception Code"] = int(raw[8])
		}
		out["Role"] = "response"
		if req, ok := s.pending[tid]; ok {
			out["In Reply To"] = req.name
			delete(s.pending, tid)
		} else {
			out["Association"] = "missing-request"
		}
		return out, nil
	}
	if response {
		out["Role"] = "response"
		if matched {
			out["In Reply To"] = req.name
			delete(s.pending, tid)
		} else {
			out["Association"] = "missing-request"
		}
	} else {
		s.client, s.hasClient = dir, true
		if !matched && len(s.pending) >= sessionCollectionLimit(s.maxPending) {
			return nil, protocolError(ErrResourceExceeded, "Modbus pending transaction budget exceeded")
		}
		s.pending[tid] = modbusRequest{direction: dir, unit: unit, function: base, name: name, addressValue: [4]byte(raw[8:12])}
		out["Role"] = "request"
	}
	return out, nil
}

func validateModbusPDU(pdu []byte, response bool) error {
	if len(pdu) < 2 || len(pdu) > 253 {
		return fmt.Errorf("modbus: invalid PDU length")
	}
	fc, body := pdu[0], pdu[1:]
	if fc&0x80 != 0 {
		if len(body) != 1 {
			return fmt.Errorf("modbus: invalid exception length")
		}
		return nil
	}
	if response && fc <= 4 {
		if int(body[0]) != len(body)-1 || body[0] == 0 || fc >= 3 && body[0]%2 != 0 {
			return fmt.Errorf("modbus: invalid response byte count")
		}
		return nil
	}
	if fc <= 6 || response {
		if len(body) != 4 {
			return fmt.Errorf("modbus: expected address and quantity/value")
		}
	} else {
		if len(body) < 5 || int(body[4]) != len(body)-5 {
			return fmt.Errorf("modbus: invalid write byte count")
		}
	}
	if !response && fc != 5 && fc != 6 {
		quantity := int(binary.BigEndian.Uint16(body[2:4]))
		limit := 2000
		if fc == 3 || fc == 4 {
			limit = 125
		}
		if fc == 15 {
			limit = 1968
		}
		if fc == 16 {
			limit = 123
		}
		if quantity == 0 || quantity > limit {
			return fmt.Errorf("modbus: invalid quantity")
		}
		if fc == 15 && int(body[4]) != (quantity+7)/8 || fc == 16 && int(body[4]) != quantity*2 {
			return fmt.Errorf("modbus: quantity/byte count mismatch")
		}
	}
	if fc == 5 && binary.BigEndian.Uint16(body[2:4]) != 0 && binary.BigEndian.Uint16(body[2:4]) != 0xff00 {
		return fmt.Errorf("modbus: invalid coil value")
	}
	return nil
}
