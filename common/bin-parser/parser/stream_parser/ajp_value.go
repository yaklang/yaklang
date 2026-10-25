package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ajpMethods = map[byte]string{1: "OPTIONS", 2: "GET", 3: "HEAD", 4: "POST", 5: "PUT", 6: "DELETE", 7: "TRACE", 8: "PROPFIND", 9: "PROPPATCH", 10: "MKCOL", 11: "COPY", 12: "MOVE", 13: "LOCK", 14: "UNLOCK", 15: "ACL", 16: "REPORT", 17: "VERSION-CONTROL", 18: "CHECKIN", 19: "CHECKOUT", 20: "UNCHECKOUT", 21: "SEARCH", 22: "MKWORKSPACE", 23: "UPDATE", 24: "LABEL", 25: "MERGE", 26: "BASELINE-CONTROL", 27: "MKACTIVITY", 28: "MKCALENDAR"}
var ajpRequestHeaders = []string{"", "accept", "accept-charset", "accept-encoding", "accept-language", "authorization", "connection", "content-type", "content-length", "cookie", "cookie2", "host", "pragma", "referer", "user-agent"}
var ajpResponseHeaders = []string{"", "content-type", "content-language", "content-length", "date", "last-modified", "location", "set-cookie", "set-cookie2", "servlet-engine", "status", "www-authenticate"}

func AJPWireLength(w []byte) (int, error) {
	if len(w) == 0 {
		return 0, nil
	}
	if w[0] != 0x12 && w[0] != 0x41 {
		return 0, fmt.Errorf("ajp: invalid magic")
	}
	if len(w) < 2 {
		return 0, nil
	}
	if w[0] == 0x12 && w[1] != 0x34 || w[0] == 0x41 && w[1] != 0x42 {
		return 0, fmt.Errorf("ajp: invalid directional magic")
	}
	if len(w) < 4 {
		return 0, nil
	}
	return 4 + int(binary.BigEndian.Uint16(w[2:4])), nil
}

func DecodeAJPMessage(w []byte, requestBody bool, max int) (map[string]any, error) {
	n, err := AJPWireLength(w)
	if err != nil {
		return nil, err
	}
	if n == 0 || n != len(w) {
		return nil, fmt.Errorf("ajp: exact complete packet required")
	}
	if max < 1 {
		return nil, ErrObservedWireBudget
	}
	proxy := w[0] == 0x12
	b := w[4:]
	out := map[string]any{"Magic": binary.BigEndian.Uint16(w), "Length": len(b), "Proxy To Container": proxy, "Authentication Verified": false}
	ranges := map[string][]int{"Magic": {0, 2}, "Length": {2, 4}}
	out["Wire Field Ranges"] = ranges
	if requestBody {
		if !proxy {
			return out, fmt.Errorf("ajp: untyped body in wrong direction")
		}
		sz := 0
		at := 0
		if len(b) > 0 {
			if len(b) < 2 {
				return out, fmt.Errorf("ajp: request body length missing")
			}
			sz = int(binary.BigEndian.Uint16(b))
			at = 2
			if sz != len(b)-2 {
				return out, fmt.Errorf("ajp: request body chunk length mismatch")
			}
		}
		out["Packet Name"], out["Body Length"], out["Body"], out["End Body"] = "Request Body", sz, bytes.Clone(b[at:]), sz == 0
		ranges["Body"] = []int{4 + at, len(w)}
		return out, nil
	}
	if len(b) == 0 {
		return out, fmt.Errorf("%w: ajp empty untyped body needs a request cycle", ErrObservedWireContext)
	}
	code := b[0]
	out["Code"] = code
	ranges["Code"] = []int{4, 5}
	at := 1
	count := 0
	str := func() (any, error) {
		if len(b)-at < 2 {
			return nil, fmt.Errorf("ajp: string length missing")
		}
		sz := int(binary.BigEndian.Uint16(b[at:]))
		at += 2
		if sz == 65535 {
			return nil, nil
		}
		if sz >= len(b)-at {
			return nil, fmt.Errorf("ajp: string exceeds packet")
		}
		if b[at+sz] != 0 {
			return nil, fmt.Errorf("ajp: string terminator is nonzero")
		}
		raw := b[at : at+sz]
		at += sz + 1
		if utf8.Valid(raw) {
			return string(raw), nil
		}
		quoted := strconv.QuoteToASCII(string(raw))
		return quoted[1 : len(quoted)-1], nil
	}
	readString := func(key string, required bool) error {
		start := at
		v, e := str()
		if e != nil {
			return e
		}
		if required && v == nil {
			return fmt.Errorf("ajp: required %s is null", key)
		}
		out[key] = v
		ranges[key] = []int{4 + start, 4 + at}
		return nil
	}
	u16 := func() (uint16, error) {
		if len(b)-at < 2 {
			return 0, fmt.Errorf("ajp: integer exceeds packet")
		}
		v := binary.BigEndian.Uint16(b[at:])
		at += 2
		return v, nil
	}
	headers := func(response bool) ([]map[string]any, error) {
		v, e := u16()
		if e != nil {
			return nil, e
		}
		out["Header Count"] = v
		if int(v) > max-count {
			return nil, ErrObservedWireBudget
		}
		rows := make([]map[string]any, 0, int(v))
		table := ajpRequestHeaders
		if response {
			table = ajpResponseHeaders
		}
		for i := 0; i < int(v); i++ {
			start := at
			if len(b)-at < 2 {
				return nil, fmt.Errorf("ajp: header name missing")
			}
			key := int(binary.BigEndian.Uint16(b[at:]))
			name := ""
			row := map[string]any{}
			if key&0xff00 == 0xa000 {
				at += 2
				k := key & 255
				if k == 0 || k >= len(table) {
					return nil, fmt.Errorf("ajp: unknown coded header %04x", key)
				}
				name = table[k]
				row["Code"] = uint16(key)
			} else {
				val, e := str()
				if e != nil {
					return nil, e
				}
				s, ok := val.(string)
				if !ok || s == "" {
					return nil, fmt.Errorf("ajp: empty/null header name")
				}
				for _, c := range []byte(s) {
					if c <= 32 || c >= 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={}", rune(c)) {
						return nil, fmt.Errorf("ajp: invalid HTTP header name")
					}
				}
				name = strings.ToLower(s)
			}
			val, e := str()
			if e != nil {
				return nil, e
			}
			if val == nil {
				return nil, fmt.Errorf("ajp: null header value")
			}
			row["Name"], row["Value"], row["Byte Range"] = name, val, []int{4 + start, 4 + at}
			rows = append(rows, row)
			count++
		}
		return rows, nil
	}
	if proxy {
		switch code {
		case 2:
			out["Packet Name"] = "Forward Request"
			if len(b)-at < 1 {
				return out, fmt.Errorf("ajp: method missing")
			}
			method := b[at]
			at++
			if ajpMethods[method] == "" && method != 255 {
				return out, fmt.Errorf("ajp: unknown request method")
			}
			out["Method Code"], out["Method"] = method, ajpMethods[method]
			ranges["Method Code"] = []int{5, 6}
			for _, f := range []struct {
				name     string
				required bool
			}{{"Protocol", true}, {"URI", true}, {"Remote Address", true}, {"Remote Host", false}, {"Server Name", true}} {
				if err = readString(f.name, f.required); err != nil {
					return out, err
				}
			}
			port, e := u16()
			if e != nil {
				return out, e
			}
			if at >= len(b) || b[at] > 1 {
				return out, fmt.Errorf("ajp: SSL boolean missing/invalid")
			}
			out["Server Port"], out["SSL"] = port, b[at] == 1
			at++
			hs, e := headers(false)
			if e != nil {
				return out, e
			}
			out["Headers"] = hs
			var contentLength *uint64
			for _, h := range hs {
				if h["Name"] == "content-length" {
					s := h["Value"].(string)
					if s == "" {
						return out, fmt.Errorf("ajp: empty Content-Length")
					}
					for _, ch := range s {
						if ch < '0' || ch > '9' {
							return out, fmt.Errorf("ajp: invalid Content-Length")
						}
					}
					n, e := strconv.ParseUint(s, 10, 64)
					if e != nil {
						return out, fmt.Errorf("ajp: Content-Length overflow")
					}
					if contentLength != nil && *contentLength != n {
						return out, fmt.Errorf("ajp: conflicting Content-Length")
					}
					contentLength = &n
				}
			}
			if contentLength != nil {
				out["Content Length"] = *contentLength
			}
			var attrs []map[string]any
			seen := map[byte]bool{}
			terminated := false
			for at < len(b) {
				typ := b[at]
				at++
				if typ == 255 {
					terminated = true
					break
				}
				count++
				if count > max {
					return out, ErrObservedWireBudget
				}
				if typ < 1 || typ > 13 {
					return out, fmt.Errorf("ajp: unknown request attribute %d", typ)
				}
				if typ != 10 && seen[typ] {
					return out, fmt.Errorf("ajp: duplicate singleton attribute")
				}
				seen[typ] = true
				a := map[string]any{"Code": typ}
				if typ == 11 {
					v, e := u16()
					if e != nil {
						return out, e
					}
					a["Name"], a["Value"] = "SSL Key Size", v
				} else {
					if typ == 10 {
						name, e := str()
						if e != nil {
							return out, e
						}
						if name == nil {
							return out, fmt.Errorf("ajp: null request attribute name")
						}
						a["Name"] = name
					} else {
						a["Name"] = map[byte]string{1: "Context", 2: "Servlet Path", 3: "Remote User", 4: "Auth Type", 5: "Query String", 6: "Route", 7: "SSL Certificate", 8: "SSL Cipher", 9: "SSL Session", 12: "Secret", 13: "Stored Method"}[typ]
					}
					v, e := str()
					if e != nil {
						return out, e
					}
					a["Value"] = v
					if typ == 13 {
						if v == nil {
							return out, fmt.Errorf("ajp: null stored method")
						}
						out["Stored Method"] = v
					}
				}
				attrs = append(attrs, a)
			}
			if !terminated {
				return out, fmt.Errorf("ajp: missing forward-request FF terminator")
			}
			if method == 255 {
				v, ok := out["Stored Method"]
				if !ok {
					return out, fmt.Errorf("ajp: stored method attribute missing")
				}
				out["Method"] = v
			}
			out["Attributes"] = attrs
		case 7:
			out["Packet Name"] = "Shutdown"
			if at < len(b) {
				if err = readString("Shutdown Secret", false); err != nil {
					return out, err
				}
			}
		case 8:
			out["Packet Name"] = "Ping"
		case 10:
			out["Packet Name"] = "CPing"
		default:
			return out, fmt.Errorf("%w: ajp proxy packet is an unselected body/control", ErrObservedWireContext)
		}
	} else {
		switch code {
		case 3:
			out["Packet Name"] = "Send Body Chunk"
			sz, e := u16()
			if e != nil {
				return out, e
			}
			if len(b)-at != int(sz)+1 || b[len(b)-1] != 0 {
				return out, fmt.Errorf("ajp: response body length/terminator")
			}
			out["Body Length"], out["Body"] = int(sz), bytes.Clone(b[at:at+int(sz)])
			ranges["Body"] = []int{4 + at, 4 + at + int(sz)}
			at = len(b)
		case 4:
			out["Packet Name"] = "Send Headers"
			status, e := u16()
			if e != nil {
				return out, e
			}
			if status < 100 || status > 999 {
				return out, fmt.Errorf("ajp: response status range")
			}
			out["Status Code"] = status
			ranges["Status Code"] = []int{5, 7}
			if err = readString("Status Message", false); err != nil {
				return out, err
			}
			hs, e := headers(true)
			if e != nil {
				return out, e
			}
			out["Headers"] = hs
		case 5:
			out["Packet Name"] = "End Response"
			if at >= len(b) || b[at] > 1 {
				return out, fmt.Errorf("ajp: reuse boolean missing/invalid")
			}
			out["Reuse"] = b[at] == 1
			at++
		case 6:
			out["Packet Name"] = "Get Body Chunk"
			n, e := u16()
			if e != nil {
				return out, e
			}
			out["Requested Bytes"] = n
		case 9:
			out["Packet Name"] = "CPong"
		default:
			return out, fmt.Errorf("ajp: unknown container response code %d", code)
		}
	}
	if at != len(b) {
		return out, fmt.Errorf("ajp: bytes after complete packet grammar")
	}
	return out, nil
}
