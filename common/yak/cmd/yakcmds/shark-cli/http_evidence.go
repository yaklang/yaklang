package sharkcli

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

// A method word or HTTP/1. prefix is not evidence of an HTTP message. Require
// a complete, syntactically valid start line and header block at this offset.
// Body bytes are intentionally not consumed: a binary body does not invalidate
// real HTTP headers, and CONNECT/101 may switch to a different wire protocol.
type httpEvidence struct {
	line, method   string
	status, length int
	header         http.Header
}

func inspectHTTP(p []byte) *httpEvidence {
	if len(p) > streamPrefixLimit {
		p = p[:streamPrefixLimit]
	}
	end := bytes.Index(p, []byte("\r\n\r\n"))
	first := bytes.Index(p, []byte("\r\n"))
	if end < 0 || first <= 0 || first > 4096 {
		return nil
	}
	line := string(p[:first])
	for _, b := range p[:first] {
		if b < 32 || b >= 127 {
			return nil
		}
	}
	reader := bufio.NewReader(bytes.NewReader(p[:end+4]))
	e := &httpEvidence{line: line, length: end + 4}
	if strings.HasPrefix(line, "HTTP/") {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 || !httpVersion(parts[0]) || len(parts[1]) != 3 {
			return nil
		}
		for _, b := range parts[1] {
			if b < '0' || b > '9' {
				return nil
			}
		}
		resp, err := http.ReadResponse(reader, nil)
		if err != nil || resp.StatusCode < 100 || resp.StatusCode > 599 {
			return nil
		}
		e.status, e.header = resp.StatusCode, resp.Header
	} else {
		parts := strings.Split(line, " ")
		if len(parts) != 3 || !httpVersion(parts[2]) || parts[0] == "" || parts[1] == "" {
			return nil
		}
		for _, b := range parts[0] {
			if !httpToken(b) {
				return nil
			}
		}
		for _, b := range parts[1] {
			if b <= 32 || b >= 127 {
				return nil
			}
		}
		req, err := http.ReadRequest(reader)
		if err != nil {
			return nil
		}
		e.method, e.header = req.Method, req.Header
	}
	// Preserve framing evidence even when net/http normalizes/removes a header
	// (notably Transfer-Encoding). Validate field names before retaining them.
	headers := ""
	if end > first {
		headers = string(p[first+2 : end])
	}
	for _, line := range strings.Split(headers, "\r\n") {
		if line == "" {
			continue
		}
		at := strings.IndexByte(line, ':')
		if at <= 0 {
			return nil
		}
		for _, b := range line[:at] {
			if !httpToken(b) {
				return nil
			}
		}
		value := strings.TrimSpace(line[at+1:])
		for _, b := range value {
			if b < 32 && b != '\t' || b == 127 {
				return nil
			}
		}
		e.header.Set(line[:at], value)
	}
	return e
}
func httpVersion(s string) bool { return s == "HTTP/1.0" || s == "HTTP/1.1" }
func httpToken(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

func httpConnectionContext(prefix [2][]byte) string {
	for d := 0; d < 2; d++ {
		req, resp := inspectHTTP(prefix[d]), inspectHTTP(prefix[1-d])
		if req == nil || resp == nil {
			continue
		}
		if req.method == "CONNECT" && resp.status >= 200 && resp.status < 300 {
			return "HTTP CONNECT tunnel established; subsequent tunnel bytes are not identified as HTTP"
		}
		if req.method != "" && resp.status == 101 && req.header.Get("Upgrade") != "" && resp.header.Get("Upgrade") != "" {
			return fmt.Sprintf("HTTP 101 Upgrade to %s; subsequent bytes use the upgraded protocol", safeText(resp.header.Get("Upgrade")))
		}
	}
	return "HTTP headers observed at the captured prefix; later bytes may be a body or another protocol"
}
