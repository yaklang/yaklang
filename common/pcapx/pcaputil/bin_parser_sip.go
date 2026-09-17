package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// binSIP is the M0 session state for RFC 3261. Ports 5060/5061 are never
// consulted. ACK and CANCEL are first-class methods. UDP retransmission is
// the same Call-ID/CSeq/Via-branch seen again before a final response.
type binSIP struct {
	pending map[sipTxnKey]string
	seen    map[sipTxnKey]int
}

type sipTxnKey struct {
	callID, branch, method string
	cseq                   uint32
}

var sipCompact = map[string]string{
	"i": "call-id",
	"m": "contact",
	"e": "content-encoding",
	"l": "content-length",
	"c": "content-type",
	"f": "from",
	"s": "subject",
	"k": "supported",
	"t": "to",
	"v": "via",
}

var sipMethods = []string{
	"INVITE", "ACK", "BYE", "CANCEL", "REGISTER", "OPTIONS",
	"INFO", "PRACK", "SUBSCRIBE", "NOTIFY", "UPDATE", "MESSAGE", "REFER", "PUBLISH",
}

func probeSIP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if !sipLooksLike(w) {
		return ProbeResult{Verdict: ProbeReject}
	}
	line, ok := sipFirstLine(w)
	if !ok {
		if len(w) >= min(limit, 512) {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("sip", "2.0", len(w), min(limit, 64))
	}
	if sipRequestLine(line) || sipResponseLine(line) {
		return probeAccept("sip", "2.0", 93)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func sipLooksLike(w []byte) bool {
	if bytes.HasPrefix(w, []byte("SIP/2.0")) {
		return true
	}
	for _, m := range sipMethods {
		if bytes.HasPrefix(w, []byte(m+" ")) {
			return true
		}
	}
	return false
}

func sipFirstLine(w []byte) (string, bool) {
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		return "", false
	}
	return string(w[:i]), true
}

func sipRequestLine(line string) bool {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || parts[2] != "SIP/2.0" {
		return false
	}
	ok := false
	for _, m := range sipMethods {
		if parts[0] == m {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	uri := strings.ToLower(parts[1])
	return strings.HasPrefix(uri, "sip:") || strings.HasPrefix(uri, "sips:") || uri == "*"
}

func sipResponseLine(line string) bool {
	if !strings.HasPrefix(line, "SIP/2.0 ") || len(line) < 11 {
		return false
	}
	code := line[8:11]
	if code[0] < '1' || code[0] > '6' {
		return false
	}
	return code[1] >= '0' && code[1] <= '9' && code[2] >= '0' && code[2] <= '9'
}

func (f *binFlow) frameSIP(w []byte) (int, *binSpec, error) {
	s := f.sip
	if s == nil {
		return 0, nil, sessionContext("SIP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending)+len(s.seen))*32); err != nil {
		return 0, nil, err
	}
	end := bytes.Index(w, []byte("\r\n\r\n"))
	if end < 0 {
		if len(w) > 32<<10 {
			return 0, nil, fmt.Errorf("sip: header exceeds 32 KiB")
		}
		return 0, nil, nil
	}
	cl, err := sipContentLength(w[:end])
	if err != nil {
		return 0, nil, err
	}
	n := end + 4 + cl
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.a.specs["sip/SIP"], nil
}

func sipContentLength(header []byte) (int, error) {
	hs, err := sipParseHeaders(header)
	if err != nil {
		return 0, err
	}
	v, ok := hs["content-length"]
	if !ok || v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("sip: invalid Content-Length")
	}
	return n, nil
}

func sipParseHeaders(header []byte) (map[string]string, error) {
	lines := bytes.Split(header, []byte("\r\n"))
	if len(lines) == 0 {
		return nil, fmt.Errorf("sip: empty header")
	}
	out := map[string]string{}
	var last string
	for i, line := range lines {
		if i == 0 {
			continue
		}
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if last == "" {
				return nil, fmt.Errorf("sip: folded header without name")
			}
			out[last] = out[last] + " " + string(bytes.TrimSpace(line))
			continue
		}
		name, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			return nil, fmt.Errorf("sip: header missing colon")
		}
		key := sipHeaderName(string(name))
		out[key] = strings.TrimSpace(string(value))
		last = key
	}
	return out, nil
}

func sipHeaderName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if full, ok := sipCompact[n]; ok {
		return full
	}
	return n
}

func (s *binSIP) consume(raw []byte, max int) (map[string]any, error) {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		return nil, fmt.Errorf("sip: truncated header")
	}
	line := string(raw[:bytes.Index(raw, []byte("\r\n"))])
	hs, err := sipParseHeaders(raw[:end])
	if err != nil {
		return nil, err
	}
	callID := hs["call-id"]
	if callID == "" {
		return nil, protocolError(ErrMalformedMessage, "SIP Call-ID is required")
	}
	cseqN, cseqM, err := sipParseCSeq(hs["cseq"])
	if err != nil {
		return nil, err
	}
	from := hs["from"]
	to := hs["to"]
	via := hs["via"]
	info := map[string]any{
		"Call-ID":       callID,
		"CSeq":          cseqN,
		"CSeq Method":   cseqM,
		"From":          from,
		"To":            to,
		"From Tag":      sipParam(from, "tag"),
		"To Tag":        sipParam(to, "tag"),
		"Via":           via,
		"Via Branch":    sipParam(via, "branch"),
		"Content Type":  hs["content-type"],
		"Context Level": "observed",
	}
	key := sipTxnKey{callID: strings.ToLower(callID), branch: sipParam(via, "branch"), method: strings.ToUpper(cseqM), cseq: cseqN}
	body := raw[end+4:]
	if strings.Contains(strings.ToLower(hs["content-type"]), "application/sdp") {
		if sdp := sipParseSDP(body); sdp != nil {
			info["SDP"] = sdp
		}
	}
	if sipResponseLine(line) {
		status, _ := strconv.Atoi(line[8:11])
		info["Packet Name"] = "Response"
		info["Status"] = status
		info["Reason"] = strings.TrimSpace(line[11:])
		if want, ok := s.pending[key]; ok {
			info["Matched Request"] = want
			info["Association Status"] = "matched"
			if status >= 200 {
				delete(s.pending, key)
			}
		} else {
			info["Unmatched"] = true
			info["Association Status"] = "missing-request"
			info["Context Level"] = "partial"
		}
		return info, nil
	}
	if !sipRequestLine(line) {
		return nil, fmt.Errorf("sip: invalid start line")
	}
	method := strings.SplitN(line, " ", 3)[0]
	uri := strings.SplitN(line, " ", 3)[1]
	info["Packet Name"] = method
	info["Method"] = method
	info["URI"] = uri
	if n := s.seen[key]; n > 0 {
		info["Retransmission"] = true
		info["Retransmission Count"] = n
		s.seen[key] = n + 1
		return info, nil
	}
	s.seen[key] = 1
	switch method {
	case "ACK":
		info["Association Status"] = "ack"
	case "CANCEL":
		inv := key
		inv.method = "INVITE"
		if _, ok := s.pending[inv]; ok {
			info["Cancels"] = "INVITE"
		}
		fallthrough
	default:
		if method != "ACK" {
			if max <= 0 {
				max = 4096
			}
			if len(s.pending) >= max {
				return info, protocolError(ErrResourceExceeded, "SIP outstanding transactions exceed budget")
			}
			s.pending[key] = method
			info["Outstanding"] = true
		}
	}
	return info, nil
}

func sipParseCSeq(v string) (uint32, string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq is required")
	}
	num, meth, ok := strings.Cut(v, " ")
	if !ok {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq must be number and method")
	}
	n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 32)
	if err != nil {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq number is invalid")
	}
	return uint32(n), strings.ToUpper(strings.TrimSpace(meth)), nil
}

func sipParam(v, name string) string {
	low := strings.ToLower(v)
	key := ";" + strings.ToLower(name) + "="
	i := strings.Index(low, key)
	if i < 0 {
		return ""
	}
	rest := v[i+len(key):]
	if j := strings.IndexAny(rest, ";>"); j >= 0 {
		rest = rest[:j]
	}
	return strings.Trim(strings.TrimSpace(rest), `"`)
}

func sipParseSDP(body []byte) map[string]any {
	text := string(body)
	if !strings.HasPrefix(text, "v=") {
		return nil
	}
	out := map[string]any{}
	var media []map[string]any
	for _, line := range strings.Split(text, "\r\n") {
		if len(line) < 2 || line[1] != '=' {
			continue
		}
		val := line[2:]
		switch line[0] {
		case 'v':
			out["Version"] = val
		case 'o':
			out["Origin"] = val
		case 's':
			out["Session Name"] = val
		case 'c':
			out["Connection"] = val
		case 'm':
			parts := strings.SplitN(val, " ", 4)
			m := map[string]any{"Line": val}
			if len(parts) >= 3 {
				m["Type"] = parts[0]
				m["Port"] = parts[1]
				m["Proto"] = parts[2]
			}
			if len(parts) >= 4 {
				m["Format"] = parts[3]
			}
			media = append(media, m)
		}
	}
	if len(media) > 0 {
		out["Media"] = media
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
