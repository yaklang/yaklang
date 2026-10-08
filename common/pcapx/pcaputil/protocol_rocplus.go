package pcaputil

import (
	"encoding/binary"
	"math"
	"time"
)

// ROC Plus Oct-2022 clock/error profile over TCP. Clock year matches the
// pinned CISA opcode 7/8 codec's network order. CRC is always transmitted;
// Ethernet devices may ignore it. A mismatched CRC is an observation, never a
// claimed receiver rejection. Clock and logical station identity are unverified.
type binROCPlus struct {
	clientDir   int
	clientKnown bool
	exchange    uint64
	pending     *rocClockRequest
	ambiguous   bool
}

type rocClockRequest struct {
	addresses [4]byte
	opcode    byte
	eventID   uint64
}

func rocCRC(w []byte) uint16 {
	var crc uint16
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

func rocFrameSize(w []byte, maxBytes int) (int, error) {
	if len(w) < 6 {
		return 0, nil
	}
	n := 8 + int(w[5])
	if n > maxBytes {
		return 0, protocolError(ErrResourceExceeded, "ROC Plus frame exceeds byte budget")
	}
	return n, nil
}

func rocClockFields(p []byte) (map[string]any, error) {
	if len(p) != 7 && len(p) != 8 {
		return nil, protocolError(ErrMalformedMessage, "ROC Plus clock data length is invalid")
	}
	year := int(binary.BigEndian.Uint16(p[5:7]))
	// Point type 136 documents these ranges. Preserve the reported weekday;
	// neither this field nor the capture timestamp establishes a timezone.
	if p[0] > 59 || p[1] > 59 || p[2] > 23 || p[3] < 1 || p[4] < 1 || p[4] > 12 || year < 2000 || year > 2104 {
		return nil, protocolError(ErrMalformedMessage, "ROC Plus clock field is out of range")
	}
	date := time.Date(year, time.Month(p[4]), int(p[3]), 0, 0, 0, 0, time.UTC)
	if date.Day() != int(p[3]) || date.Month() != time.Month(p[4]) {
		return nil, protocolError(ErrMalformedMessage, "ROC Plus calendar date is invalid")
	}
	f := map[string]any{"Second": p[0], "Minute": p[1], "Hour": p[2], "Day": p[3], "Month": p[4], "Year": year}
	if len(p) == 8 {
		if p[7] < 1 || p[7] > 7 {
			return nil, protocolError(ErrMalformedMessage, "ROC Plus weekday is out of range")
		}
		f["Day Of Week"] = p[7]
		f["Weekday Matches Date"] = int(p[7]) == int(date.Weekday())+1
	}
	return f, nil
}

func decodeROCClock(w []byte, maxElements int) (map[string]any, error) {
	n, err := rocFrameSize(w, len(w))
	if err != nil || n == 0 || n != len(w) {
		return nil, protocolError(ErrMalformedMessage, "ROC Plus incomplete frame or mismatched boundary")
	}
	f := map[string]any{"Destination Unit": w[0], "Destination Group": w[1], "Source Unit": w[2], "Source Group": w[3], "Opcode": w[4], "Data Length": w[5], "CRC": binary.LittleEndian.Uint16(w[len(w)-2:]), "CRC Valid": binary.LittleEndian.Uint16(w[len(w)-2:]) == rocCRC(w[:len(w)-2]), "Observation": "unverified-station-clock", "CRC Policy": "Ethernet receiver may ignore checksum"}
	p := w[6 : len(w)-2]
	switch w[4] {
	case 7:
		f["Packet Name"] = "Read Real-time Clock"
		if len(p) == 0 {
			f["Role"] = "request"
		} else if len(p) == 8 {
			f["Role"] = "response"
		} else {
			return nil, protocolError(ErrMalformedMessage, "ROC Plus read-clock length must be zero or eight")
		}
	case 8:
		f["Packet Name"] = "Set Real-time Clock"
		if len(p) == 7 {
			f["Role"] = "request"
		} else if len(p) == 0 {
			f["Role"] = "response"
		} else {
			return nil, protocolError(ErrMalformedMessage, "ROC Plus set-clock length must be zero or seven")
		}
	case 255:
		if len(p) == 0 || len(p)%2 != 0 {
			return nil, protocolError(ErrMalformedMessage, "ROC Plus error list must contain complete code/offset pairs")
		}
		if len(p)/2 > maxElements {
			return nil, protocolError(ErrResourceExceeded, "ROC Plus error list exceeds collection budget")
		}
		f["Packet Name"], f["Role"] = "Error Indicator", "response"
		pairs := make([]map[string]any, 0, len(p)/2)
		for i := 0; i < len(p); i += 2 {
			// Keep unknown codes, including future codes, as observed numbers.
			pairs = append(pairs, map[string]any{"Code": p[i], "Offset": p[i+1]})
		}
		f["Errors"] = pairs
	default:
		return nil, protocolError(ErrUnsupportedFeature, "ROC Plus opcode is outside the clock/error profile")
	}
	if len(p) > 0 && w[4] != 255 {
		clock, err := rocClockFields(p)
		if err != nil {
			return nil, err
		}
		f["Clock"] = clock
	}
	return f, nil
}

func probeROCPlus(w []byte, maxBytes int) ProbeResult {
	if len(w) < 6 || w[4] != 7 && w[4] != 8 || w[4] == 7 && w[5] != 0 || w[4] == 8 && w[5] != 7 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := rocFrameSize(w, maxBytes)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < n {
		return probeNeed("roc-plus", "tcp-clock-2022", len(w), n)
	}
	if rocCRC(w[:n-2]) != binary.LittleEndian.Uint16(w[n-2:n]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if _, err := decodeROCClock(w[:n], 4096); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("roc-plus", "tcp-clock-2022", 98)
}

func (s *binROCPlus) consume(dir int, raw []byte, eventID uint64, maxElements int) (map[string]any, uint64, error) {
	if s.ambiguous {
		return nil, 0, protocolError(ErrContextRequired, "ROC Plus association is ambiguous until connection close")
	}
	// A second request, even one outside this payload profile, can receive an
	// Error Indicator without a wire transaction ID. Do not pair that later
	// response with an earlier clock request. A recognizable response sent on
	// the wrong direction is diagnosed separately and cannot consume the slot.
	responseShape := len(raw) >= 6 && (raw[4] == 7 && raw[5] == 8 || raw[4] == 8 && raw[5] == 0 || raw[4] == 255)
	if s.pending != nil && s.clientKnown && dir == s.clientDir && !responseShape {
		s.pending, s.ambiguous = nil, true
		return nil, 0, protocolError(ErrContextRequired, "overlapping ROC Plus requests have no unambiguous wire transaction ID")
	}
	f, err := decodeROCClock(raw, maxElements)
	if err != nil {
		return nil, 0, err
	}
	if dir != 0 && dir != 1 || !s.clientKnown {
		return nil, 0, protocolError(ErrContextRequired, "ROC Plus clock correlation requires an observed TCP initiator")
	}
	request := f["Role"] == "request"
	if request != (dir == s.clientDir) {
		return nil, 0, protocolError(ErrContextRequired, "ROC Plus clock direction differs from observed initiator")
	}
	if request {
		if s.pending != nil {
			s.pending, s.ambiguous = nil, true
			return nil, 0, protocolError(ErrContextRequired, "overlapping ROC Plus requests have no unambiguous wire transaction ID")
		}
		if maxElements < 1 {
			return nil, 0, protocolError(ErrResourceExceeded, "ROC Plus request slot exceeds collection budget")
		}
		if s.exchange == math.MaxUint64 {
			return nil, 0, protocolError(ErrResourceExceeded, "ROC Plus observed exchange counter exhausted")
		}
		s.exchange++
		f["Observed Exchange"] = s.exchange
		if raw[0] == 0 {
			// Broadcast recipients must not reply. Never retain a reply slot.
			f["Association"] = "broadcast-no-response"
		} else {
			s.pending = &rocClockRequest{[4]byte(raw[:4]), raw[4], eventID}
		}
		return f, 0, nil
	}
	r := s.pending
	if r == nil || raw[0] != r.addresses[2] || raw[1] != r.addresses[3] || raw[2] != r.addresses[0] || raw[3] != r.addresses[1] || raw[4] != r.opcode && raw[4] != 255 {
		return nil, 0, protocolError(ErrContextRequired, "ROC Plus response has no matching logical station/opcode request")
	}
	f["Association"], f["In Reply To Opcode"], f["Observed Exchange"] = "matched", r.opcode, s.exchange
	s.pending = nil
	return f, r.eventID, nil
}

// Only a complete ROC frame with a trusted boundary can recover at the next
// frame. Framing failures and exhausted resources still stop the session.
func rocRecoverableMessageError(err error) bool {
	pe, ok := err.(*ProtocolError)
	return ok && (pe.Kind == ErrMalformedMessage || pe.Kind == ErrUnsupportedFeature || pe.Kind == ErrContextRequired)
}
