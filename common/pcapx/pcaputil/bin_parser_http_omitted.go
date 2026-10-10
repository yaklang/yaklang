package pcaputil

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func httpHeaderFields(header http.Header) map[string]any {
	fields := make(map[string]any, len(header))
	for name, values := range header {
		fields[name] = append([]string(nil), values...)
	}
	return fields
}

// Preserve a validated header even when body retention exceeds the budget.
// Exact framing continues without running the body decoder or storing the body.
// This is never used to recover from invalid headers or a lost TCP segment.
func (f *binFlow) omitHTTPPayload(dir int, wire []byte, reason string) bool {
	d := &f.directions[dir]
	h := d.http
	if f.protocol != "http" || h == nil || h.omitted || h.tunnel || len(h.rawHeader) == 0 {
		return false
	}
	h.omitted = true
	h.skipPrefix = h.header
	switch {
	case h.closeDelimited:
		h.skipPhase = "close"
	case h.chunked:
		h.skipPrefix = h.cursor
		if h.trailers {
			h.skipPhase = "trailers"
		} else if h.next > 0 {
			h.skipRemaining = int64(h.next - h.cursor)
			h.skipPhase = "data"
		} else if h.skipRemaining > 0 {
			h.skipPhase = "data"
		} else {
			h.skipPhase = "size"
		}
	default:
		h.skipRemaining = h.bodyLength
		h.skipPhase = "fixed"
	}
	e := f.event(dir, h.rawHeader, "limited", h.summary+"; HTTP body omitted: "+reason)
	f.httpEvidence(dir, e)
	e.Completeness = "headers"
	e.ExpertCode = "HTTPPayloadOmitted"
	e.Length = len(h.rawHeader)
	if e.Session == nil {
		e.Session = make(map[string]any)
	}
	e.Session["Method"] = h.method
	e.Session["Payload Retained"] = false
	e.Session["Declared Message Bytes"] = h.declared
	e.Session["Declared Body Bytes"] = h.bodyLength
	e.Session["Max Message Bytes"] = f.a.config.MaxMessageBytes
	e.Session["Max Buffered Bytes"] = f.a.config.MaxBufferedBytes
	e.semanticFields = cloneSession(h.headerFields)
	e.Structured = map[string]any{"fields": cloneSession(h.headerFields)}
	f.a.messages.Add(1)
	f.a.emit(e)
	return true
}

// Returns only consumed bytes. Incomplete chunk lines/trailers remain in the
// ordinary bounded buffer; arbitrarily large chunks are skipped incrementally.
func (h *binHTTPState) consumeOmitted(wire []byte) (used int, done bool, err error) {
	if h.skipPrefix > 0 {
		n := min(h.skipPrefix, len(wire))
		used += n
		h.skipPrefix -= n
		wire = wire[n:]
		if h.skipPrefix > 0 {
			return used, false, nil
		}
	}
	for {
		switch h.skipPhase {
		case "close":
			return used + len(wire), false, nil
		case "fixed", "data":
			n := int(min(h.skipRemaining, int64(len(wire))))
			h.skipRemaining -= int64(n)
			used += n
			wire = wire[n:]
			if h.skipRemaining > 0 {
				return used, false, nil
			}
			if h.skipPhase == "fixed" {
				return used, true, nil
			}
			h.skipPhase = "crlf"
		case "crlf":
			if len(wire) < 2 {
				return used, false, nil
			}
			if !bytes.Equal(wire[:2], []byte("\r\n")) {
				return used, false, fmt.Errorf("HTTP chunk has no trailing CRLF")
			}
			used += 2
			wire = wire[2:]
			h.skipPhase = "size"
		case "size", "trailers":
			at := bytes.Index(wire, []byte("\r\n"))
			limit := 4096
			if h.skipPhase == "trailers" {
				limit = 32<<10 - h.trailerBytes
			}
			if at < 0 {
				if len(wire) > limit {
					return used, false, fmt.Errorf("HTTP chunk line/trailers exceed limit")
				}
				return used, false, nil
			}
			if at+2 > limit {
				return used, false, fmt.Errorf("HTTP chunk line/trailers exceed limit")
			}
			line := string(wire[:at])
			used += at + 2
			wire = wire[at+2:]
			if h.skipPhase == "trailers" {
				h.trailerBytes += at + 2
				if line == "" {
					return used, true, nil
				}
				name, _, ok := strings.Cut(line, ":")
				if !ok || !validHTTPFieldName(name) {
					return used, false, fmt.Errorf("invalid HTTP trailer field")
				}
				continue
			}
			sizeText, _, _ := strings.Cut(line, ";")
			size, e := strconv.ParseUint(sizeText, 16, 32)
			if e != nil {
				return used, false, fmt.Errorf("invalid HTTP chunk size")
			}
			if size == 0 {
				h.skipPhase = "trailers"
			} else {
				h.skipRemaining = int64(size)
				h.skipPhase = "data"
			}
		default:
			return used, false, fmt.Errorf("invalid HTTP omission phase")
		}
	}
}

func validHTTPFieldName(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b)) {
			continue
		}
		return false
	}
	return true
}
