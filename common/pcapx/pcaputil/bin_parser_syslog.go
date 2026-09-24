package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// binSyslog deliberately keeps no cross-message state. A TCP flow may choose a
// different RFC 6587 framing independently in each direction.
type binSyslog struct{}

type syslogFrameKind string

const (
	syslogDatagram       syslogFrameKind = "udp-datagram"
	syslogOctetCounting  syslogFrameKind = "tcp-octet-counting"
	syslogNonTransparent syslogFrameKind = "tcp-non-transparent-lf"
)

func syslogStreamFraming(w []byte) syslogFrameKind {
	if len(w) > 0 && w[0] >= '0' && w[0] <= '9' {
		return syslogOctetCounting
	}
	return syslogNonTransparent
}

func syslogProfileForWire(w []byte, framing syslogFrameKind) string {
	if framing == syslogOctetCounting {
		space := bytes.IndexByte(w, ' ')
		if space <= 0 || space >= len(w) {
			return "syslog-stream"
		}
		w = w[space+1:]
	}
	if len(w) < 5 || w[0] != '<' {
		return "syslog-stream"
	}
	close := bytes.IndexByte(w[:min(len(w), 5)], '>')
	if close < 2 {
		return "syslog-stream"
	}
	rest := w[close+1:]
	if space := bytes.IndexByte(rest, ' '); space > 0 {
		version, err := strconv.Atoi(string(rest[:space]))
		if err == nil && version == 1 {
			return "syslog-rfc5424-v1"
		}
		if err == nil && version > 0 {
			return "syslog-rfc5424-versioned"
		}
	}
	if len(rest) >= 4 && rest[3] == ' ' && syslogMonth(rest[:3]) {
		return "syslog-rfc3164-bounded"
	}
	return "syslog-stream"
}

// probeSyslogStreamStart recognizes only a complete, bounded framing prefix
// plus a versioned or classic syslog header. Port numbers are not consulted.
func probeSyslogStreamStart(w []byte) bool {
	payload := w
	if len(w) == 0 {
		return false
	}
	if w[0] >= '0' && w[0] <= '9' {
		space := bytes.IndexByte(w, ' ')
		if space < 0 || space > 8 || space == 0 || w[0] == '0' {
			return false
		}
		for _, c := range w[:space] {
			if c < '0' || c > '9' {
				return false
			}
		}
		payload = w[space+1:]
	}
	if len(payload) < 4 || payload[0] != '<' {
		return false
	}
	close := bytes.IndexByte(payload[:min(len(payload), 5)], '>')
	if close < 2 || close > 4 {
		return false
	}
	pri, err := strconv.Atoi(string(payload[1:close]))
	if err != nil || pri < 0 || pri > 191 {
		return false
	}
	rest := payload[close+1:]
	if i := bytes.IndexByte(rest, ' '); i > 0 && i <= 3 && rest[0] != '0' {
		version, parseErr := strconv.Atoi(string(rest[:i]))
		if parseErr == nil && version >= 1 && version <= 999 {
			return true // versioned header; unsupported versions are reported as such
		}
	}
	if len(rest) >= 4 && rest[3] == ' ' {
		return syslogMonth(rest[:3]) // bounded RFC 3164 timestamp
	}
	return false
}

func syslogValidDatagramStart(w []byte) bool {
	if len(w) < 4 || w[0] != '<' {
		return false
	}
	close := bytes.IndexByte(w[:min(len(w), 5)], '>')
	if close < 2 || close > 4 {
		return false
	}
	pri, err := strconv.Atoi(string(w[1:close]))
	if err != nil || pri < 0 || pri > 191 {
		return false
	}
	return probeSyslogStreamStart(w)
}

func (f *binFlow) frameSyslog(w []byte) (int, *binSpec, error) {
	if len(w) == 0 {
		return 0, nil, nil
	}
	if w[0] >= '0' && w[0] <= '9' {
		space := bytes.IndexByte(w, ' ')
		if space < 0 {
			if len(w) > 8 {
				return 0, nil, protocolError(ErrMalformedMessage, "syslog: invalid octet-count prefix")
			}
			return 0, nil, nil
		}
		if space == 0 || space > 8 || w[0] == '0' {
			return 0, nil, protocolError(ErrMalformedMessage, "syslog: invalid octet-count prefix")
		}
		n := 0
		for _, c := range w[:space] {
			if c < '0' || c > '9' {
				return 0, nil, protocolError(ErrMalformedMessage, "syslog: invalid octet-count prefix")
			}
			n = n*10 + int(c-'0')
			if n > f.a.config.MaxMessageBytes {
				return f.a.config.MaxMessageBytes + 1, nil, nil
			}
		}
		if n == 0 {
			return 0, nil, protocolError(ErrMalformedMessage, "syslog: zero-length octet-counted message")
		}
		frameLen := space + 1 + n
		if frameLen > f.a.config.MaxMessageBytes {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if frameLen > len(w) {
			return frameLen, nil, nil
		}
		return frameLen, f.a.specs["syslog/Syslog"], nil
	}
	if w[0] != '<' {
		return 0, nil, protocolError(ErrMalformedMessage, "syslog: unsupported TCP framing")
	}
	if end := bytes.IndexByte(w, '\n'); end >= 0 {
		if end == 0 || end+1 > f.a.config.MaxMessageBytes {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		return end + 1, f.a.specs["syslog/Syslog"], nil
	}
	if len(w) >= f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	return 0, nil, nil
}

func (s *binSyslog) consume(raw []byte, framing syslogFrameKind, maxMessageBytes, maxElements int) (map[string]any, error) {
	if maxMessageBytes <= 0 {
		maxMessageBytes = DefaultParserBudget().MaxMessageBytes
	}
	if len(raw) > maxMessageBytes {
		return map[string]any{"Message Length": len(raw), "Limited": true}, protocolError(ErrResourceExceeded, "syslog: message exceeds byte budget")
	}
	message := raw
	switch framing {
	case syslogOctetCounting:
		space := bytes.IndexByte(raw, ' ')
		if space <= 0 || space > 8 {
			return nil, protocolError(ErrMalformedMessage, "syslog: missing octet-count prefix")
		}
		n, err := strconv.Atoi(string(raw[:space]))
		if err != nil || n <= 0 || n != len(raw)-space-1 {
			return nil, protocolError(ErrMalformedMessage, "syslog: octet-count does not match message boundary")
		}
		message = raw[space+1:]
	case syslogNonTransparent:
		if len(raw) == 0 || raw[len(raw)-1] != '\n' {
			return nil, protocolError(ErrMalformedMessage, "syslog: missing non-transparent trailer")
		}
		message = raw[:len(raw)-1]
		message = bytes.TrimSuffix(message, []byte{'\r'})
	case syslogDatagram:
	default:
		return nil, protocolError(ErrUnsupportedFeature, "syslog: unknown framing profile")
	}
	fields, err := parseSyslogMessage(message, maxElements)
	if err != nil {
		if fields == nil {
			fields = map[string]any{"Message Length": len(message), "Unsupported": true}
		}
		fields["Framing"] = string(framing)
		fields["Message Length"] = len(message)
		return fields, err
	}
	fields["Framing"] = string(framing)
	fields["Message Length"] = len(message)
	return fields, nil
}

func parseSyslogMessage(w []byte, maxElements int) (map[string]any, error) {
	if len(w) == 0 {
		return nil, protocolError(ErrMalformedMessage, "syslog: empty message")
	}
	if maxElements <= 0 {
		maxElements = DefaultParserBudget().MaxCollectionElements
	}
	if w[0] != '<' {
		return nil, protocolError(ErrMalformedMessage, "syslog: missing PRI")
	}
	close := bytes.IndexByte(w[:min(len(w), 5)], '>')
	if close < 2 || close > 4 {
		return nil, protocolError(ErrMalformedMessage, "syslog: invalid PRI")
	}
	pri, err := strconv.Atoi(string(w[1:close]))
	if err != nil || pri < 0 || pri > 191 {
		return nil, protocolError(ErrMalformedMessage, "syslog: PRI outside 0..191")
	}
	fields := map[string]any{
		"PRI": pri, "Facility": pri / 8, "Severity": pri % 8,
		"Facility Name": syslogFacility(pri / 8), "Severity Name": syslogSeverity(pri % 8),
		"Declared Sender": "unverified-message-content",
	}
	if close+1 < len(w) && w[close+1] >= '0' && w[close+1] <= '9' {
		return parseSyslog5424(w, close+1, fields, maxElements)
	}
	return parseSyslog3164(w, close+1, fields)
}

func parseSyslog5424(w []byte, off int, fields map[string]any, maxElements int) (map[string]any, error) {
	readToken := func() (string, int, error) {
		if off >= len(w) {
			return "", off, protocolError(ErrMalformedMessage, "syslog5424: truncated header")
		}
		end := bytes.IndexByte(w[off:], ' ')
		if end < 0 {
			return "", off, protocolError(ErrMalformedMessage, "syslog5424: missing header separator")
		}
		start := off
		tok := string(w[off : off+end])
		off += end + 1
		return tok, start, nil
	}
	version, _, err := readToken()
	if err != nil {
		return nil, err
	}
	versionNumber, parseErr := strconv.Atoi(version)
	if parseErr != nil || versionNumber < 1 || versionNumber > 999 {
		return nil, protocolError(ErrMalformedMessage, "syslog5424: invalid version")
	}
	if versionNumber != 1 {
		return nil, protocolError(ErrUnsupportedVersion, "syslog5424: supported version is 1")
	}
	timestamp, _, err := readToken()
	if err != nil {
		return nil, err
	}
	host, _, err := readToken()
	if err != nil {
		return nil, err
	}
	app, _, err := readToken()
	if err != nil {
		return nil, err
	}
	proc, _, err := readToken()
	if err != nil {
		return nil, err
	}
	msgID, _, err := readToken()
	if err != nil {
		return nil, err
	}
	for _, v := range []string{host, app, proc, msgID} {
		if !syslogToken(v) {
			return nil, protocolError(ErrMalformedMessage, "syslog5424: invalid header token")
		}
	}
	if len(host) > 255 || len(app) > 48 || len(proc) > 128 || len(msgID) > 32 {
		return nil, protocolError(ErrResourceExceeded, "syslog5424: header token exceeds RFC limit")
	}
	fields["Version"], fields["Declared Timestamp"], fields["Hostname"] = 1, timestamp, syslogNil(host)
	fields["App Name"], fields["ProcID"], fields["MsgID"] = syslogNil(app), syslogNil(proc), syslogNil(msgID)
	if timestamp == "-" {
		fields["Time Confidence"] = "unknown"
	} else {
		t, parseErr := time.Parse(time.RFC3339Nano, timestamp)
		if parseErr != nil || !syslogTimestampValid(timestamp) {
			return nil, protocolError(ErrMalformedMessage, "syslog5424: invalid timestamp")
		}
		fields["Timestamp UTC"] = t.UTC().Format(time.RFC3339Nano)
		fields["Time Confidence"] = "declared-offset"
	}
	if off >= len(w) {
		return nil, protocolError(ErrMalformedMessage, "syslog5424: missing structured data")
	}
	var sd []map[string]any
	collectionElements := 0
	if w[off] == '-' {
		off++
	} else {
		for off < len(w) && w[off] == '[' {
			if collectionElements >= maxElements {
				return nil, protocolError(ErrResourceExceeded, "syslog5424: structured-data element budget exceeded")
			}
			elem, next, parameterCount, parseErr := parseSyslogSDElement(w, off, maxElements-collectionElements-1)
			if parseErr != nil {
				return nil, parseErr
			}
			sd = append(sd, elem)
			collectionElements += 1 + parameterCount
			off = next
		}
		if len(sd) == 0 {
			return nil, protocolError(ErrMalformedMessage, "syslog5424: invalid structured data")
		}
	}
	fields["Structured Data"] = sd
	if off < len(w) {
		if w[off] != ' ' {
			return nil, protocolError(ErrMalformedMessage, "syslog5424: missing message separator")
		}
		off++
		msg := w[off:]
		fields["Message Bytes"] = append([]byte(nil), msg...)
		encoding := "opaque-octets"
		if bytes.HasPrefix(msg, []byte{0xef, 0xbb, 0xbf}) {
			body := msg[3:]
			if !utf8.Valid(body) {
				return nil, protocolError(ErrMalformedMessage, "syslog5424: invalid UTF-8 after BOM")
			}
			encoding = "utf-8-bom"
		}
		fields["Message Encoding"] = encoding
		fields["Message Display"] = strconv.QuoteToASCII(string(msg))
	}
	return fields, nil
}

func parseSyslogSDElement(w []byte, off, maxParams int) (map[string]any, int, int, error) {
	if off >= len(w) || w[off] != '[' {
		return nil, off, 0, protocolError(ErrMalformedMessage, "syslog5424: invalid structured-data element")
	}
	start := off + 1
	off = start
	for off < len(w) && w[off] != ' ' && w[off] != ']' {
		off++
	}
	id := string(w[start:off])
	if !syslogSDName(id) {
		return nil, off, 0, protocolError(ErrMalformedMessage, "syslog5424: invalid structured-data id")
	}
	params := make([]map[string]any, 0, min(maxParams, 4))
	byName := make(map[string][]string)
	for off < len(w) && w[off] == ' ' {
		off++
		if off >= len(w) || w[off] == ']' {
			return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: incomplete structured-data parameter")
		}
		nameStart := off
		for off < len(w) && w[off] != '=' && w[off] != ' ' && w[off] != ']' {
			off++
		}
		name := string(w[nameStart:off])
		if !syslogSDName(name) || off+1 >= len(w) || w[off] != '=' || w[off+1] != '"' {
			return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: invalid structured-data parameter")
		}
		off += 2
		var value []byte
		closed := false
		for off < len(w) {
			c := w[off]
			off++
			if c == '"' {
				closed = true
				break
			}
			if c == ']' {
				return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: unescaped close bracket in parameter")
			}
			if c == '\\' {
				if off >= len(w) || (w[off] != '"' && w[off] != '\\' && w[off] != ']') {
					return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: invalid structured-data escape")
				}
				c = w[off]
				off++
			}
			value = append(value, c)
			if len(value) > 4096 {
				return nil, off, len(params), protocolError(ErrResourceExceeded, "syslog5424: structured-data value exceeds budget")
			}
		}
		if !closed || !utf8.Valid(value) {
			return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: unterminated or invalid UTF-8 parameter")
		}
		if len(params) >= maxParams {
			return nil, off, len(params), protocolError(ErrResourceExceeded, "syslog5424: structured-data parameter budget exceeded")
		}
		v := string(value)
		params = append(params, map[string]any{"Name": name, "Value": v})
		byName[name] = append(byName[name], v)
	}
	if off >= len(w) || w[off] != ']' {
		return nil, off, len(params), protocolError(ErrMalformedMessage, "syslog5424: unterminated structured-data element")
	}
	off++
	return map[string]any{"ID": id, "Parameters": params, "Parameters By Name": byName}, off, len(params), nil
}

func parseSyslog3164(w []byte, off int, fields map[string]any) (map[string]any, error) {
	// RFC 3164 places the timestamp immediately after PRI; deployments which
	// emit an extra separator are common, so accept exactly one optional SP.
	if off < len(w) && w[off] == ' ' {
		off++
	}
	start := off
	if len(w) < start+16 || w[start+15] != ' ' || !syslog3164Timestamp(w[start:start+15]) {
		return nil, protocolError(ErrMalformedMessage, "syslog3164: invalid timestamp")
	}
	fields["Version"] = nil
	fields["Declared Timestamp"] = string(w[start : start+15])
	fields["Time Confidence"] = "partial-no-year-or-zone"
	off = start + 16
	space := bytes.IndexByte(w[off:], ' ')
	if space <= 0 {
		return nil, protocolError(ErrMalformedMessage, "syslog3164: missing hostname")
	}
	host := string(w[off : off+space])
	if !syslogVisibleToken(host, 255) {
		return nil, protocolError(ErrMalformedMessage, "syslog3164: invalid hostname")
	}
	off += space + 1
	msg := w[off:]
	fields["Hostname"] = host
	fields["App Name"], fields["ProcID"], fields["MsgID"] = nil, nil, nil
	fields["Structured Data"] = []map[string]any{}
	fields["Message Bytes"] = append([]byte(nil), msg...)
	fields["Message Encoding"] = "unknown"
	fields["Message Display"] = strconv.QuoteToASCII(string(msg))
	return fields, nil
}

func syslogToken(v string) bool {
	if v == "-" {
		return true
	}
	for _, c := range []byte(v) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return len(v) > 0
}

func syslogSDName(v string) bool {
	if len(v) == 0 || len(v) > 32 {
		return false
	}
	for _, c := range []byte(v) {
		if c < 33 || c > 126 || c == '=' || c == ']' || c == '"' {
			return false
		}
	}
	return true
}

func syslogVisibleToken(v string, maxLen int) bool {
	if len(v) == 0 || len(v) > maxLen {
		return false
	}
	for _, c := range []byte(v) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func syslogNil(v string) any {
	if v == "-" {
		return nil
	}
	return v
}

func syslogTimestampValid(v string) bool {
	// RFC 5424 uses the uppercase RFC 3339 form with no more than six
	// fractional digits. Go's parser intentionally accepts a wider fraction
	// and is case tolerant, so validate the wire grammar before parsing.
	if len(v) < 20 || v[4] != '-' || v[7] != '-' || v[10] != 'T' || v[13] != ':' || v[16] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	i := 19
	if i < len(v) && v[i] == '.' {
		i++
		start := i
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
		}
		if i-start < 1 || i-start > 6 {
			return false
		}
	}
	if i >= len(v) {
		return false
	}
	if v[i] == 'Z' {
		i++
	} else if v[i] == '+' || v[i] == '-' {
		if i+6 != len(v) || v[i+3] != ':' {
			return false
		}
		for _, j := range []int{i + 1, i + 2, i + 4, i + 5} {
			if v[j] < '0' || v[j] > '9' {
				return false
			}
		}
		i += 6
	} else {
		return false
	}
	if i != len(v) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, v)
	return err == nil
}

func syslogMonth(b []byte) bool {
	if len(b) != 3 {
		return false
	}
	switch string(b) {
	case "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec":
		return true
	default:
		return false
	}
}

func syslog3164Timestamp(b []byte) bool {
	if len(b) != 15 || b[3] != ' ' || b[6] != ' ' || b[9] != ':' || b[12] != ':' || !syslogMonth(b[:3]) {
		return false
	}
	day := strings.TrimSpace(string(b[4:6]))
	if len(day) < 1 || len(day) > 2 {
		return false
	}
	for _, c := range []byte(day + string(b[7:9]) + string(b[10:12]) + string(b[13:15])) {
		if c < '0' || c > '9' {
			return false
		}
	}
	_, err := time.Parse("Jan _2 15:04:05", string(b))
	return err == nil
}

func syslogFacility(n int) string {
	names := [...]string{"kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news", "uucp", "clock", "authpriv", "ftp", "ntp", "audit", "alert", "clock2", "local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7"}
	if n < 0 || n >= len(names) {
		return fmt.Sprintf("facility-%d", n)
	}
	return names[n]
}

func syslogSeverity(n int) string {
	names := [...]string{"emergency", "alert", "critical", "error", "warning", "notice", "informational", "debug"}
	if n < 0 || n >= len(names) {
		return fmt.Sprintf("severity-%d", n)
	}
	return names[n]
}
