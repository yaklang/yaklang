package pcaputil

import (
	"strconv"
	"strings"
)

const (
	h3MethodUnknown = iota
	h3MethodOther
	h3MethodHEAD
	h3MethodCONNECT
)

type h3MessageBody struct {
	length              string // canonical decimal; no integer truncation
	headerPDU, observed uint64
	status              int
	header, hasLength   bool
	normalized          bool
}

const h3BodyScope = "observed reassembled DATA octets and drained FIN; representation size, tunnel payload semantics and peer delivery are not certified"

// Repeated equivalent decimal lengths are accepted as an explicit recipient
// policy. Preserve the actual fields separately; never round through float64.
func h3ContentLength(headers []map[string]any) (value string, present, normalized bool, err error) {
	for _, h := range headers {
		if h["Name"] != "content-length" {
			continue
		}
		literal, _ := h["Value"].(string)
		for _, part := range strings.Split(literal, ",") {
			part = strings.Trim(part, " \t")
			if part == "" {
				return "", false, false, protocolError(ErrMalformedMessage, "HTTP3 empty Content-Length")
			}
			for i := range part {
				if part[i] < '0' || part[i] > '9' {
					return "", false, false, protocolError(ErrMalformedMessage, "HTTP3 non-decimal Content-Length")
				}
			}
			canonical := strings.TrimLeft(part, "0")
			if canonical == "" {
				canonical = "0"
			}
			if present && value != canonical {
				return "", false, false, protocolError(ErrMalformedMessage, "HTTP3 conflicting Content-Length values")
			}
			normalized = normalized || present || canonical != literal
			value, present = canonical, true
		}
	}
	return
}

func (h3 *h3StreamState) configureBody(dir int, kind string, headers []map[string]any, pdu uint64) error {
	length, present, normalized, err := h3ContentLength(headers)
	if err != nil {
		return err
	}
	if kind == "trailers" {
		if present {
			return protocolError(ErrMalformedMessage, "HTTP3 Content-Length in trailers cannot change message framing")
		}
		return nil
	}
	status, method := 0, h3MethodOther
	for _, h := range headers {
		if h["Name"] == ":status" {
			status, _ = strconv.Atoi(h["Value"].(string))
		}
		if h["Name"] == ":method" {
			switch h["Value"] {
			case "HEAD":
				method = h3MethodHEAD
			case "CONNECT":
				method = h3MethodCONNECT
			}
		}
	}
	if dir == 1 && present && (status < 200 || status == 204 || status >= 200 && status < 300 && h3.requestMethod == h3MethodCONNECT) {
		return protocolError(ErrMalformedMessage, "HTTP3 Content-Length forbidden for this response")
	}
	if kind == "informational" {
		return nil // no final-response body state is created by an interim section
	}
	if dir == 0 {
		h3.requestMethod = method
	}
	b := &h3.body[dir]
	b.header, b.headerPDU, b.status = true, pdu, status
	b.length, b.hasLength, b.normalized = length, present, normalized
	return nil
}

func h3DecimalCompare(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

func (h3 *h3StreamState) checkBody(dir int, sid uint64, fin bool) (map[string]any, error) {
	b := &h3.body[dir]
	m := map[string]any{"Stream ID": sid, "Direction": uint64(dir), "Observed DATA Bytes": b.observed, "Drained FIN": fin, "Header PDU ID": b.headerPDU, "Scope": h3BodyScope}
	if b.hasLength {
		m["Declared Content-Length"] = b.length
		if b.normalized {
			m["Length Normalization"] = "equivalent decimal values accepted; literal fields retained"
		}
	}
	bad := func(reason string) (map[string]any, error) {
		m["Length Check"], m["Error"] = "malformed", reason
		return m, protocolError(ErrMalformedMessage, "HTTP3 "+reason)
	}
	if !b.header {
		m["Length Check"] = "awaiting-final-headers"
		if fin {
			return bad("FIN before final HTTP message headers")
		}
		return m, nil
	}
	method := h3.requestMethod
	if h3.fieldFailure[0] != "" || h3.framingFailure[0] {
		method = h3MethodUnknown
	}
	if dir == 1 && (b.status == 204 || b.status == 304 || method == h3MethodHEAD) {
		m["Length Check"] = "exempt-no-content"
		if b.observed != 0 {
			return bad("DATA content forbidden for this response")
		}
		return m, nil
	}
	if method == h3MethodCONNECT && (dir == 0 || b.status >= 200 && b.status < 300) {
		m["Length Check"] = "CONNECT-stream; tunnel bytes are not HTTP content length"
		return m, nil
	}
	if !b.hasLength {
		m["Length Check"] = "not-declared"
		return m, nil
	}
	actual := strconv.FormatUint(b.observed, 10)
	comparison := h3DecimalCompare(actual, b.length)
	if dir == 1 && method == h3MethodUnknown && b.observed == 0 && b.length != "0" {
		m["Length Check"] = "unverified-request-method; HEAD response cannot be excluded"
		return m, nil
	}
	if comparison > 0 {
		return bad("DATA exceeds declared Content-Length")
	}
	if fin && comparison != 0 {
		return bad("drained FIN with DATA shorter than Content-Length")
	}
	m["Length Check"] = "pending-FIN"
	if fin {
		m["Length Check"] = "matched"
	}
	return m, nil
}

func (q *binQUIC) rejectH3Message(st *quicStream, dir int, sid uint64, fr, info map[string]any, frames []map[string]any, err error) error {
	_, pe := classifySessionError(err)
	st.h3.fieldFailure[dir] = ErrMalformedMessage
	if pe != nil {
		st.h3.fieldFailure[dir] = pe.Kind
	}
	h3RejectedFieldStream(fr, info, sid, frames)
	if dir == 0 {
		st.requestPDU = 0
		st.h3.requestMethod = h3MethodUnknown
		delete(fr, "Request PDU ID")
		delete(fr, "Transaction ID")
	}
	st.sources[dir].prune(^uint64(0))
	return &h3FieldStreamError{err}
}
