package pcaputil

import (
	"encoding/binary"
	"fmt"
	"math"
)

type binIEC104 struct {
	seen     [2]bool
	next     [2]uint16
	active   bool
	uPending [2]map[byte]bool
	uPending [2]map[byte]bool
}

func probeIEC104(w []byte, _ int) ProbeResult {
	if len(w) < 2 || w[0] != 0x68 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 6 {
		return probeNeed("iec104", "104", len(w), 6)
	}
	n := int(w[1])
	if n < 4 || n > 253 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[2]&1 == 0 {
		if n < 10 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < 12 {
			return probeNeed("iec104", "104", len(w), 12)
		}
		if w[6] == 0 || w[7]&127 == 0 {
			return ProbeResult{Verdict: ProbeReject}
		}
	} else if n != 4 || !iecControl(w[2:6]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("iec104", "104", 96)
}
func iecControl(c []byte) bool {
	if len(c) != 4 {
		return false
	}
	if c[0] == 1 {
		return c[1] == 0 && c[2]&1 == 0
	}
	switch c[0] {
	case 7, 11, 19, 35, 67, 131:
		return c[1] == 0 && c[2] == 0 && c[3] == 0
	}
	return false
}
func (f *binFlow) frameIEC104(w []byte) (int, *binSpec, error) {
	if err := f.reserveSession(128); err != nil {
		return 0, nil, err
	}
	if len(w) < 2 {
		return 0, nil, nil
	}
	if w[0] != 0x68 || w[1] < 4 || w[1] > 253 {
		return 0, nil, fmt.Errorf("iec104: APDU length/start")
	}
	return int(w[1]) + 2, f.spec("extended_protocols", "IEC104"), nil
}
func iecTime(w []byte) map[string]any {
	return map[string]any{"Milliseconds": binary.LittleEndian.Uint16(w), "Minute": w[2] & 63, "Invalid": w[2]&128 != 0, "Hour": w[3] & 31, "Summer Time": w[3]&128 != 0, "Day": w[4] & 31, "Weekday": w[4] >> 5, "Month": w[5] & 15, "Year": 2000 + int(w[6]&127)}
}
func iecObjects(a []byte, max int) (map[string]any, error) {
	if len(a) < 6 {
		return nil, fmt.Errorf("iec104: truncated ASDU")
	}
	kind, n, sq := a[0], int(a[1]&127), a[1]&128 != 0
	if n == 0 {
		return nil, fmt.Errorf("iec104: empty ASDU")
	}
	if n > max {
		return nil, protocolError(ErrResourceExceeded, "IEC104 object count")
	}
	out := map[string]any{"Type ID": kind, "Count": n, "Sequential": sq, "Cause": a[2] & 63, "Test": a[2]&128 != 0, "Negative": a[2]&64 != 0, "Originator": a[3], "Common Address": binary.LittleEndian.Uint16(a[4:])}
	sizes := map[byte]int{1: 1, 3: 1, 5: 2, 7: 5, 9: 3, 11: 3, 13: 5, 15: 5, 21: 2, 30: 8, 31: 8, 32: 9, 33: 12, 34: 10, 35: 10, 36: 12, 37: 12, 45: 1, 46: 1, 47: 1, 48: 3, 49: 3, 50: 5, 51: 4, 58: 8, 59: 8, 60: 8, 61: 10, 62: 10, 63: 12, 64: 11, 70: 1, 100: 1, 101: 1, 102: 0, 103: 7, 104: 2, 105: 1, 106: 2, 107: 9}
	size, known := sizes[kind]
	if !known {
		out["Semantic Status"] = "unsupported-asdu-type"
		out["Raw Objects"] = append([]byte(nil), a[6:]...)
		return out, nil
	}
	b := a[6:]
	expected := n * (3 + size)
	if sq {
		expected = 3 + n*size
	}
	if len(b) != expected {
		return nil, fmt.Errorf("iec104: object boundary for type %d", kind)
	}
	objects := make([]map[string]any, 0, n)
	var addr uint32
	for i := 0; i < n; i++ {
		if !sq || i == 0 {
			addr = uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
			b = b[3:]
		} else {
			addr++
			if addr > 0xffffff {
				return nil, fmt.Errorf("iec104: IOA overflow")
			}
		}
		v := b[:size]
		b = b[size:]
		o := map[string]any{"Address": addr, "Raw Value": append([]byte(nil), v...)}
		switch kind {
		case 1, 30:
			o["Value"] = v[0]&1 != 0
			o["Quality"] = v[0] & 0xf0
		case 3, 31:
			o["Value"] = v[0] & 3
			o["Quality"] = v[0] & 0xf0
		case 9, 11, 34, 35:
			o["Value"] = int16(binary.LittleEndian.Uint16(v))
			o["Quality"] = v[2]
		case 13, 36:
			o["Value"] = math.Float32frombits(binary.LittleEndian.Uint32(v))
			o["Quality"] = v[4]
		case 45, 46, 47, 58, 59, 60:
			o["Command"] = v[0] & 3
			o["Select"] = v[0]&128 != 0
			o["Qualifier"] = (v[0] >> 2) & 31
		case 100, 101:
			o["Qualifier"] = v[0]
		}
		if (kind >= 30 && kind <= 37) || (kind >= 58 && kind <= 64) || kind == 103 || kind == 107 {
			o["CP56Time2a"] = iecTime(v[len(v)-7:])
		}
		objects = append(objects, o)
	}
	out["Objects"] = objects
	return out, nil
}
func (s *binIEC104) consume(dir int, w []byte, max int) (map[string]any, error) {
	if len(w) < 6 || w[0] != 0x68 || int(w[1])+2 != len(w) {
		return nil, fmt.Errorf("iec104: APDU")
	}
	c := w[2:6]
	out := map[string]any{}
	if c[0]&1 == 0 {
		if c[2]&1 != 0 {
			return nil, fmt.Errorf("iec104: receive sequence reserved bit")
		}
		asdu, err := iecObjects(w[6:], max)
		if err != nil {
			return nil, err
		}
		seq := binary.LittleEndian.Uint16(c) >> 1
		out["Frame Type"] = "I"
		out["Send Sequence"] = seq
		out["Receive Sequence"] = binary.LittleEndian.Uint16(c[2:]) >> 1
		out["ASDU"] = asdu
		out["Format"] = "I"
		out["TypeID"] = int(w[6])
		out["COT"] = int(w[8] & 63)
		out["Packet Name"] = iec104TypeName(w[6])
		out["Recv Sequence"] = int(binary.LittleEndian.Uint16(c[2:]) >> 1)
		out["Role"] = "indication"
		if objects, ok := asdu["Objects"].([]map[string]any); ok && len(objects) > 0 {
			out["IOA"] = int(objects[0]["Address"].(uint32))
		}
		if w[8]&63 == 6 || w[8]&63 == 8 {
			out["Role"] = "request"
		} else if w[8]&63 == 7 || w[8]&63 == 9 || w[8]&63 == 10 {
			out["Role"] = "response"
		}
		if s.seen[dir] && seq != s.next[dir] {
			out["Sequence Gap"] = true
			out["Expected Sequence"] = s.next[dir]
		} else if !s.seen[dir] {
			out["First Observed Sequence"] = true
		}
		s.seen[dir] = true
		s.next[dir] = (seq + 1) & 32767
	} else {
		if len(w) != 6 || !iecControl(c) {
			return nil, fmt.Errorf("iec104: invalid S/U control")
		}
		if c[0] == 1 {
			out["Frame Type"] = "S"
			out["Format"] = "S"
			out["Packet Name"] = "S-format"
			out["Recv Sequence"] = int(binary.LittleEndian.Uint16(c[2:]) >> 1)
			out["Receive Sequence"] = binary.LittleEndian.Uint16(c[2:]) >> 1
		} else {
			out["Frame Type"] = "U"
			out["Format"] = "U"
			out["Packet Name"] = iec104UName(c[0])
			if s.uPending[0] == nil {
				s.uPending = [2]map[byte]bool{{}, {}}
			}
			if c[0] == 7 || c[0] == 19 || c[0] == 67 {
				s.uPending[dir][c[0]] = true
				out["Role"] = "request"
			} else {
				out["Role"] = "response"
				act := map[byte]byte{11: 7, 35: 19, 131: 67}[c[0]]
				if s.uPending[1-dir][act] {
					out["In Reply To"] = iec104UName(act)
					delete(s.uPending[1-dir], act)
				}
			}
			out["Function"] = map[byte]string{7: "STARTDT act", 11: "STARTDT con", 19: "STOPDT act", 35: "STOPDT con", 67: "TESTFR act", 131: "TESTFR con"}[c[0]]
			if c[0] == 11 {
				s.active = true
			}
			if c[0] == 35 {
				s.active = false
			}
		}
	}
	out["Observed STARTDT Confirmation"] = s.active
	return out, nil
}

func iec104UName(c byte) string {
	return map[byte]string{7: "STARTDT ACT", 11: "STARTDT CON", 19: "STOPDT ACT", 35: "STOPDT CON", 67: "TESTFR ACT", 131: "TESTFR CON"}[c]
}
func iec104TypeName(c byte) string {
	if n, ok := map[byte]string{1: "M_SP_NA_1", 3: "M_DP_NA_1", 9: "M_ME_NA_1", 11: "M_ME_NB_1", 13: "M_ME_NC_1", 30: "M_SP_TB_1", 31: "M_DP_TB_1", 34: "M_ME_TD_1", 35: "M_ME_TE_1", 36: "M_ME_TF_1", 45: "C_SC_NA_1", 100: "C_IC_NA_1", 103: "C_CS_NA_1"}[c]; ok {
		return n
	}
	return fmt.Sprintf("TypeID %d", c)
}
