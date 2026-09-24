package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binGOOSE is the M0 session state for IEC 61850-8-1 GOOSE PDUs starting at APPID.
// EtherType 0x88b8 is never consulted. MMS/SV/SCL are out of scope.
type binGOOSE struct {
	stNum, sqNum uint64
}

func probeGOOSE(w []byte, limit int) ProbeResult {
	if len(w) < 9 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 10 || n > 1500 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[8] != 0x61 {
		return ProbeResult{Verdict: ProbeReject}
	}
	outerLength, firstField, need, ok := gooseProbeBERLength(w, 9)
	if !ok {
		if need > 0 && need <= limit {
			return probeNeed("goose", "iec61850", len(w), need)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if firstField+outerLength != n || firstField >= n {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) <= firstField {
		if firstField+1 <= limit {
			return probeNeed("goose", "iec61850", len(w), firstField+1)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	// The first field of the GOOSE PDU is its nonempty gocbRef (tag 0x80).
	// A tag byte alone is too weak to admit unrelated binary traffic.
	if w[firstField] != 0x80 {
		return ProbeResult{Verdict: ProbeReject}
	}
	fieldLength, valueAt, need, ok := gooseProbeBERLength(w, firstField+1)
	if !ok {
		if need > 0 && need <= limit {
			return probeNeed("goose", "iec61850", len(w), need)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	fieldEnd := valueAt + fieldLength
	if fieldLength == 0 || fieldEnd > n || fieldEnd > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < fieldEnd {
		return probeNeed("goose", "iec61850", len(w), fieldEnd)
	}
	for _, c := range w[valueAt:fieldEnd] {
		if c < 0x20 || c > 0x7e {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	return probeAccept("goose", "iec61850", 93)
}

// gooseProbeBERLength reads only a bounded BER length header. A full PDU is
// often larger than ProbeBytes, so admission validates the envelope and its
// first mandatory field without requiring the entire packet in the probe.
func gooseProbeBERLength(w []byte, at int) (length, next, need int, ok bool) {
	if at >= len(w) {
		return 0, 0, at + 1, false
	}
	b := w[at]
	at++
	if b < 0x80 {
		return int(b), at, 0, true
	}
	count := int(b & 0x7f)
	if count == 0 || count > 2 {
		return 0, 0, 0, false
	}
	if at+count > len(w) {
		return 0, 0, at + count, false
	}
	for i := 0; i < count; i++ {
		length = length<<8 | int(w[at+i])
	}
	return length, at + count, 0, true
}

func (f *binFlow) frameGOOSE(w []byte) (int, *binSpec, error) {
	s := f.goose
	if s == nil {
		return 0, nil, sessionContext("GOOSE session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if len(w) < 8 {
		return 0, nil, nil
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 10 {
		return 0, nil, fmt.Errorf("goose: invalid frame length")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.a.specs["iec61850/GOOSE"], nil
}

func berRead(w []byte, at int) (byte, []byte, int, error) {
	if at >= len(w) {
		return 0, nil, at, fmt.Errorf("goose: truncated TLV")
	}
	tag := w[at]
	at++
	if at >= len(w) {
		return 0, nil, at, fmt.Errorf("goose: truncated length")
	}
	ln := int(w[at])
	at++
	if ln&0x80 != 0 {
		c := ln & 0x7f
		if c == 0 || c > 4 || at+c > len(w) {
			return 0, nil, at, fmt.Errorf("goose: invalid BER length")
		}
		ln = 0
		for i := 0; i < c; i++ {
			ln = ln<<8 | int(w[at])
			at++
		}
	}
	if at+ln > len(w) {
		return 0, nil, at, fmt.Errorf("goose: truncated value")
	}
	return tag, w[at : at+ln], at + ln, nil
}

func berUint(val []byte) uint64 {
	var n uint64
	for _, b := range val {
		n = n<<8 | uint64(b)
	}
	return n
}

func (s *binGOOSE) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 9 {
		return nil, fmt.Errorf("goose: truncated header")
	}
	n := int(binary.BigEndian.Uint16(raw[2:4]))
	if n != len(raw) {
		return nil, fmt.Errorf("goose: length does not match frame")
	}
	if raw[8] != 0x61 {
		return nil, fmt.Errorf("goose: expected goosePdu")
	}
	_, pdu, _, err := berRead(raw, 8)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"Packet Name": "GOOSE", "APPID": int(binary.BigEndian.Uint16(raw[0:2])), "Length": n,
	}
	at := 0
	for at < len(pdu) {
		tag, val, next, err := berRead(pdu, at)
		if err != nil {
			return nil, err
		}
		switch tag {
		case 0x80:
			out["Control Block Reference"] = string(val)
		case 0x82:
			out["Dataset"] = string(val)
		case 0x83:
			out["GOOSE ID"] = string(val)
		case 0x85:
			s.stNum = berUint(val)
			out["State Number"] = int(s.stNum)
		case 0x86:
			s.sqNum = berUint(val)
			out["Sequence Number"] = int(s.sqNum)
		case 0x8a:
			out["Dataset Entry Count"] = int(berUint(val))
		case 0xab:
			bools, bits := gooseValues(val)
			if len(bools) > 0 {
				out["Boolean"] = bools
			}
			if len(bits) > 0 {
				out["Bits"] = bits
			}
		}
		at = next
	}
	if _, ok := out["Dataset"]; !ok {
		return nil, fmt.Errorf("goose: missing dataset")
	}
	return out, nil
}

func gooseValues(w []byte) ([]bool, [][]byte) {
	var bools []bool
	var bits [][]byte
	at := 0
	for at < len(w) {
		tag, val, next, err := berRead(w, at)
		if err != nil {
			break
		}
		switch tag {
		case 0x83:
			bools = append(bools, len(val) > 0 && val[0] != 0)
		case 0x84:
			if len(val) > 1 {
				bits = append(bits, append([]byte(nil), val[1:]...))
			}
		}
		at = next
	}
	return bools, bits
}
