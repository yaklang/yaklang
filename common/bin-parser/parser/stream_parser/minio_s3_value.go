package stream_parser

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AWS S3 developer guide: sigv4-auth-using-authorization-header.html and
// sig-v4-header-based-auth.html. These are structure checks only, not HMAC,
// canonical-request, payload-digest, credential or endpoint validation.
const minioS3MaxBytes = 1048576
const minioS3MaxHeaders = 256
const minioS3MaxLine = 8192

type minioS3Field struct {
	Name, Type string
	Start, End int
	Info       map[string]any
	Children   []minioS3Field
}

func minioS3Leaf(name, typ string, start, end int) minioS3Field {
	return minioS3Field{Name: name, Type: typ, Start: start, End: end}
}

func minioS3Token(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
			return false
		}
	}
	return true
}

func minioS3LowerHex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

func minioS3ScopeAtom(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, b := range []byte(s) {
		if b <= 32 || b >= 127 || strings.ContainsRune("/,;=\"\\", rune(b)) {
			return false
		}
	}
	return true
}

func minioS3TrimSpan(s string, start int) (string, int, int) {
	left := len(s) - len(strings.TrimLeft(s, " \t"))
	right := len(strings.TrimRight(s, " \t"))
	if right < left {
		right = left
	}
	return s[left:right], start + left, start + right
}

func minioS3Authorization(value string, start int) (minioS3Field, map[string]any, error) {
	f := minioS3Field{Name: "Authorization Value", Start: start, End: start + len(value)}
	space := strings.IndexByte(value, ' ')
	if space < 0 || value[:space] != "AWS4-HMAC-SHA256" {
		return f, nil, fmt.Errorf("s3-sigv4: unsupported authorization algorithm or syntax")
	}
	f.Children = append(f.Children, minioS3Leaf("Algorithm", "string", start, start+space))
	info := map[string]any{"Algorithm": "AWS4-HMAC-SHA256"}
	seen := map[string]bool{}
	for at := space; at < len(value); {
		begin := at
		for at < len(value) && (value[at] == ' ' || value[at] == '\t') {
			at++
		}
		if at > begin {
			f.Children = append(f.Children, minioS3Leaf("Parameter Whitespace", "raw", start+begin, start+at))
		}
		if at == len(value) {
			return f, nil, fmt.Errorf("s3-sigv4: empty authorization parameter")
		}
		end := strings.IndexByte(value[at:], ',')
		if end < 0 {
			end = len(value)
		} else {
			end += at
		}
		param, pstart, pend := minioS3TrimSpan(value[at:end], start+at)
		equals := strings.IndexByte(param, '=')
		if equals <= 0 {
			return f, nil, fmt.Errorf("s3-sigv4: malformed authorization parameter")
		}
		name, nstart, nend := minioS3TrimSpan(param[:equals], pstart)
		v, vstart, vend := minioS3TrimSpan(param[equals+1:], pstart+equals+1)
		if seen[name] {
			return f, nil, fmt.Errorf("s3-sigv4: duplicate authorization parameter %s", name)
		}
		seen[name] = true
		part := minioS3Field{Name: name, Start: pstart, End: pend}
		part.Children = append(part.Children, minioS3Leaf("Parameter Name", "string", nstart, nend), minioS3Leaf("Assignment", "raw", nend, vstart))
		switch name {
		case "Credential":
			components := strings.Split(v, "/")
			if len(components) != 5 {
				return f, nil, fmt.Errorf("s3-sigv4: credential requires key/date/region/service/aws4_request")
			}
			if !minioS3ScopeAtom(components[0]) || !minioS3ScopeAtom(components[2]) || components[3] != "s3" || components[4] != "aws4_request" {
				return f, nil, fmt.Errorf("s3-sigv4: unsupported credential scope")
			}
			if len(components[1]) != 8 {
				return f, nil, fmt.Errorf("s3-sigv4: invalid scope date")
			}
			if _, err := time.Parse("20060102", components[1]); err != nil {
				return f, nil, fmt.Errorf("s3-sigv4: invalid scope date")
			}
			cursor := vstart
			for i, fieldName := range []string{"Access Key ID", "Scope Date", "Region", "Service", "Scope Terminator"} {
				part.Children = append(part.Children, minioS3Leaf(fieldName, "string", cursor, cursor+len(components[i])))
				info[fieldName] = components[i]
				cursor += len(components[i])
				if i < 4 {
					part.Children = append(part.Children, minioS3Leaf("Scope Separator", "raw", cursor, cursor+1))
					cursor++
				}
			}
		case "SignedHeaders":
			names := strings.Split(v, ";")
			if len(names) > minioS3MaxHeaders {
				return f, nil, fmt.Errorf("s3-sigv4: signed-header list exceeds 256-entry profile")
			}
			cursor, previous := vstart, ""
			for i, name := range names {
				if !minioS3Token(name) || name != strings.ToLower(name) || i > 0 && name <= previous {
					return f, nil, fmt.Errorf("s3-sigv4: signed headers must be lowercase sorted unique names")
				}
				part.Children = append(part.Children, minioS3Leaf(fmt.Sprintf("Signed Header %d", i), "string", cursor, cursor+len(name)))
				cursor, previous = cursor+len(name), name
				if i < len(names)-1 {
					part.Children = append(part.Children, minioS3Leaf("Header Separator", "raw", cursor, cursor+1))
					cursor++
				}
			}
			info["Signed Headers"] = names
		case "Signature":
			if !minioS3LowerHex(v) {
				return f, nil, fmt.Errorf("s3-sigv4: signature must be 64 lowercase hexadecimal characters")
			}
			part.Children = append(part.Children, minioS3Leaf("Signature Hex", "string", vstart, vend))
			info["Signature Hex"] = v
		default:
			return f, nil, fmt.Errorf("s3-sigv4: unsupported authorization parameter %s", name)
		}
		if pend > vend {
			part.Children = append(part.Children, minioS3Leaf("Parameter Value Whitespace", "raw", vend, pend))
		}
		f.Children = append(f.Children, part)
		if start+end > pend {
			f.Children = append(f.Children, minioS3Leaf("Parameter Trailing Whitespace", "raw", pend, start+end))
		}
		if end == len(value) {
			break
		}
		f.Children = append(f.Children, minioS3Leaf("Parameter Separator", "raw", start+end, start+end+1))
		at = end + 1
		if at == len(value) {
			return f, nil, fmt.Errorf("s3-sigv4: trailing authorization comma")
		}
	}
	for _, required := range []string{"Credential", "SignedHeaders", "Signature"} {
		if !seen[required] {
			return f, nil, fmt.Errorf("s3-sigv4: missing authorization parameter %s", required)
		}
	}
	return f, info, nil
}

func decodeS3SignatureV4Request(wire []byte) ([]minioS3Field, map[string]any, error) {
	if len(wire) == 0 || len(wire) > minioS3MaxBytes {
		return nil, nil, fmt.Errorf("s3-sigv4: request exceeds 1..1048576-byte profile")
	}
	lineAt := 0
	line := func() ([]byte, int, int, error) {
		start := lineAt
		available := len(wire) - start
		if available > minioS3MaxLine {
			available = minioS3MaxLine
		}
		end := bytes.Index(wire[start:start+available], []byte("\r\n"))
		if end < 0 {
			return nil, 0, 0, fmt.Errorf("s3-sigv4: incomplete or oversized 8192-byte line")
		}
		lineAt = start + end + 2
		return wire[start : start+end], start, start + end, nil
	}
	first, _, end, err := line()
	if err != nil {
		return nil, nil, err
	}
	parts := strings.Split(string(first), " ")
	if len(parts) != 3 || parts[2] != "HTTP/1.1" {
		return nil, nil, fmt.Errorf("s3-sigv4: HTTP/1.1 request line required")
	}
	method, target := parts[0], parts[1]
	if !map[string]bool{"GET": true, "PUT": true, "POST": true, "DELETE": true, "HEAD": true, "OPTIONS": true}[method] || !strings.HasPrefix(target, "/") {
		return nil, nil, fmt.Errorf("s3-sigv4: unsupported request method or non-origin target")
	}
	fields := []minioS3Field{{Name: "Request Line", Start: 0, End: lineAt, Children: []minioS3Field{
		minioS3Leaf("Method", "string", 0, len(method)), minioS3Leaf("Method Separator", "raw", len(method), len(method)+1),
		minioS3Leaf("Request Target", "string", len(method)+1, len(method)+1+len(target)), minioS3Leaf("Target Separator", "raw", len(method)+1+len(target), end-8),
		minioS3Leaf("HTTP Version", "string", end-8, end), minioS3Leaf("Request Line End", "raw", end, lineAt),
	}}}
	headers := map[string][]string{}
	headerFields := minioS3Field{Name: "Headers", Start: lineAt}
	var info map[string]any
	for count := 0; ; count++ {
		value, start, end, err := line()
		if err != nil {
			return nil, nil, err
		}
		if len(value) == 0 {
			headerFields.End = start
			fields = append(fields, headerFields, minioS3Leaf("Header End", "raw", start, lineAt))
			break
		}
		if count == minioS3MaxHeaders {
			return nil, nil, fmt.Errorf("s3-sigv4: header count exceeds 256-entry profile")
		}
		colon := bytes.IndexByte(value, ':')
		if colon <= 0 || !minioS3Token(string(value[:colon])) {
			return nil, nil, fmt.Errorf("s3-sigv4: invalid header name or folded line")
		}
		for _, b := range value[colon+1:] {
			if b < 32 && b != '\t' || b == 127 {
				return nil, nil, fmt.Errorf("s3-sigv4: invalid header value character")
			}
		}
		name := strings.ToLower(string(value[:colon]))
		v, vstart, vend := minioS3TrimSpan(string(value[colon+1:]), start+colon+1)
		headers[name] = append(headers[name], v)
		if len(headers[name]) > 1 && (name == "host" || name == "authorization" || name == "content-length" || name == "x-amz-date" || name == "date" || name == "x-amz-content-sha256" || name == "content-md5") {
			return nil, nil, fmt.Errorf("s3-sigv4: duplicate singleton header %s", name)
		}
		h := minioS3Field{Name: fmt.Sprintf("Header %d", count), Start: start, End: lineAt, Info: map[string]any{"Normalized Name": name}, Children: []minioS3Field{
			minioS3Leaf("Header Name", "string", start, start+colon), minioS3Leaf("Header Assignment", "raw", start+colon, vstart),
		}}
		if name == "authorization" {
			var auth minioS3Field
			auth, info, err = minioS3Authorization(v, vstart)
			if err != nil {
				return nil, nil, err
			}
			h.Children = append(h.Children, auth)
		} else {
			h.Children = append(h.Children, minioS3Leaf("Header Value", "string", vstart, vend))
		}
		if vend < end {
			h.Children = append(h.Children, minioS3Leaf("Header Trailing Whitespace", "raw", vend, end))
		}
		h.Children = append(h.Children, minioS3Leaf("Header Line End", "raw", end, lineAt))
		headerFields.Children = append(headerFields.Children, h)
	}
	if info == nil {
		return nil, nil, fmt.Errorf("s3-sigv4: missing authorization header")
	}
	get := func(name string) string {
		if len(headers[name]) == 0 {
			return ""
		}
		return headers[name][0]
	}
	if get("host") == "" {
		return nil, nil, fmt.Errorf("s3-sigv4: one nonempty Host required")
	}
	if strings.ContainsAny(get("host"), "/?#@\\") {
		return nil, nil, fmt.Errorf("s3-sigv4: invalid Host authority")
	}
	endpoint, err := decodeHTTPServiceURL("http://" + get("host") + target)
	if err != nil {
		return nil, nil, fmt.Errorf("s3-sigv4: invalid authority or request target: %w", err)
	}
	if len(headers["transfer-encoding"]) != 0 {
		return nil, nil, fmt.Errorf("s3-sigv4: streaming/chunked request profile unsupported")
	}
	for _, value := range headers["content-encoding"] {
		for _, coding := range strings.Split(value, ",") {
			if strings.EqualFold(strings.Trim(coding, " \t"), "aws-chunked") {
				return nil, nil, fmt.Errorf("s3-sigv4: streaming/chunked request profile unsupported")
			}
		}
	}
	payloadHash := get("x-amz-content-sha256")
	if payloadHash != "UNSIGNED-PAYLOAD" && !minioS3LowerHex(payloadHash) {
		return nil, nil, fmt.Errorf("s3-sigv4: payload hash must be 64 lowercase hex or UNSIGNED-PAYLOAD; streaming unsupported")
	}
	contentLength := 0
	if len(headers["content-length"]) != 0 {
		v := get("content-length")
		if v == "" {
			return nil, nil, fmt.Errorf("s3-sigv4: invalid Content-Length")
		}
		for _, b := range []byte(v) {
			if b < '0' || b > '9' || contentLength > (minioS3MaxBytes-int(b-'0'))/10 {
				return nil, nil, fmt.Errorf("s3-sigv4: invalid or oversized Content-Length")
			}
			contentLength = contentLength*10 + int(b-'0')
		}
	}
	if len(wire)-lineAt != contentLength {
		return nil, nil, fmt.Errorf("s3-sigv4: body length does not match exact Content-Length boundary")
	}
	fields = append(fields, minioS3Leaf("Payload", "raw", lineAt, len(wire)))
	signed := map[string]bool{}
	for _, name := range info["Signed Headers"].([]string) {
		if name == "authorization" || len(headers[name]) == 0 {
			return nil, nil, fmt.Errorf("s3-sigv4: signed header %s is absent or self-referential", name)
		}
		signed[name] = true
	}
	if !signed["host"] {
		return nil, nil, fmt.Errorf("s3-sigv4: Host must be listed in SignedHeaders")
	}
	for name := range headers {
		if (strings.HasPrefix(name, "x-amz-") && name != "x-amz-content-sha256" || name == "content-md5") && !signed[name] {
			return nil, nil, fmt.Errorf("s3-sigv4: required header %s absent from SignedHeaders", name)
		}
	}
	var timestamp time.Time
	source := "x-amz-date"
	if len(headers[source]) != 0 {
		if len(get(source)) != 16 {
			return nil, nil, fmt.Errorf("s3-sigv4: invalid x-amz-date timestamp")
		}
		timestamp, err = time.Parse("20060102T150405Z", get(source))
	} else {
		source = "date"
		timestamp, err = http.ParseTime(get(source))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("s3-sigv4: missing or invalid request timestamp")
	}
	if timestamp.UTC().Format("20060102") != info["Scope Date"] {
		return nil, nil, fmt.Errorf("s3-sigv4: scope date differs from selected request timestamp")
	}
	info["Profile"] = "S3-compatible Signature V4 HTTP/1.1 single-request structural fields"
	info["Timestamp Header"], info["Timestamp UTC"] = source, timestamp.UTC().Format(time.RFC3339)
	info["Payload Hash Declaration"], info["Payload Digest Declared"] = payloadHash, payloadHash != "UNSIGNED-PAYLOAD"
	info["Payload Bytes"] = contentLength
	info["Authority"], info["Escaped Path"], info["Raw Query"] = get("host"), endpoint["escaped_path"], endpoint["raw_query"]
	info["Maximum Request Bytes"], info["Maximum Header Count"] = minioS3MaxBytes, minioS3MaxHeaders
	for _, name := range []string{"Signature Verified", "Payload Digest Verified", "Canonical Request Constructed", "Credential Validated", "Timestamp Freshness Validated", "Endpoint Product Identified", "Object Operation Outcome Validated"} {
		info[name] = false
	}
	return fields, info, nil
}
