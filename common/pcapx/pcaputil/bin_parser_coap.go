package pcaputil

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// binCoAP is the M0 session state for RFC 7252 CON/NON/ACK/RST on datagrams.
// Port 5683 is never consulted. OSCORE, observe, and CoAP-over-TCP are out of scope.
type binCoAP struct {
	pending    map[coapMessageID]coapRequest
	tokens     map[coapToken]uint16
	maxPending int
}

type coapMessageID struct {
	direction int
	mid       uint16
}

type coapToken struct {
	direction int
	token     string
}

type coapRequest struct {
	token string
	name  string
}

func (s *binCoAP) removeRequest(key coapMessageID) {
	if req, ok := s.pending[key]; ok {
		delete(s.tokens, coapToken{key.direction, req.token})
		delete(s.pending, key)
	}
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
	class := code >> 5
	// Automatic discovery needs an assigned RFC 7252 code, not just a response
	// class: otherwise ASCII banners such as AMQP and SSH look like CoAP.
	// Established sessions still accept unrecognized response details.
	switch code {
	case 0, 1, 2, 3, 4, 65, 66, 67, 68, 69, 128, 129, 130, 131, 132, 133, 134, 140, 141, 143, 160, 161, 162, 163, 164, 165:
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	if typ == 3 && code != 0 || typ == 2 && code > 0 && class == 0 || typ == 1 && code == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("coap", "rfc7252", 88)
}

func (f *binFlow) frameCoAP(w []byte) (int, *binSpec, error) {
	s := f.coap
	if s == nil {
		return 0, nil, sessionContext("CoAP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending)+1)*160); err != nil {
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
			if at+1 == len(w) {
				return 0, fmt.Errorf("coap: empty payload marker")
			}
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
		if delta < 0 || ln < 0 {
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
		return -1, at, true
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

func (s *binCoAP) consume(dir int, raw []byte) (map[string]any, error) {
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
	if code == 0 && (tkl != 0 || len(raw) != 4) {
		return nil, fmt.Errorf("coap: empty message has data")
	}
	class := code >> 5
	if (class != 0 && class != 2 && class != 4 && class != 5) || typ == 3 && code != 0 || typ == 2 && class == 0 && code != 0 || typ == 1 && code == 0 {
		return nil, fmt.Errorf("coap: invalid type/code combination")
	}
	mid := binary.BigEndian.Uint16(raw[2:4])
	token := append([]byte(nil), raw[4:4+tkl]...)
	paths, payload, err := coapOptions(raw, 4+tkl, sessionCollectionLimit(s.maxPending))
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
		s.pending = map[coapMessageID]coapRequest{}
		s.tokens = map[coapToken]uint16{}
	}
	tokenKey := coapToken{dir, string(token)}
	key := coapMessageID{dir, mid}
	switch {
	case code > 0 && code < 32:
		// A transaction belongs to its originating endpoint. Retransmission and
		// replacement update both indexes in constant time, even at the budget limit.
		oldMID, sameToken := s.tokens[tokenKey]
		_, sameMID := s.pending[key]
		if !sameToken && !sameMID && len(s.pending) >= sessionCollectionLimit(s.maxPending) {
			return nil, protocolError(ErrResourceExceeded, "CoAP pending request budget exceeded")
		}
		if sameToken {
			s.removeRequest(coapMessageID{dir, oldMID})
		}
		s.removeRequest(key)
		s.pending[key] = coapRequest{tokenKey.token, coapCodeName(code)}
		s.tokens[tokenKey] = mid
		out["Role"] = "request"
	case code == 0:
		out["Role"] = "acknowledgement"
		key.direction = 1 - dir
		if typ == 0 {
			out["Role"] = "ping"
		} else if req, ok := s.pending[key]; ok {
			out["In Reply To"] = req.name
			if typ == 3 {
				s.removeRequest(key)
			}
		} else {
			out["Association"] = "missing-request"
		}
	default:
		out["Role"] = "response"
		tokenKey.direction = 1 - dir
		if requestMID, ok := s.tokens[tokenKey]; ok && (typ != 2 || requestMID == mid) {
			key = coapMessageID{1 - dir, requestMID}
			out["In Reply To"] = s.pending[key].name
			s.removeRequest(key)
		} else {
			out["Association"] = "missing-request"
		}
	}
	return out, nil
}

func coapOptions(w []byte, at, maxOptions int) ([]string, []byte, error) {
	deltaBase := 0
	var paths []string
	for count := 0; at < len(w); count++ {
		if w[at] == 0xff {
			if at+1 == len(w) {
				return nil, nil, fmt.Errorf("coap: empty payload marker")
			}
			return paths, w[at+1:], nil
		}
		if count >= maxOptions {
			return nil, nil, protocolError(ErrResourceExceeded, "CoAP option budget exceeded")
		}
		hdr := w[at]
		at++
		delta, ln := int(hdr>>4), int(hdr&0x0f)
		var ok bool
		if delta, at, ok = coapExt(w, at, delta); !ok || delta < 0 {
			return nil, nil, fmt.Errorf("coap: truncated option")
		}
		if ln, at, ok = coapExt(w, at, ln); !ok || ln < 0 {
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
