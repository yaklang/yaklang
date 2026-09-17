package pcaputil

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// binCoAP is the M0 session state for RFC 7252 CON/NON/ACK/RST on datagrams.
// Port 5683 is never consulted. OSCORE, observe, and CoAP-over-TCP are out of scope.
type binCoAP struct {
	pending map[uint16]string
}

func probeCoAP(w []byte, limit int) ProbeResult {
	if len(w) < 4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0]>>6 != 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0]&0x0f > 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	typ := (w[0] >> 4) & 3
	code := w[1]
	switch typ {
	case 0, 1: // CON/NON carry request codes in the first-version profile
		if code > 4 {
			return ProbeResult{Verdict: ProbeReject}
		}
	case 2, 3: // ACK/RST: empty or 2.xx/4.xx/5.xx
		class := code >> 5
		if code != 0 && class != 2 && class != 4 && class != 5 {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	_ = limit
	return probeAccept("coap", "rfc7252", 88)
}

func (f *binFlow) frameCoAP(w []byte) (int, *binSpec, error) {
	s := f.coap
	if s == nil {
		return 0, nil, sessionContext("CoAP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending))*8); err != nil {
		return 0, nil, err
	}
	n, err := coapMessageLength(w)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	return n, f.spec("extended_protocols", "CoAP"), nil
}

func coapMessageLength(w []byte) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if w[0]>>6 != 1 {
		return 0, fmt.Errorf("coap: version must be 1")
	}
	tkl := int(w[0] & 0x0f)
	if tkl > 8 {
		return 0, fmt.Errorf("coap: invalid token length")
	}
	if len(w) < 4+tkl {
		return 0, nil
	}
	at := 4 + tkl
	for at < len(w) {
		if w[at] == 0xff {
			return len(w), nil
		}
		hdr := w[at]
		at++
		delta, ln := int(hdr>>4), int(hdr&0x0f)
		var ok bool
		if delta, at, ok = coapExt(w, at, delta); !ok {
			return 0, nil
		}
		if ln, at, ok = coapExt(w, at, ln); !ok {
			return 0, nil
		}
		if delta == 15 || ln == 15 {
			return 0, fmt.Errorf("coap: invalid option header")
		}
		if at+ln > len(w) {
			return 0, nil
		}
		at += ln
	}
	return len(w), nil
}

func coapExt(w []byte, at, v int) (int, int, bool) {
	switch v {
	case 13:
		if at >= len(w) {
			return 0, at, false
		}
		return 13 + int(w[at]), at + 1, true
	case 14:
		if at+1 >= len(w) {
			return 0, at, false
		}
		return 269 + int(binary.BigEndian.Uint16(w[at:])), at + 2, true
	case 15:
		return 15, at, true
	default:
		return v, at, true
	}
}

func coapTypeName(t byte) string {
	switch t {
	case 0:
		return "CON"
	case 1:
		return "NON"
	case 2:
		return "ACK"
	case 3:
		return "RST"
	default:
		return fmt.Sprintf("T%d", t)
	}
}

func coapCodeName(code byte) string {
	class, detail := code>>5, code&0x1f
	switch code {
	case 0:
		return "Empty"
	case 1:
		return "GET"
	case 2:
		return "POST"
	case 3:
		return "PUT"
	case 4:
		return "DELETE"
	case 69:
		return "2.05 Content"
	case 128:
		return "4.00 Bad Request"
	case 132:
		return "4.04 Not Found"
	}
	return fmt.Sprintf("%d.%02d", class, detail)
}

func (s *binCoAP) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("coap: truncated header")
	}
	if raw[0]>>6 != 1 {
		return nil, fmt.Errorf("coap: version must be 1")
	}
	typ := (raw[0] >> 4) & 3
	tkl := int(raw[0] & 0x0f)
	if tkl > 8 || len(raw) < 4+tkl {
		return nil, fmt.Errorf("coap: invalid token")
	}
	code := raw[1]
	mid := binary.BigEndian.Uint16(raw[2:4])
	token := append([]byte(nil), raw[4:4+tkl]...)
	paths, payload, err := coapOptions(raw, 4+tkl)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"Packet Name": coapTypeName(typ), "Type": int(typ), "Code": coapCodeName(code),
		"Code Value": int(code), "Message ID": int(mid), "Token": token,
	}
	if len(paths) > 0 {
		out["Uri-Path"] = strings.Join(paths, "/")
	}
	if payload != nil {
		out["Payload Bytes"] = len(payload)
	}
	if s.pending == nil {
		s.pending = map[uint16]string{}
	}
	switch typ {
	case 0, 1:
		s.pending[mid] = coapCodeName(code)
		out["Role"] = "request"
	case 2, 3:
		out["Role"] = "response"
		if req, ok := s.pending[mid]; ok {
			out["In Reply To"] = req
			delete(s.pending, mid)
		} else {
			out["Association"] = "missing-request"
		}
	}
	return out, nil
}

func coapOptions(w []byte, at int) ([]string, []byte, error) {
	deltaBase := 0
	var paths []string
	for at < len(w) {
		if w[at] == 0xff {
			return paths, w[at+1:], nil
		}
		hdr := w[at]
		at++
		delta, ln := int(hdr>>4), int(hdr&0x0f)
		var ok bool
		if delta, at, ok = coapExt(w, at, delta); !ok || delta == 15 {
			return nil, nil, fmt.Errorf("coap: truncated option")
		}
		if ln, at, ok = coapExt(w, at, ln); !ok || ln == 15 {
			return nil, nil, fmt.Errorf("coap: truncated option")
		}
		if at+ln > len(w) {
			return nil, nil, fmt.Errorf("coap: truncated option value")
		}
		deltaBase += delta
		if deltaBase == 11 {
			paths = append(paths, string(w[at:at+ln]))
		}
		at += ln
	}
	return paths, nil, nil
}
