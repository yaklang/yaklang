package pcaputil

import (
	"strconv"
	"strings"
)

// The frame/HPACK state has already advanced. HTTP message errors are scoped
// to the stream; they must not discard another stream's compression context.
func (f *binFlow) validateH2Body(dir int, e *ProtocolEvent, s *binH2Stream) error {
	if s == nil || e.Session["Reset"] == true || s.grpcFailed {
		return nil
	}
	fail := func(why string) error { s.grpcFailed = true; return protocolError(ErrMalformedMessage, "HTTP/2 "+why) }
	headers, _ := e.Session["Headers"].([]map[string]any)
	kind, _ := e.Session["Header Kind"].(string)
	status := ""
	for _, h := range headers {
		if h["Name"] == ":status" {
			status, _ = h["Value"].(string)
		}
	}
	if kind == "response" {
		s.noBody[dir] = s.method == "HEAD" || status == "204" || status == "304"
	}
	for _, h := range headers {
		if h["Name"] != "content-length" {
			continue
		}
		if kind == "trailers" || kind == "informational" || status == "204" {
			return fail("Content-Length forbidden in this header block")
		}
		for _, value := range strings.Split(h["Value"].(string), ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				return fail("invalid Content-Length")
			}
			for _, b := range value {
				if b < '0' || b > '9' {
					return fail("invalid Content-Length")
				}
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || s.bodyLengthSet[dir] && s.bodyExpected[dir] != n {
				return fail("conflicting or invalid Content-Length")
			}
			s.bodyLengthSet[dir], s.bodyExpected[dir] = true, n
		}
	}
	if e.Session["Frame Type"] == byte(0) {
		n := int64(len(grpcDATAPayload(e.Raw)))
		if n > 0 && s.noBody[dir] {
			return fail("DATA in a bodyless response")
		}
		s.bodyBytes[dir] += n
	}
	if !s.noBody[dir] && s.bodyLengthSet[dir] && (s.bodyBytes[dir] > s.bodyExpected[dir] || e.Session["End Stream"] == true && s.bodyBytes[dir] != s.bodyExpected[dir]) {
		return fail("DATA length differs from Content-Length")
	}
	e.Session["Body Bytes"] = s.bodyBytes[dir]
	e.Session["HTTP Semantics Validated"] = true
	return nil
}
