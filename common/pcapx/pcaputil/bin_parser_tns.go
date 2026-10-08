package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binTNS is the M0 session state for Oracle TNS Connect/Accept/Refuse/Data.
// Port 1521 is never consulted. Native crypto and SQL*Net schema are out of scope.
type binTNS struct {
	version uint16
	sawConn bool
	pending int
}

func probeTNS(w []byte, limit int) ProbeResult {
	if len(w) < 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] == 0x05 && w[1] == 0x64 {
		return ProbeResult{Verdict: ProbeReject}
	}
	typ := w[4]
	if typ != 1 && typ != 2 && typ != 4 && typ != 6 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 6 && w[5] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		return probeNeed("tns", "connect", len(w), 8)
	}
	n := int(binary.BigEndian.Uint16(w[:2]))
	if n < 8 || n > 8192 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if typ == 1 {
		// A TNS CONNECT has a fixed 34-byte prefix. Validate the embedded
		// connect-data span before admitting the stream: a SOCKS5 greeting
		// followed by a CONNECT request can otherwise look like a large TNS
		// length plus packet type 1.
		if len(w) < 28 {
			return probeNeed("tns", "connect", len(w), 28)
		}
		if n < 34 {
			return ProbeResult{Verdict: ProbeReject}
		}
		dataLength := int(binary.BigEndian.Uint16(w[24:26]))
		dataOffset := int(binary.BigEndian.Uint16(w[26:28]))
		if dataOffset < 34 || dataOffset > n || dataLength > n-dataOffset {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	_ = limit
	return probeAccept("tns", "connect", 90)
}

func (f *binFlow) frameTNS(w []byte) (int, *binSpec, error) {
	s := f.tns
	if s == nil {
		return 0, nil, sessionContext("TNS session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if len(w) < 8 {
		return 0, nil, nil
	}
	n := int(binary.BigEndian.Uint16(w[:2]))
	if n < 8 {
		return 0, nil, fmt.Errorf("tns: packet shorter than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("tns", "TNS"), nil
}

func tnsPacketName(typ byte) string {
	switch typ {
	case 1:
		return "Connect"
	case 2:
		return "Accept"
	case 3:
		return "Ack"
	case 4:
		return "Refuse"
	case 5:
		return "Redirect"
	case 6:
		return "Data"
	default:
		return fmt.Sprintf("Type %d", typ)
	}
}

func (s *binTNS) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("tns: truncated header")
	}
	n := int(binary.BigEndian.Uint16(raw[:2]))
	if n != len(raw) {
		return nil, fmt.Errorf("tns: length %d does not match frame %d", n, len(raw))
	}
	typ := raw[4]
	if typ < 1 || typ > 15 {
		return nil, fmt.Errorf("tns: unknown packet type")
	}
	out := map[string]any{"Packet Name": tnsPacketName(typ), "Packet Type": int(typ), "Packet Length": n}
	switch typ {
	case 1:
		if n < 34 {
			return nil, fmt.Errorf("tns: CONNECT shorter than header")
		}
		ver := binary.BigEndian.Uint16(raw[8:10])
		s.version, s.sawConn = ver, true
		out["Version"] = int(ver)
		cdl := int(binary.BigEndian.Uint16(raw[24:26]))
		off := int(binary.BigEndian.Uint16(raw[26:28]))
		if cdl > 0 && (off < 34 || off+cdl > n) {
			return nil, fmt.Errorf("tns: CONNECT data outside packet")
		}
		if cdl > 0 {
			out["Connect Data"] = string(raw[off : off+cdl])
		}
		s.pending++
	case 2:
		out["Role"] = "accept"
		if s.pending > 0 {
			out["In Reply To"] = "Connect"
			s.pending--
		}
		if s.version != 0 {
			out["Version"] = int(s.version)
		}
	case 4:
		out["Role"] = "refuse"
		if s.pending > 0 {
			out["In Reply To"] = "Connect"
			s.pending--
		}
	case 6:
		if n < 10 {
			return nil, fmt.Errorf("tns: DATA shorter than data flags")
		}
		out["Data Flag"] = int(binary.BigEndian.Uint16(raw[8:10]))
		if n > 10 {
			out["Bytes"] = n - 10
		}
	}
	return out, nil
}
