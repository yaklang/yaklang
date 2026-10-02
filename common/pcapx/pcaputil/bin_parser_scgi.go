package pcaputil

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

const (
	scgiHeaderLimit = 32 << 10
	scgiBodyLimit   = 1 << 20
)

type scgiRequest struct {
	method string
	uri    string
}

type scgiMessage struct {
	request     bool
	method      string
	uri         string
	status      int
	statusLine  string
	contentType string
	contentLen  int
	body        []byte
}

type binSCGI struct {
	clientDir  int
	pending    []scgiRequest
	maxPending int
}

var errSCGINeedMore = errors.New("scgi: incomplete message")

func probeSCGIRequest(wire []byte) bool {
	message, n, err := parseSCGIRequest(wire, 1<<20)
	return err == nil && n > 0 && message.request
}

func scgiRequestNeedsMore(wire []byte) bool {
	_, _, err := parseSCGIRequest(wire, 1<<20)
	return errors.Is(err, errSCGINeedMore)
}

func (f *binFlow) frameSCGI(dir, clientDir int, wire []byte) (int, *binSpec, error) {
	var n int
	var err error
	if dir == clientDir {
		_, n, err = parseSCGIRequest(wire, f.a.config.MaxMessageBytes)
	} else {
		_, n, err = parseSCGIResponse(wire, f.a.config.MaxMessageBytes)
	}
	if err == errSCGINeedMore {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	if n <= 0 {
		return 0, nil, protocolError(ErrMalformedMessage, "SCGI parser returned an empty message")
	}
	return n, &binSpec{}, nil
}

func (s *binSCGI) consume(dir int, wire []byte, maxMessage, maxPending int) (map[string]any, error) {
	if dir == s.clientDir {
		message, n, err := parseSCGIRequest(wire, maxMessage)
		if err != nil {
			return nil, err
		}
		if n != len(wire) {
			return nil, protocolError(ErrMalformedMessage, "SCGI request frame does not match the delivered message")
		}
		if s.maxPending <= 0 {
			s.maxPending = maxPending
		}
		if len(s.pending) >= s.maxPending {
			return nil, protocolError(ErrResourceExceeded, "SCGI pending response budget exceeded")
		}
		s.pending = append(s.pending, scgiRequest{method: message.method, uri: message.uri})
		fields := map[string]any{
			"Role":           "request",
			"Request Method": message.method,
			"Request URI":    message.uri,
			"Content Length": message.contentLen,
		}
		if printableSCGIBytes(message.body) {
			fields["Body"] = string(message.body)
		}
		return fields, nil
	}
	message, n, err := parseSCGIResponse(wire, maxMessage)
	if err != nil {
		return nil, err
	}
	if n != len(wire) {
		return nil, protocolError(ErrMalformedMessage, "SCGI response frame does not match the delivered message")
	}
	if len(s.pending) == 0 {
		return nil, sessionContext("SCGI response has no observed request")
	}
	request := s.pending[0]
	s.pending = s.pending[1:]
	fields := map[string]any{
		"Role":           "response",
		"Status Code":    message.status,
		"Status Line":    message.statusLine,
		"Content Length": message.contentLen,
		"In Reply To":    request.method,
		"Request URI":    request.uri,
	}
	if message.contentType != "" {
		fields["Content Type"] = message.contentType
	}
	if printableSCGIBytes(message.body) {
		fields["Body"] = string(message.body)
	}
	return fields, nil
}

func parseSCGIRequest(wire []byte, maxMessage int) (scgiMessage, int, error) {
	payload, netstringLen, incomplete, err := parseSCGINetstring(wire, scgiHeaderLimit)
	if err != nil {
		return scgiMessage{}, 0, err
	}
	if incomplete {
		return scgiMessage{}, 0, errSCGINeedMore
	}
	headers, err := parseSCGIEnv(payload)
	if err != nil {
		return scgiMessage{}, 0, err
	}
	if headers["SCGI"] != "1" {
		return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI request is missing SCGI=1")
	}
	contentLen, err := parseSCGIContentLength(headers["CONTENT_LENGTH"], maxMessage)
	if err != nil {
		return scgiMessage{}, 0, err
	}
	method, uri := headers["REQUEST_METHOD"], headers["REQUEST_URI"]
	if !validSCGIMethod(method) || !validSCGIURI(uri) {
		return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI request method or URI is invalid")
	}
	total := netstringLen + contentLen
	if total > maxMessage {
		return scgiMessage{}, 0, protocolError(ErrResourceExceeded, "SCGI request exceeds parser limit")
	}
	if len(wire) < total {
		return scgiMessage{}, 0, errSCGINeedMore
	}
	return scgiMessage{request: true, method: method, uri: uri, contentLen: contentLen, body: wire[netstringLen:total]}, total, nil
}

func parseSCGINetstring(wire []byte, maxHeader int) ([]byte, int, bool, error) {
	colon := bytes.IndexByte(wire, ':')
	if colon < 0 {
		if len(wire) == 0 || len(wire) > 10 {
			return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring length prefix is invalid")
		}
		for _, c := range wire {
			if c < '0' || c > '9' {
				return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring length prefix is not decimal")
			}
		}
		return nil, 0, true, nil
	}
	if colon == 0 || colon > 10 {
		return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring length prefix has invalid size")
	}
	lengthText := wire[:colon]
	for _, c := range lengthText {
		if c < '0' || c > '9' {
			return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring length prefix is not decimal")
		}
	}
	if len(lengthText) > 1 && lengthText[0] == '0' {
		return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring length prefix has a leading zero")
	}
	length, err := strconv.Atoi(string(lengthText))
	if err != nil || length > maxHeader {
		return nil, 0, false, protocolError(ErrResourceExceeded, "SCGI netstring header exceeds parser limit")
	}
	end := colon + 1 + length
	if end < colon || end >= len(wire) {
		return nil, 0, true, nil
	}
	if wire[end] != ',' {
		return nil, 0, false, protocolError(ErrMalformedMessage, "SCGI netstring is missing its trailing comma")
	}
	return wire[colon+1 : end], end + 1, false, nil
}

func parseSCGIEnv(payload []byte) (map[string]string, error) {
	if len(payload) == 0 || len(payload) > scgiHeaderLimit || payload[len(payload)-1] != 0 {
		return nil, protocolError(ErrMalformedMessage, "SCGI environment block is empty or missing its trailing NUL")
	}
	parts := bytes.Split(payload, []byte{0})
	if len(parts) < 3 || len(parts)%2 != 1 || len(parts[len(parts)-1]) != 0 {
		return nil, protocolError(ErrMalformedMessage, "SCGI environment block must contain complete NUL-terminated name/value pairs")
	}
	if !bytes.Equal(parts[0], []byte("CONTENT_LENGTH")) {
		return nil, protocolError(ErrMalformedMessage, "SCGI CONTENT_LENGTH must be the first environment variable")
	}
	out := make(map[string]string, (len(parts)-1)/2)
	for i := 0; i+1 < len(parts)-1; i += 2 {
		name, value := parts[i], parts[i+1]
		if len(name) == 0 || !validSCGIEnvName(name) || !printableSCGIBytes(value) {
			return nil, protocolError(ErrMalformedMessage, "SCGI environment variable is invalid")
		}
		key := string(name)
		if _, exists := out[key]; exists {
			return nil, protocolError(ErrMalformedMessage, "SCGI environment variable is duplicated")
		}
		out[key] = string(value)
	}
	if _, ok := out["CONTENT_LENGTH"]; !ok {
		return nil, protocolError(ErrMalformedMessage, "SCGI CONTENT_LENGTH is required")
	}
	return out, nil
}

func validSCGIEnvName(name []byte) bool {
	for _, c := range name {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func parseSCGIContentLength(value string, maxMessage int) (int, error) {
	if value == "" || len(value) > 10 {
		return 0, protocolError(ErrMalformedMessage, "SCGI CONTENT_LENGTH is invalid")
	}
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return 0, protocolError(ErrMalformedMessage, "SCGI CONTENT_LENGTH is not decimal")
		}
	}
	length, err := strconv.Atoi(value)
	if err != nil || length > maxMessage || length > scgiBodyLimit {
		return 0, protocolError(ErrResourceExceeded, "SCGI body exceeds parser limit")
	}
	return length, nil
}

func validSCGIMethod(method string) bool {
	if method == "" || len(method) > 32 {
		return false
	}
	for i := range method {
		c := method[i]
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))) {
			return false
		}
	}
	return true
}

func validSCGIURI(uri string) bool {
	if uri == "" || len(uri) > 4096 || uri[0] != '/' {
		return false
	}
	for i := range uri {
		if uri[i] < 0x21 || uri[i] > 0x7e {
			return false
		}
	}
	return true
}

func parseSCGIResponse(wire []byte, maxMessage int) (scgiMessage, int, error) {
	end := bytes.Index(wire, []byte("\r\n\r\n"))
	if end < 0 {
		if len(wire) > scgiHeaderLimit {
			return scgiMessage{}, 0, protocolError(ErrResourceExceeded, "SCGI response headers exceed parser limit")
		}
		return scgiMessage{}, 0, errSCGINeedMore
	}
	if end+4 > scgiHeaderLimit {
		return scgiMessage{}, 0, protocolError(ErrResourceExceeded, "SCGI response headers exceed parser limit")
	}
	lines := bytes.Split(wire[:end], []byte("\r\n"))
	if len(lines) < 2 || !bytes.HasPrefix(lines[0], []byte("Status: ")) {
		return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI response must begin with a Status header")
	}
	statusLine := string(lines[0])
	statusText := strings.TrimPrefix(statusLine, "Status: ")
	if len(statusText) < 3 || statusText[0] < '1' || statusText[0] > '5' || statusText[1] < '0' || statusText[1] > '9' || statusText[2] < '0' || statusText[2] > '9' || len(statusText) > 3 && statusText[3] != ' ' {
		return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI Status header has an invalid status code")
	}
	status, _ := strconv.Atoi(statusText[:3])
	headers := make(map[string]string, len(lines)-1)
	for _, line := range lines[1:] {
		colon := bytes.Index(line, []byte(": "))
		if colon <= 0 || !validSCGIResponseHeaderName(line[:colon]) || !printableSCGIBytes(line[colon+2:]) {
			return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI response header is malformed")
		}
		name := strings.ToLower(string(line[:colon]))
		if _, exists := headers[name]; exists {
			return scgiMessage{}, 0, protocolError(ErrMalformedMessage, "SCGI response header is duplicated")
		}
		headers[name] = string(line[colon+2:])
	}
	contentLenText, ok := headers["content-length"]
	if !ok {
		return scgiMessage{}, 0, protocolError(ErrUnsupportedFeature, "SCGI response without Content-Length is not supported")
	}
	contentLen, err := parseSCGIContentLength(contentLenText, maxMessage)
	if err != nil {
		return scgiMessage{}, 0, err
	}
	total := end + 4 + contentLen
	if total > maxMessage {
		return scgiMessage{}, 0, protocolError(ErrResourceExceeded, "SCGI response exceeds parser limit")
	}
	if len(wire) < total {
		return scgiMessage{}, 0, errSCGINeedMore
	}
	return scgiMessage{
		status:      status,
		statusLine:  statusLine,
		contentType: headers["content-type"],
		contentLen:  contentLen,
		body:        wire[end+4 : total],
	}, total, nil
}

func validSCGIResponseHeaderName(name []byte) bool {
	if len(name) == 0 {
		return false
	}
	for _, c := range name {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))) {
			return false
		}
	}
	return true
}

func printableSCGIBytes(value []byte) bool {
	for _, c := range value {
		if c != '\t' && (c < 0x20 || c > 0x7e) {
			return false
		}
	}
	return true
}
