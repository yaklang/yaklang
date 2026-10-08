package stream_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"
)

const mimeFieldsMaxParts = 64
const mimeFieldsMaxDepth = 8
const mimeFieldsMaxDecoded = 1 << 20

// MIME values supplement the unchanged physical line tree. All ranges refer
// to encoded original response bytes, even across removed transparency dots.
// Decoded bodies are derived values, not wire leaves. Nothing is rendered,
// fetched, executed or persisted outside the returned parser result.
type mimeFieldsReader struct {
	wire     []byte
	origins  []int
	parts    []map[string]any
	decoded  int
	complete bool
}

func (r *mimeFieldsReader) ranges(start, end int) [][2]int {
	spans := make([][2]int, 0)
	for at := start; at < end; {
		next := at + 1
		for next < end && r.origins[next] == r.origins[next-1]+1 {
			next++
		}
		spans = append(spans, [2]int{r.origins[at], r.origins[next-1] + 1})
		at = next
	}
	return spans
}

func (r *mimeFieldsReader) headers(start, end int) ([]map[string]any, map[string][]string, int, error) {
	headers := make([]map[string]any, 0)
	values := map[string][]string{}
	at := start
	for at < end {
		n := bytes.Index(r.wire[at:end], []byte("\r\n"))
		if n < 0 {
			return nil, nil, 0, fmt.Errorf("mime-fields: incomplete header line")
		}
		lineEnd := at + n
		if lineEnd == at {
			return headers, values, at + 2, nil
		}
		if len(headers) >= 1024 {
			return nil, nil, 0, fmt.Errorf("mime-fields: header resource limit")
		}
		colon := bytes.IndexByte(r.wire[at:lineEnd], ':')
		if colon <= 0 {
			return nil, nil, 0, fmt.Errorf("mime-fields: invalid header name")
		}
		colon += at
		for _, c := range r.wire[at:colon] {
			if c < 33 || c > 126 {
				return nil, nil, 0, fmt.Errorf("mime-fields: invalid header character")
			}
		}
		name, start := string(r.wire[at:colon]), at
		var unfolded strings.Builder
		valueRanges := make([][2]int, 0)
		valueAt := colon + 1
		for {
			for _, c := range r.wire[valueAt:lineEnd] {
				if c != '\t' && (c < 32 || c > 126) {
					return nil, nil, 0, fmt.Errorf("mime-fields: invalid header value")
				}
			}
			unfolded.Write(r.wire[valueAt:lineEnd])
			valueRanges = append(valueRanges, r.ranges(valueAt, lineEnd)...)
			at = lineEnd + 2
			if at >= end || r.wire[at] != ' ' && r.wire[at] != '\t' {
				break
			}
			valueAt = at
			n = bytes.Index(r.wire[at:end], []byte("\r\n"))
			if n < 0 {
				return nil, nil, 0, fmt.Errorf("mime-fields: incomplete folded header")
			}
			lineEnd = at + n
		}
		value := unfolded.String()
		key := strings.ToLower(name)
		values[key] = append(values[key], value)
		headers = append(headers, map[string]any{"Name": name, "Unfolded Value": value, "Encoded Relative Byte Ranges": r.ranges(start, at), "Value Encoded Relative Byte Ranges": valueRanges})
	}
	return headers, values, at, nil
}

func (r *mimeFieldsReader) entity(start, end, depth int, path, defaultType string) error {
	if depth > mimeFieldsMaxDepth || len(r.parts) >= mimeFieldsMaxParts {
		return fmt.Errorf("mime-fields: part/depth resource limit")
	}
	headers, values, bodyAt, err := r.headers(start, end)
	if err != nil {
		return err
	}
	for _, key := range []string{"content-type", "content-transfer-encoding", "content-disposition"} {
		if len(values[key]) > 1 {
			return fmt.Errorf("mime-fields: duplicate interpretation header")
		}
	}
	get := func(key string) string {
		if len(values[key]) == 0 {
			return ""
		}
		return strings.TrimSpace(values[key][0])
	}
	media, params := defaultType, map[string]string{}
	if value := get("content-type"); value != "" {
		media, params, err = mime.ParseMediaType(value)
		if err != nil || strings.Count(media, "/") != 1 {
			return fmt.Errorf("mime-fields: invalid media type")
		}
	}
	encoding := strings.ToLower(get("content-transfer-encoding"))
	if encoding == "" {
		encoding = "7bit"
	}
	part := map[string]any{
		"Path": path, "Media Type": media, "Media Parameters": params, "Transfer Encoding": encoding, "Headers": headers,
		"Encoded Relative Byte Ranges": r.ranges(start, end), "Body Encoded Relative Byte Ranges": r.ranges(bodyAt, end),
		"Body Transfer Decoded": false, "Text Available": false, "Content Rendered": false,
		"Transfer Sender Conformance Validated": false,
	}
	if value := get("content-disposition"); value != "" {
		kind, params, err := mime.ParseMediaType(value)
		if err != nil {
			return fmt.Errorf("mime-fields: invalid disposition")
		}
		part["Disposition"] = kind
		part["Disposition Parameters"] = params
	}
	r.parts = append(r.parts, part)
	body := r.wire[bodyAt:end]
	if strings.HasPrefix(media, "multipart/") {
		if encoding != "7bit" && encoding != "8bit" && encoding != "binary" {
			return fmt.Errorf("mime-fields: encoded multipart envelope")
		}
		boundary := params["boundary"]
		if len(boundary) < 1 || len(boundary) > 70 || boundary[len(boundary)-1] == ' ' {
			return fmt.Errorf("mime-fields: invalid multipart boundary")
		}
		for _, c := range []byte(boundary) {
			if !smtpFieldsAlphaNum(c) && !strings.ContainsRune("'()+_,-./:=? ", rune(c)) {
				return fmt.Errorf("mime-fields: invalid boundary character")
			}
		}
		type marker struct {
			start, after int
			close        bool
		}
		var markers []marker
		prefix := []byte("--" + boundary)
		for at := bodyAt; at < end; {
			n := bytes.Index(r.wire[at:end], []byte("\r\n"))
			stop, after := end, end
			if n >= 0 {
				stop, after = at+n, at+n+2
			}
			line := r.wire[at:stop]
			if bytes.HasPrefix(line, prefix) {
				tail := bytes.TrimRight(line[len(prefix):], " \t")
				if len(tail) == 0 || bytes.Equal(tail, []byte("--")) {
					markers = append(markers, marker{at, after, len(tail) == 2})
					if len(markers) > mimeFieldsMaxParts {
						return fmt.Errorf("mime-fields: boundary resource limit")
					}
					if len(tail) == 2 {
						break
					}
				}
			}
			at = after
		}
		if len(markers) < 2 || markers[0].close || !markers[len(markers)-1].close {
			return fmt.Errorf("mime-fields: incomplete multipart delimiters")
		}
		part["Preamble Encoded Relative Byte Ranges"] = r.ranges(bodyAt, markers[0].start)
		part["Epilogue Encoded Relative Byte Ranges"] = r.ranges(markers[len(markers)-1].after, end)
		delimiters := make([][][2]int, 0, len(markers))
		childDefault := "text/plain"
		if media == "multipart/digest" {
			childDefault = "message/rfc822"
		}
		for i, m := range markers {
			markerStart := m.start
			if i > 0 && markerStart >= 2 {
				markerStart -= 2
			}
			delimiters = append(delimiters, r.ranges(markerStart, m.after))
			if m.close {
				break
			}
			childEnd := markers[i+1].start
			if childEnd >= m.after+2 {
				childEnd -= 2
			}
			if err := r.entity(m.after, childEnd, depth+1, fmt.Sprintf("%s.%d", path, i+1), childDefault); err != nil {
				return err
			}
		}
		part["Boundary Encoded Relative Byte Ranges"] = delimiters
		part["Child Count"] = len(markers) - 1
		return nil
	}
	if media == "message/rfc822" && (encoding == "7bit" || encoding == "8bit" || encoding == "binary") {
		part["Child Count"] = 1
		return r.entity(bodyAt, end, depth+1, path+".1", "text/plain")
	}
	var decoded []byte
	switch encoding {
	case "7bit", "8bit", "binary":
		decoded = bytes.Clone(body)
	case "quoted-printable":
		decoded, err = io.ReadAll(io.LimitReader(quotedprintable.NewReader(bytes.NewReader(body)), int64(mimeFieldsMaxDecoded-r.decoded)+1))
	case "base64":
		// MIME ignores non-alphabet octets (RFC 2045 6.8). Preserve their
		// count and original ranges; the transport's SASL profile is stricter.
		filtered := make([]byte, 0, len(body))
		ignored := 0
		for _, c := range body {
			if smtpFieldsAlphaNum(c) || c == '+' || c == '/' || c == '=' {
				filtered = append(filtered, c)
			} else {
				ignored++
			}
		}
		decoded, err = base64.StdEncoding.DecodeString(string(filtered))
		part["Ignored Transfer Octets"] = ignored
	default:
		r.complete = false
		part["Unparsed Reason"] = "unsupported transfer encoding"
		return nil
	}
	if err != nil {
		return fmt.Errorf("mime-fields: invalid transfer encoding")
	}
	if len(decoded) > mimeFieldsMaxDecoded-r.decoded {
		return fmt.Errorf("mime-fields: decoded resource limit")
	}
	r.decoded += len(decoded)
	part["Body Transfer Decoded"] = true
	part["Decoded Body"] = decoded
	part["Decoded Octets"] = len(decoded)
	part["Decoded SHA256"] = fmt.Sprintf("%x", sha256.Sum256(decoded))
	if strings.HasPrefix(media, "text/") {
		charset := strings.ToLower(params["charset"])
		if charset == "" {
			charset = "us-ascii"
		}
		valid := false
		if charset == "utf-8" {
			valid = utf8.Valid(decoded)
		} else if charset == "us-ascii" {
			valid = true
			for _, c := range decoded {
				if c > 127 {
					valid = false
					break
				}
			}
		}
		part["Text Charset"] = charset
		part["Text Available"] = valid
		if valid {
			part["Decoded Text"] = string(decoded)
		}
	}
	return nil
}

func decodeMIMEDotMessage(wire []byte, start int) (map[string]any, error) {
	if start < 0 || start > len(wire) || len(wire) > 1<<20 {
		return nil, fmt.Errorf("mime-fields: invalid message boundary")
	}
	r := &mimeFieldsReader{complete: true}
	for at := start; at < len(wire); {
		n := bytes.Index(wire[at:], []byte("\r\n"))
		if n < 0 {
			return nil, fmt.Errorf("mime-fields: incomplete dot block")
		}
		end := at + n
		if n == 1 && wire[at] == '.' {
			if end+2 != len(wire) {
				return nil, fmt.Errorf("mime-fields: bytes after dot block")
			}
			if err := r.entity(0, len(r.wire), 0, "0", "text/plain"); err != nil {
				return nil, err
			}
			return map[string]any{"Parts": r.parts, "Part Count": len(r.parts), "Decoded Octets": r.decoded, "All Transfers Decoded": r.complete, "Encoded Ranges Refer To": "original response before dot removal", "Content Rendered": false, "External Content Retrieved": false}, nil
		}
		if n > 0 && wire[at] == '.' {
			at++
		}
		for i := at; i < end+2; i++ {
			r.wire = append(r.wire, wire[i])
			r.origins = append(r.origins, i)
		}
		at = end + 2
	}
	return nil, fmt.Errorf("mime-fields: missing dot terminator")
}
