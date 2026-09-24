package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	stompMaxLineBytes   = 4096
	stompMaxHeaderBytes = 16 << 10
	stompMaxHeaders     = 64
)

type stompFrame struct {
	command   string
	headers   map[string]string
	body      []byte
	total     int
	heartbeat bool
}

type binSTOMP struct {
	clientDir int
	version   string
}

var stompCommands = map[string]bool{
	"CONNECT": true, "STOMP": true, "CONNECTED": true,
	"SEND": true, "SUBSCRIBE": true, "UNSUBSCRIBE": true,
	"ACK": true, "NACK": true, "BEGIN": true, "COMMIT": true,
	"ABORT": true, "DISCONNECT": true, "MESSAGE": true,
	"RECEIPT": true, "ERROR": true,
}

var stompServerCommands = map[string]bool{"CONNECTED": true, "MESSAGE": true, "RECEIPT": true, "ERROR": true}
var stompClientCommands = map[string]bool{
	"CONNECT": true, "STOMP": true, "SEND": true, "SUBSCRIBE": true,
	"UNSUBSCRIBE": true, "ACK": true, "NACK": true, "BEGIN": true,
	"COMMIT": true, "ABORT": true, "DISCONNECT": true,
}

func stompWrongDirection(command string, direction, clientDirection int) bool {
	return direction == clientDirection && stompServerCommands[command] ||
		direction != clientDirection && stompClientCommands[command]
}

// parseSTOMPLine reads one LF or CRLF line without trimming its contents.
// complete=false means that the available bytes could still form a line.
func parseSTOMPLine(w []byte, offset, max int) (line []byte, next int, complete bool, err error) {
	if offset >= len(w) {
		return nil, 0, false, nil
	}
	if offset >= max {
		return nil, 0, false, protocolError(ErrResourceExceeded, "stomp: header exceeds frame limit")
	}
	endLimit := min(len(w), offset+min(stompMaxLineBytes+2, max-offset))
	rel := bytes.IndexByte(w[offset:endLimit], '\n')
	if rel < 0 {
		if len(w)-offset >= stompMaxLineBytes+2 || len(w) >= max {
			return nil, 0, false, protocolError(ErrResourceExceeded, "stomp: line exceeds limit")
		}
		return nil, 0, false, nil
	}
	end := offset + rel
	line = w[offset:end]
	if len(line) > stompMaxLineBytes {
		return nil, 0, false, protocolError(ErrResourceExceeded, "stomp: line exceeds limit")
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	if bytes.IndexByte(line, '\r') >= 0 || !utf8.Valid(line) {
		return nil, 0, false, protocolError(ErrMalformedMessage, "stomp: invalid line encoding")
	}
	return line, end + 1, true, nil
}

func stompDecodeHeader(v []byte) (string, error) {
	var out strings.Builder
	out.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' {
			if v[i] < 0x20 || v[i] == 0x7f {
				return "", protocolError(ErrMalformedMessage, "stomp: control byte in header")
			}
			out.WriteByte(v[i])
			continue
		}
		i++
		if i >= len(v) {
			return "", protocolError(ErrMalformedMessage, "stomp: truncated header escape")
		}
		switch v[i] {
		case 'r':
			out.WriteByte('\r')
		case 'n':
			out.WriteByte('\n')
		case 'c':
			out.WriteByte(':')
		case '\\':
			out.WriteByte('\\')
		default:
			return "", protocolError(ErrMalformedMessage, "stomp: undefined header escape")
		}
	}
	if !utf8.ValidString(out.String()) {
		return "", protocolError(ErrMalformedMessage, "stomp: invalid header UTF-8")
	}
	return out.String(), nil
}

func stompParseDecimal(s string, max int) (int, error) {
	if s == "" {
		return 0, protocolError(ErrMalformedMessage, "stomp: empty content-length")
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, protocolError(ErrMalformedMessage, "stomp: invalid content-length")
		}
	}
	n, err := strconv.ParseUint(s, 10, 63)
	if err != nil || n > uint64(max) {
		return 0, protocolError(ErrResourceExceeded, "stomp: content-length exceeds limit")
	}
	return int(n), nil
}

// parseSTOMPFrame parses exactly one bounded STOMP frame. It deliberately
// reports an incomplete prefix separately from malformed wire data so TCP
// reassembly can wait for the remaining bytes.
func parseSTOMPFrame(w []byte, max int) (stompFrame, bool, error) {
	var frame stompFrame
	if max <= 0 {
		return frame, false, protocolError(ErrResourceExceeded, "stomp: invalid frame limit")
	}
	if len(w) == 0 {
		return frame, false, nil
	}
	if w[0] == '\n' {
		return stompFrame{total: 1, heartbeat: true}, true, nil
	}
	if w[0] == '\r' {
		if len(w) == 1 {
			return frame, false, nil
		}
		if w[1] != '\n' {
			return frame, false, protocolError(ErrMalformedMessage, "stomp: bare carriage return")
		}
		return stompFrame{total: 2, heartbeat: true}, true, nil
	}
	if len(w) > max {
		w = w[:max]
	}

	line, cursor, complete, err := parseSTOMPLine(w, 0, max)
	if err != nil || !complete {
		return frame, false, err
	}
	if len(line) == 0 || !stompCommands[string(line)] {
		return frame, false, protocolError(ErrMalformedMessage, "stomp: unknown or empty command")
	}
	frame.command = string(line)
	frame.headers = make(map[string]string)
	headerBytes, headerCount := cursor, 0
	contentLength, hasContentLength := 0, false
	for {
		if cursor-headerBytes > stompMaxHeaderBytes || cursor >= max {
			return frame, false, protocolError(ErrResourceExceeded, "stomp: headers exceed limit")
		}
		header, next, done, lineErr := parseSTOMPLine(w, cursor, max)
		if lineErr != nil || !done {
			return frame, false, lineErr
		}
		cursor = next
		if len(header) == 0 {
			break
		}
		headerCount++
		if headerCount > stompMaxHeaders {
			return frame, false, protocolError(ErrResourceExceeded, "stomp: too many headers")
		}
		colon := bytes.IndexByte(header, ':')
		if colon <= 0 {
			return frame, false, protocolError(ErrMalformedMessage, "stomp: malformed header")
		}
		keyBytes, valueBytes := header[:colon], header[colon+1:]
		key, value := string(keyBytes), string(valueBytes)
		if frame.command != "CONNECT" && frame.command != "CONNECTED" {
			if bytes.IndexByte(valueBytes, ':') >= 0 {
				return frame, false, protocolError(ErrMalformedMessage, "stomp: unescaped colon in header value")
			}
			key, err = stompDecodeHeader(keyBytes)
			if err != nil {
				return frame, false, err
			}
			value, err = stompDecodeHeader(valueBytes)
			if err != nil {
				return frame, false, err
			}
		} else {
			if !utf8.Valid(keyBytes) || !utf8.Valid(valueBytes) || stompHasControl(keyBytes) || stompHasControl(valueBytes) {
				return frame, false, protocolError(ErrMalformedMessage, "stomp: invalid header encoding")
			}
		}
		if key == "" {
			return frame, false, protocolError(ErrMalformedMessage, "stomp: empty header name")
		}
		if _, exists := frame.headers[key]; !exists {
			frame.headers[key] = value // STOMP 1.2 says the first repeated header wins.
		}
		if key == "content-length" && !hasContentLength {
			contentLength, err = stompParseDecimal(value, max)
			if err != nil {
				return frame, false, err
			}
			hasContentLength = true
		}
	}
	if value, ok := frame.headers["heart-beat"]; ok && !stompValidHeartbeat(value) {
		return frame, false, protocolError(ErrMalformedMessage, "stomp: invalid heart-beat setting")
	}
	if _, ok := frame.headers["heart-beat"]; ok && frame.command != "CONNECT" && frame.command != "STOMP" && frame.command != "CONNECTED" {
		return frame, false, protocolError(ErrMalformedMessage, "stomp: heart-beat is only valid during connection setup")
	}

	bodyAllowed := frame.command == "SEND" || frame.command == "MESSAGE" || frame.command == "ERROR"
	if !bodyAllowed && hasContentLength && contentLength != 0 {
		return frame, false, protocolError(ErrMalformedMessage, "stomp: body is not allowed for this command")
	}
	bodyStart := cursor
	if hasContentLength {
		if bodyStart >= max || contentLength >= max-bodyStart {
			return frame, false, protocolError(ErrResourceExceeded, "stomp: content-length frame exceeds limit")
		}
		terminator := bodyStart + contentLength
		if len(w) <= terminator {
			return frame, false, nil
		}
		if w[terminator] != 0 {
			return frame, false, protocolError(ErrMalformedMessage, "stomp: content-length body is not followed by NUL")
		}
		frame.body = append([]byte(nil), w[bodyStart:terminator]...)
		frame.total = terminator + 1
		return frame, true, nil
	}
	if !bodyAllowed {
		if len(w) <= bodyStart {
			return frame, false, nil
		}
		if w[bodyStart] != 0 {
			return frame, false, protocolError(ErrMalformedMessage, "stomp: body is not allowed for this command")
		}
		frame.total = bodyStart + 1
		return frame, true, nil
	}
	rest := w[bodyStart:]
	if len(rest) > max-bodyStart {
		rest = rest[:max-bodyStart]
	}
	terminator := bytes.IndexByte(rest, 0)
	if terminator < 0 {
		if len(w) >= max {
			return frame, false, protocolError(ErrResourceExceeded, "stomp: unterminated body exceeds limit")
		}
		return frame, false, nil
	}
	frame.body = append([]byte(nil), rest[:terminator]...)
	frame.total = bodyStart + terminator + 1
	return frame, true, nil
}

func stompHasControl(v []byte) bool {
	for _, r := range string(v) {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func stompValidHeartbeat(v string) bool {
	parts := strings.Split(v, ",")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i := range part {
			if part[i] < '0' || part[i] > '9' {
				return false
			}
		}
		if _, err := strconv.ParseUint(part, 10, 63); err != nil {
			return false
		}
	}
	return true
}

func stompInitialProbeHeaders(frame stompFrame) bool {
	// A 1.1/1.2 handshake needs both headers. In particular, accept-version
	// alone is not a safe signature: it also appears in malformed corpus data.
	if frame.headers["host"] == "" {
		return false
	}
	return stompHasVersionOffer(frame.headers["accept-version"], "1.1", "1.2")
}

func stompHasVersionOffer(value string, supported ...string) bool {
	for _, offered := range strings.Split(value, ",") {
		for _, version := range supported {
			if offered == version {
				return true
			}
		}
	}
	return false
}

func stompInitialPrefix(w []byte) bool {
	if len(w) == 0 {
		return false
	}
	for _, command := range []string{"CONNECT", "STOMP"} {
		if len(w) <= len(command) && bytes.Equal(w, []byte(command[:len(w)])) {
			return true
		}
		if bytes.HasPrefix(w, []byte(command)) {
			suffix := w[len(command):]
			if len(suffix) == 0 || bytes.Equal(suffix, []byte("\r")) {
				return true
			}
		}
		if bytes.HasPrefix(w, []byte(command+"\n")) || bytes.HasPrefix(w, []byte(command+"\r\n")) {
			return true
		}
	}
	return false
}

func probeSTOMP(w []byte, limit int) ProbeResult {
	if limit <= 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) > limit {
		w = w[:limit]
	}
	frame, complete, err := parseSTOMPFrame(w, limit)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
	}
	if !complete {
		if stompInitialPrefix(w) {
			if len(w) < limit {
				return ProbeResult{Verdict: ProbeNeedMore, Protocol: "stomp", NeedBytes: 1, Confidence: 45, Reason: "incomplete STOMP CONNECT/STOMP frame"}
			}
		}
		return ProbeResult{Verdict: ProbeReject, Reason: "no complete STOMP handshake"}
	}
	if (frame.command != "CONNECT" && frame.command != "STOMP") || frame.total > limit || !stompInitialProbeHeaders(frame) {
		return ProbeResult{Verdict: ProbeReject, Reason: "not a bounded STOMP handshake"}
	}
	return probeAccept("stomp", "", 96)
}

func (f *binFlow) frameSTOMP(dir int, w []byte) (int, *binSpec, error) {
	frame, complete, err := parseSTOMPFrame(w, f.a.config.MaxMessageBytes)
	if err != nil {
		return 0, nil, err
	}
	if !complete {
		return 0, nil, nil
	}
	if stompWrongDirection(frame.command, dir, f.stomp.clientDir) {
		return 0, nil, protocolError(ErrMalformedMessage, "stomp: command on wrong direction")
	}
	entry := frame.command
	if frame.heartbeat {
		entry = "Heart-beat"
	}
	return frame.total, &binSpec{entry: entry}, nil
}

func (s *binSTOMP) consume(dir int, wire []byte, max int) (map[string]any, error) {
	frame, complete, err := parseSTOMPFrame(wire, max)
	if err != nil {
		return nil, err
	}
	if !complete || frame.total != len(wire) {
		return nil, protocolError(ErrMalformedMessage, "stomp: event does not contain exactly one complete frame")
	}
	if stompWrongDirection(frame.command, dir, s.clientDir) {
		return nil, protocolError(ErrMalformedMessage, "stomp: command on wrong direction")
	}
	if v := frame.headers["version"]; (frame.command == "CONNECTED") && (v == "1.1" || v == "1.2") {
		s.version = v
	}
	headers := make(map[string]any, len(frame.headers))
	for key, value := range frame.headers {
		if key == "passcode" {
			headers[key] = "[redacted]"
		} else {
			headers[key] = value
		}
	}
	return map[string]any{
		"Command": frame.command, "Headers": headers,
		"Body": append([]byte(nil), frame.body...), "Heartbeat": frame.heartbeat,
		"Version": s.version,
	}, nil
}

func (f *binFlow) consumeSTOMP(dir int, e *ProtocolEvent) error {
	if f.stomp == nil {
		return fmt.Errorf("stomp: session was not observed")
	}
	fields, err := f.stomp.consume(dir, e.Raw, f.a.config.MaxMessageBytes)
	if err != nil {
		return err
	}
	e.Session = fields
	e.semanticFields = cloneSession(fields)
	return nil
}
