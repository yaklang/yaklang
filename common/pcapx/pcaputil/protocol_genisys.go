package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// This TCP wire profile describes observed bytes, not railway signal meaning
// or the control/checkback state machine. The pinned CISA trace uses an FFFF
// CRC seed, while its reverse-engineered codec uses zero. Keep that distinction
// visible and pin the first checksum dialect for the lifetime of the flow.
type binGenisys struct {
	seed      uint16
	masterDir int
}

func genisysHeader(b byte) bool { return b >= 0xf1 && b <= 0xf3 || b >= 0xf9 && b <= 0xfe }

func genisysFrameSize(w []byte, limit int) (int, error) {
	if len(w) == 0 {
		return 0, nil
	}
	if !genisysHeader(w[0]) {
		return 0, protocolError(ErrDesynchronized, "Genisys header is outside the wire profile")
	}
	// At most 4096 pairs, each byte escaped; controls carry no pairs.
	limit = min(limit, 16392)
	if w[0] != 0xf2 && w[0] != 0xf3 && w[0] != 0xf9 && w[0] != 0xfc {
		limit = min(limit, 8)
	}
	if at := bytes.IndexByte(w[1:], 0xf6); at >= 0 {
		n := at + 2
		if n > limit {
			return 0, protocolError(ErrResourceExceeded, "Genisys frame exceeds byte budget")
		}
		return n, nil
	}
	if len(w) >= limit {
		return 0, protocolError(ErrResourceExceeded, "Genisys unterminated frame exceeds byte budget")
	}
	return 0, nil
}

func genisysCRC(w []byte, seed uint16) uint16 {
	crc := seed
	for _, b := range w {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xa001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func genisysBody(w []byte) ([]byte, error) {
	if len(w) < 3 || !genisysHeader(w[0]) || w[len(w)-1] != 0xf6 {
		return nil, protocolError(ErrMalformedMessage, "Genisys frame is incomplete")
	}
	b := make([]byte, 1, len(w)-1)
	b[0] = w[0]
	for i := 1; i < len(w)-1; i++ {
		v := w[i]
		if v == 0xf0 {
			i++
			if i >= len(w)-1 || w[i] > 15 {
				return nil, protocolError(ErrMalformedMessage, "Genisys invalid escape")
			}
			v |= w[i]
		} else if v > 0xf0 {
			return nil, protocolError(ErrMalformedMessage, "Genisys unescaped reserved byte")
		}
		b = append(b, v)
	}
	if len(b) < 2 {
		return nil, protocolError(ErrMalformedMessage, "Genisys slave address is missing")
	}
	return b, nil
}

func decodeGenisys(w []byte, maxElements int, seed int) (map[string]any, uint16, error) {
	b, err := genisysBody(w)
	if err != nil {
		return nil, 0, err
	}
	h, slave := b[0], b[1]
	if slave == 0 && h != 0xf9 {
		return nil, 0, protocolError(ErrMalformedMessage, "Genisys zero address requires common control")
	}
	hasCRC := h != 0xf1 && !(h == 0xfb && len(b) == 2)
	chosen := uint16(0)
	if hasCRC {
		if len(b) < 4 {
			return nil, 0, protocolError(ErrMalformedMessage, "Genisys CRC is incomplete")
		}
		wireCRC := binary.LittleEndian.Uint16(b[len(b)-2:])
		if seed >= 0 {
			chosen = uint16(seed)
			if genisysCRC(b[:len(b)-2], chosen) != wireCRC {
				return nil, 0, protocolError(ErrMalformedMessage, "Genisys checksum does not match the pinned dialect")
			}
		} else if genisysCRC(b[:len(b)-2], 0) == wireCRC {
			chosen = 0
		} else if genisysCRC(b[:len(b)-2], 0xffff) == wireCRC {
			chosen = 0xffff
		} else {
			return nil, 0, protocolError(ErrMalformedMessage, "Genisys checksum is invalid")
		}
	}
	names := map[byte]string{0xf1: "Acknowledge", 0xf2: "Indication Data", 0xf3: "Control Checkback", 0xf9: "Common Control", 0xfa: "Acknowledge and Poll", 0xfb: "Poll", 0xfc: "Control Data", 0xfd: "Recall", 0xfe: "Execute Controls"}
	role := "master-to-slave"
	if h <= 0xf3 {
		role = "slave-to-master"
	}
	f := map[string]any{"Function": h, "Packet Name": names[h], "Slave Address": slave, "Role": role, "CRC Present": hasCRC, "Observation": "unverified-wire-values", "Association": "unassociated-wire-observation"}
	p := b[2:]
	if hasCRC {
		f["CRC"] = binary.LittleEndian.Uint16(b[len(b)-2:])
		f["CRC Seed"] = chosen
		f["CRC Evidence"] = "pinned-reference-code-zero-seed"
		if chosen == 0xffff {
			f["CRC Evidence"] = "pinned-CISA-trace-FFFF-seed; not a normative version claim"
		}
		p = p[:len(p)-2]
	}
	if h == 0xf2 || h == 0xf3 || h == 0xf9 || h == 0xfc {
		if len(p)%2 != 0 {
			return nil, 0, protocolError(ErrMalformedMessage, "Genisys address/value pair is incomplete")
		}
		if len(p)/2 > maxElements {
			return nil, 0, protocolError(ErrResourceExceeded, "Genisys pairs exceed collection budget")
		}
		pairs := make([]map[string]any, 0, len(p)/2)
		for i := 0; i < len(p); i += 2 {
			pairs = append(pairs, map[string]any{"Address": p[i], "Value": p[i+1]})
		}
		f["Pairs"] = pairs
	} else if len(p) != 0 {
		return nil, 0, protocolError(ErrMalformedMessage, "Genisys control frame has trailing data")
	}
	return f, chosen, nil
}

func probeGenisys(w []byte, limit int) ProbeResult {
	if len(w) == 0 || !genisysHeader(w[0]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	// Preserve an independently valid existing MySQL greeting, including
	// packet lengths whose low octet overlaps this protocol's header range.
	if probeMySQL(w, limit).Verdict == ProbeAccept {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 2 && w[1] == 0 && w[0] != 0xf9 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := genisysFrameSize(w, limit)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("genisys", "tcp-observed-wire", len(w), len(w)+1)
	}
	f, seed, err := decodeGenisys(w[:n], 4096, -1)
	// Short checksum-free controls are too weak to identify an initial stream.
	if err != nil || !f["CRC Present"].(bool) {
		return ProbeResult{Verdict: ProbeReject}
	}
	version := "tcp-reference-crc0"
	if seed == 0xffff {
		version = "tcp-trace-crcFFFF"
	}
	return probeAccept("genisys", version, 97)
}

func newBinGenisys(dir int, w []byte) *binGenisys {
	n, _ := genisysFrameSize(w, len(w))
	f, seed, _ := decodeGenisys(w[:n], 4096, -1)
	master := dir
	if f["Role"] == "slave-to-master" {
		master = 1 - dir
	}
	return &binGenisys{seed: seed, masterDir: master}
}

func (g *binGenisys) consume(dir int, w []byte, maxElements int) (map[string]any, error) {
	f, _, err := decodeGenisys(w, maxElements, int(g.seed))
	if err != nil {
		return nil, err
	}
	master := f["Role"] == "master-to-slave"
	if master != (dir == g.masterDir) {
		return nil, protocolError(ErrContextRequired, "Genisys wire role contradicts the observed master direction")
	}
	return f, nil
}

// Only complete delimiter-bounded semantic failures can recover at the next
// frame. Framing and resource failures keep the existing fatal flow behavior.
func genisysRecoverableMessageError(err error) bool {
	pe, ok := err.(*ProtocolError)
	return ok && (pe.Kind == ErrMalformedMessage || pe.Kind == ErrContextRequired)
}
