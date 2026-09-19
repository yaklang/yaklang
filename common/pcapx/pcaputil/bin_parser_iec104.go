package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binIEC104 is the M0 session state for IEC 60870-5-104 APDUs.
// Port 2404 is never consulted. Serial 101/102/103 are out of scope.
type binIEC104 struct {
	pending map[uint16]string
}

func probeIEC104(w []byte, limit int) ProbeResult {
	if len(w) < 2 || w[0] != 0x68 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(w[1])
	if n < 4 || n > 253 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 4 {
		if len(w) >= 3 {
			if w[2]&0x03 != 3 || iec104UName(w[2]) == "" {
				return ProbeResult{Verdict: ProbeReject}
			}
		}
		_ = limit
		return probeAccept("iec104", "apdu", 92)
	}
	if len(w) < 6 {
		return probeNeed("iec104", "apdu", len(w), 7)
	}
	if w[2]&1 != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 7 && !iec104KnownType(w[6]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 7 {
		return probeNeed("iec104", "apdu", len(w), 7)
	}
	return probeAccept("iec104", "apdu", 92)
}

func iec104UName(ctrl byte) string {
	switch ctrl {
	case 0x07:
		return "STARTDT ACT"
	case 0x0b:
		return "STARTDT CON"
	case 0x13:
		return "STOPDT ACT"
	case 0x23:
		return "STOPDT CON"
	case 0x43:
		return "TESTFR ACT"
	case 0x83:
		return "TESTFR CON"
	}
	return ""
}

func iec104KnownType(id byte) bool {
	switch id {
	case 1, 3, 9, 11, 13, 30, 31, 36, 45, 46, 50, 58, 100, 101, 103:
		return true
	}
	return false
}

func iec104TypeName(id byte) string {
	switch id {
	case 1:
		return "M_SP_NA_1"
	case 3:
		return "M_DP_NA_1"
	case 9:
		return "M_ME_NA_1"
	case 36:
		return "M_ME_TF_NA_1"
	case 45:
		return "C_SC_NA_1"
	case 100:
		return "C_IC_NA_1"
	case 103:
		return "C_CS_NA_1"
	default:
		return fmt.Sprintf("TypeID %d", id)
	}
}

func (f *binFlow) frameIEC104(w []byte) (int, *binSpec, error) {
	s := f.iec104
	if s == nil {
		return 0, nil, sessionContext("IEC 104 session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	if len(w) < 2 {
		return 0, nil, nil
	}
	if w[0] != 0x68 {
		return 0, nil, fmt.Errorf("iec104: APDU must start with 0x68")
	}
	n := 2 + int(w[1])
	if n < 6 {
		return 0, nil, fmt.Errorf("iec104: APDU length shorter than control")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("extended_protocols", "IEC104"), nil
}

func (s *binIEC104) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 6 || raw[0] != 0x68 {
		return nil, fmt.Errorf("iec104: truncated APDU")
	}
	if 2+int(raw[1]) != len(raw) {
		return nil, fmt.Errorf("iec104: APDU length does not match frame")
	}
	ctrl := raw[2:6]
	if s.pending == nil {
		s.pending = map[uint16]string{}
	}
	switch {
	case ctrl[0]&0x03 == 3:
		name := iec104UName(ctrl[0])
		if name == "" {
			return nil, fmt.Errorf("iec104: unknown U-format")
		}
		out := map[string]any{"Packet Name": name, "Format": "U"}
		if ctrl[0] == 0x07 {
			s.pending[0] = name
			out["Role"] = "request"
		} else if ctrl[0] == 0x0b {
			out["Role"] = "response"
			if req, ok := s.pending[0]; ok {
				out["In Reply To"] = req
				delete(s.pending, 0)
			}
		}
		return out, nil
	case ctrl[0]&0x03 == 1:
		recv := binary.LittleEndian.Uint16(ctrl[2:]) >> 1
		return map[string]any{"Packet Name": "S-format", "Format": "S", "Recv Sequence": int(recv)}, nil
	case ctrl[0]&1 == 0:
		if len(raw) < 7 {
			return nil, fmt.Errorf("iec104: truncated I-format")
		}
		send := binary.LittleEndian.Uint16(ctrl[0:]) >> 1
		recv := binary.LittleEndian.Uint16(ctrl[2:]) >> 1
		typeID := raw[6]
		out := map[string]any{
			"Packet Name": iec104TypeName(typeID), "Format": "I", "TypeID": int(typeID),
			"Send Sequence": int(send), "Recv Sequence": int(recv),
		}
		if len(raw) >= 10 {
			cot := binary.LittleEndian.Uint16(raw[8:10]) & 0x3f
			out["COT"] = int(cot)
		}
		if len(raw) >= 15 {
			ioa := uint32(raw[12]) | uint32(raw[13])<<8 | uint32(raw[14])<<16
			out["IOA"] = int(ioa)
		}
		if typeID >= 45 {
			s.pending[send] = iec104TypeName(typeID)
			out["Role"] = "request"
		} else if req, ok := s.pending[recv]; ok {
			out["Role"] = "response"
			out["In Reply To"] = req
			delete(s.pending, recv)
		} else {
			out["Role"] = "indication"
		}
		return out, nil
	}
	return nil, fmt.Errorf("iec104: unknown control format")
}
