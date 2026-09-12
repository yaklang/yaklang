package parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
)

// These adapters implement the complete field projection of specific
// embedded rules. Pin the ENTIRE rule, not just an operator fragment. A changed
// rule or a custom default parser automatically retains normal interpretation.
var captureStructuredRules = sync.OnceValue(func() map[string]bool {
	eligible := make(map[string]bool)
	for name, want := range map[string]string{
		"http":      "e356b5b5c242f5d3b29a03a92a37795137036448162223253997d14bce14ea04",
		"tls":       "be13dbbbb55ff16bd7127c680e4f7cae22a7e826465390051b5a22e55c982bd2",
		"tls_hello": "f9809596ebeddc2ef3d1c8272aee12fbf202918bc73d46ade66b3df5f92be9d9",
		"dns":       "f000aa95e95dda7288d4a95c1c65500e29330ba61cb36096982696e6ba2b48c2",
	} {
		wire, err := rules.RuleFS.ReadFile("application-layer/" + name + ".yaml")
		if err == nil {
			eligible[name] = fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(wire, []byte("\r\n"), []byte("\n")))) == want
		}
	}
	return eligible
})

// ParseStructuredWithConfig is the bounded structured counterpart of
// ParseBinaryWithConfig. The HTTP adapter accepts only the three documented
// framing inputs; other configuration and custom parsers use the normal path.
// Failed native validation also retains the original rule's diagnostics.
func ParseStructuredWithConfig(data []byte, rule string, config map[string]any, keys ...string) (map[string]any, error) {
	if captureStructuredRules()["http"] && rule == "application-layer.http" && len(keys) == 1 && keys[0] == "HTTPExact" && base.ParserRegistration("default") == defaultParserRegistration {
		if value, ok, err := captureHTTPStructured(data, config); ok && err == nil {
			return value, nil
		}
	}
	r := &structuredReader{Reader: bytes.NewReader(data), bits: uint64(len(data)) * 8}
	node, err := ParseBinaryWithConfig(r, rule, config, keys...)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("structured parse: %d unconsumed message bytes", r.Len())
	}
	return map[string]any{"fields": stream_parser.NodeToMap(node), "metadata": node.Cfg.GetItem("additionInfo")}, nil
}

func captureTLSStructured(w []byte) (map[string]any, bool) {
	if len(w) == 0 {
		return nil, false
	}
	var records []any
	for len(w) > 0 {
		if len(w) < 5 {
			return nil, false
		}
		n := int(binary.BigEndian.Uint16(w[3:5]))
		if n > len(w)-5 {
			return nil, false
		}
		r := map[string]any{"ContentType": w[0], "Version": binary.BigEndian.Uint16(w[1:3]), "Length": uint16(n)}
		var hello map[string]any
		if w[0] == 22 {
			if !captureStructuredRules()["tls_hello"] || n == 0 {
				return nil, false
			}
			if w[5] == 1 {
				var ok bool
				hello, ok = captureTLSClientHello(w[5 : 5+n])
				if !ok {
					return nil, false
				}
			}
		}
		if hello != nil {
			r["TLSClientHello"] = hello
		} else if n > 0 {
			r["Payload"] = append([]byte(nil), w[5:5+n]...)
		}
		records = append(records, map[string]any{"Record Layer": r})
		w = w[5+n:]
	}
	return map[string]any{"fields": map[string]any{"Package": records}, "metadata": nil}, true
}

func captureHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && (b < '0' || b > '9') && !strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b)) {
			return false
		}
	}
	return true
}
func captureHTTPValue(s string) bool {
	for _, b := range []byte(s) {
		if (b < 32 && b != 9) || b == 127 {
			return false
		}
	}
	return true
}

func captureHTTPStructured(data []byte, config map[string]any) (map[string]any, bool, error) {
	limit, method, closeBody := 16777216, "", false
	for key, value := range config {
		switch key {
		case "httpBodyLimit":
			v, ok := value.(int)
			if !ok {
				return nil, false, nil
			}
			limit = v
		case "httpResponseToMethod":
			v, ok := value.(string)
			if !ok {
				return nil, false, nil
			}
			method = v
		case "httpCloseDelimited":
			v, ok := value.(bool)
			if !ok {
				return nil, false, nil
			}
			closeBody = v
		default:
			return nil, false, nil
		}
	}
	fail := func() (map[string]any, bool, error) {
		return nil, true, fmt.Errorf("HTTP message does not match admitted exact rule")
	}
	if limit <= 0 {
		return fail()
	}
	// A single owned string backs immutable string leaves; input is never retained.
	w := string(data)
	lineEnd := strings.Index(w, "\r\n")
	if lineEnd < 0 {
		return fail()
	}
	first := strings.SplitN(w[:lineEnd], " ", 3)
	if len(first) != 3 {
		return fail()
	}
	response := strings.HasPrefix(first[0], "HTTP/")
	var firstLine map[string]any
	noBody := false
	name := "HTTP Request"
	if response {
		name = "HTTP Response"
		if (first[0] != "HTTP/1.0" && first[0] != "HTTP/1.1") || len(first[1]) != 3 || !captureHTTPValue(first[2]) {
			return fail()
		}
		for _, b := range []byte(first[1]) {
			if b < '0' || b > '9' {
				return fail()
			}
		}
		status, _ := strconv.Atoi(first[1])
		if status < 100 {
			return fail()
		}
		noBody = status < 200 || status == 204 || status == 304 || method == "HEAD" || (method == "CONNECT" && status >= 200 && status < 300)
		firstLine = map[string]any{"Version": first[0], "Status": first[1], "Message": first[2]}
	} else {
		if !captureHTTPToken(first[0]) || first[1] == "" || (first[2] != "HTTP/1.0" && first[2] != "HTTP/1.1") {
			return fail()
		}
		for _, b := range []byte(first[1]) {
			if b <= 32 || b == 127 {
				return fail()
			}
		}
		firstLine = map[string]any{"Method": first[0], "Path": first[1], "Version": first[2]}
	}
	pos := lineEnd + 2
	var headers []any
	hasLength, chunked := false, false
	length := 0
	for {
		end := strings.Index(w[pos:], "\r\n")
		if end < 0 {
			return fail()
		}
		line := w[pos : pos+end]
		pos += end + 2
		headers = append(headers, line)
		if line == "" {
			break
		}
		hname, value, ok := strings.Cut(line, ":")
		if !ok || !captureHTTPToken(hname) || !captureHTTPValue(value) {
			return fail()
		}
		value = strings.Trim(value, " \t")
		switch strings.ToLower(hname) {
		case "content-length":
			if value == "" {
				return fail()
			}
			for _, b := range []byte(value) {
				if b < '0' || b > '9' {
					return fail()
				}
			}
			n, err := strconv.ParseUint(value, 10, 63)
			if err != nil || n > uint64(limit) || (hasLength && int(n) != length) {
				return fail()
			}
			hasLength, length = true, int(n)
		case "transfer-encoding":
			if chunked || strings.ToLower(value) != "chunked" {
				return fail()
			}
			chunked = true
		}
	}
	if hasLength && chunked {
		return fail()
	}
	message := map[string]any{"FirstLine": firstLine, "Headers": headers}
	if !noBody {
		if chunked {
			var chunks []any
			total := 0
			for {
				end := strings.Index(w[pos:], "\r\n")
				if end < 0 {
					return fail()
				}
				line := w[pos : pos+end]
				pos += end + 2
				sizeText, _, _ := strings.Cut(line, ";")
				sizeText = strings.Trim(sizeText, " \t")
				if sizeText == "" {
					return fail()
				}
				for _, b := range []byte(sizeText) {
					if (b < '0' || b > '9') && (b < 'a' || b > 'f') && (b < 'A' || b > 'F') {
						return fail()
					}
				}
				size, err := strconv.ParseUint(sizeText, 16, 63)
				if err != nil || size > uint64(limit-total) {
					return fail()
				}
				chunk := map[string]any{"Size": line}
				if size == 0 {
					var trailers []any
					for {
						end := strings.Index(w[pos:], "\r\n")
						if end < 0 {
							return fail()
						}
						line := w[pos : pos+end]
						pos += end + 2
						trailers = append(trailers, line)
						if line == "" {
							break
						}
						n, v, ok := strings.Cut(line, ":")
						if !ok || !captureHTTPToken(n) || !captureHTTPValue(v) || strings.EqualFold(n, "content-length") || strings.EqualFold(n, "transfer-encoding") {
							return fail()
						}
					}
					chunk["Trailers"] = trailers
					chunks = append(chunks, chunk)
					break
				}
				n := int(size)
				if n > len(w)-pos-2 || w[pos+n:pos+n+2] != "\r\n" {
					return fail()
				}
				chunk["Octets"], chunk["End"] = w[pos:pos+n], ""
				pos += n + 2
				total += n
				chunks = append(chunks, chunk)
			}
			message["Body"] = map[string]any{"DataChunks": chunks}
		} else {
			if response && !hasLength && closeBody {
				length = len(w) - pos
			}
			if length > limit || length > len(w)-pos {
				return fail()
			}
			if length > 0 {
				message["Body"] = map[string]any{"Octets": w[pos : pos+length]}
				pos += length
			}
		}
	}
	if pos != len(w) {
		return fail()
	}
	return map[string]any{"fields": map[string]any{"Message": map[string]any{name: message}}, "metadata": nil}, true, nil
}
