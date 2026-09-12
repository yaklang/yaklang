package stream_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Passive version endpoint syntax, pinned to etcd v3.5.21 and v3.6.0:
// server/etcdserver/api/etcdhttp/{base.go,version.go,version_test.go}
// api/version/version.go. The two/three JSON member layouts report strings;
// they do not prove an installed version, endpoint identity or session state.
type etcdField struct {
	Name, Type string
	Start, End int
	Children   []etcdField
	Value      any
	Decoded    bool
}

func etcdText(name string, start, end int, value any) etcdField {
	return etcdField{Name: name, Type: "raw", Start: start, End: end, Value: value, Decoded: true}
}

func etcdRaw(name string, start, end int) etcdField {
	return etcdField{Name: name, Type: "raw", Start: start, End: end}
}

func decodeEtcdVersions(body []byte, offset int) (etcdField, error) {
	root := etcdField{Name: "Versions", Start: offset, End: offset + len(body)}
	if len(body) == 0 || len(body) > 4096 || !utf8.Valid(body) || !jsonPairedSurrogates(string(body)) {
		return root, fmt.Errorf("etcd: JSON text outside bounded UTF-8 profile")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return root, fmt.Errorf("etcd: version response requires JSON object")
	}
	pos := 0
	seen := map[string]bool{}
	for decoder.More() {
		if len(seen) >= 3 {
			return root, fmt.Errorf("etcd: version member count limit")
		}
		keyStart := int(decoder.InputOffset())
		for keyStart < len(body) && strings.ContainsRune(" \t\r\n,", rune(body[keyStart])) {
			keyStart++
		}
		keyToken, e := decoder.Token()
		key, ok := keyToken.(string)
		if e != nil || !ok {
			return root, fmt.Errorf("etcd: invalid JSON member name")
		}
		if key != "etcdserver" && key != "etcdcluster" && key != "storage" {
			return root, fmt.Errorf("etcd: unknown member outside version response profile")
		}
		if seen[key] {
			return root, fmt.Errorf("etcd: duplicate version member")
		}
		seen[key] = true
		keyEnd := int(decoder.InputOffset())
		valueStart := keyEnd
		for valueStart < len(body) && strings.ContainsRune(" \t\r\n:", rune(body[valueStart])) {
			valueStart++
		}
		valueToken, e := decoder.Token()
		value, ok := valueToken.(string)
		if e != nil || !ok || len(value) == 0 || len(value) > 256 {
			return root, fmt.Errorf("etcd: version member requires 1..256 byte string")
		}
		valueEnd := int(decoder.InputOffset())
		root.Children = append(root.Children, etcdRaw("JSON Delimiters", offset+pos, offset+keyStart),
			etcdText("JSON Member Name", offset+keyStart, offset+keyEnd, key),
			etcdRaw("JSON Colon", offset+keyEnd, offset+valueStart),
			etcdText(key, offset+valueStart, offset+valueEnd, value))
		pos = valueEnd
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') {
		return root, fmt.Errorf("etcd: incomplete version object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return root, fmt.Errorf("etcd: trailing JSON data")
	}
	if !seen["etcdserver"] || !seen["etcdcluster"] {
		return root, fmt.Errorf("etcd: missing etcdserver or etcdcluster member")
	}
	root.Children = append(root.Children, etcdRaw("JSON Delimiters", offset+pos, offset+len(body)))
	return root, nil
}

func decodeEtcdVersionRecord(wire []byte) ([]etcdField, map[string]any, error) {
	fail := func(s string) ([]etcdField, map[string]any, error) { return nil, nil, fmt.Errorf("etcd: %s", s) }
	if len(wire) == 0 || len(wire) > 65536 {
		return fail("exact record size outside 1..65536 bytes")
	}
	end := bytes.Index(wire, []byte("\r\n\r\n"))
	if end < 0 {
		return fail("missing HTTP header terminator")
	}
	lineEnd := bytes.Index(wire, []byte("\r\n"))
	if lineEnd > 4096 {
		return fail("HTTP start line limit")
	}
	for _, b := range wire[:lineEnd] {
		if b < 32 || b > 126 {
			return fail("HTTP start line requires visible ASCII")
		}
	}
	parts := strings.SplitN(string(wire[:lineEnd]), " ", 3)
	if len(parts) != 3 {
		return fail("invalid HTTP start line")
	}
	response := strings.HasPrefix(parts[0], "HTTP/")
	fields := []etcdField{}
	info := map[string]any{"Profile": "Caller-selected etcd version endpoint syntax, v3.5.21/v3.6.0",
		"Server Identity": "Not established", "Session": "No request-response correlation",
		"Version Semantics": "Reported strings only; installed versions not verified"}
	if response {
		if parts[0] != "HTTP/1.1" || parts[1] != "200" {
			return fail("profile requires HTTP/1.1 200 version response")
		}
		fields = append(fields, etcdText("Version", 0, 8, "HTTP/1.1"), etcdRaw("Version Separator", 8, 9),
			etcdText("Status", 9, 12, uint64(200)), etcdRaw("Status Separator", 12, 13),
			etcdText("Reason", 13, lineEnd, parts[2]))
		info["Message"] = "Version Response"
	} else {
		if parts[0] != "GET" || parts[2] != "HTTP/1.1" {
			return fail("profile requires GET target HTTP/1.1")
		}
		if parts[1] != "/version" {
			return fail("version endpoint path must be /version, not /v3/version")
		}
		fields = append(fields, etcdText("Method", 0, 3, "GET"), etcdRaw("Method Separator", 3, 4),
			etcdText("Path", 4, 12, "/version"), etcdRaw("Target Separator", 12, 13), etcdText("Version", 13, lineEnd, "HTTP/1.1"))
		info["Message"] = "Version Request"
		info["Operation"] = "Read version endpoint"
	}
	fields = append(fields, etcdRaw("Start Line End", lineEnd, lineEnd+2))
	headers := etcdField{Name: "HTTP Headers", Start: lineEnd + 2, End: end + 4}
	values := map[string][]string{}
	size := uint64(0)
	for pos, count := lineEnd+2, 0; pos < end+2; count++ {
		if count >= 256 {
			return fail("HTTP header count limit")
		}
		n := bytes.Index(wire[pos:end+2], []byte("\r\n"))
		if n < 0 {
			return fail("incomplete HTTP header line")
		}
		line := string(wire[pos : pos+n])
		colon := strings.IndexByte(line, ':')
		if colon <= 0 || !winrmToken(line[:colon]) {
			return fail("invalid HTTP header name")
		}
		for _, c := range []byte(line[colon+1:]) {
			if c < 32 && c != '\t' || c == 127 {
				return fail("invalid HTTP header value")
			}
		}
		key, val := strings.ToLower(line[:colon]), strings.Trim(line[colon+1:], " \t")
		values[key] = append(values[key], val)
		name := "HTTP " + key
		var value any = val
		if key == "content-length" {
			if val == "" {
				return fail("invalid Content-Length")
			}
			for _, c := range []byte(val) {
				if c < '0' || c > '9' {
					return fail("invalid Content-Length")
				}
			}
			var err error
			size, err = strconv.ParseUint(val, 10, 32)
			if err != nil || size > 4096 {
				return fail("Content-Length outside version body profile")
			}
			name, value = "Content Length", size
		}
		headers.Children = append(headers.Children, etcdText(name, pos, pos+n+2, value))
		pos += n + 2
	}
	for _, key := range []string{"host", "content-length", "content-type", "content-encoding"} {
		if len(values[key]) > 1 {
			return fail("duplicate " + key + " outside exact profile")
		}
	}
	if len(values["transfer-encoding"]) != 0 {
		return fail("transfer coding outside exact profile")
	}
	if len(values["content-encoding"]) != 0 && !strings.EqualFold(values["content-encoding"][0], "identity") {
		return fail("encoded body outside text profile")
	}
	if !utf8.Valid(wire[:end+4]) {
		return fail("non-UTF-8 header outside text profile")
	}
	headers.Children = append(headers.Children, etcdRaw("Header Terminator", end+2, end+4))
	fields = append(fields, headers)
	if !response {
		if len(values["host"]) != 1 || values["host"][0] == "" {
			return fail("HTTP/1.1 request requires one nonempty Host")
		}
		if size != 0 || end+4 != len(wire) {
			return fail("version request profile requires no body or trailing bytes")
		}
		return fields, info, nil
	}
	if len(values["content-length"]) != 1 || int(size) != len(wire)-end-4 {
		return fail("Content-Length does not match exact version response")
	}
	if len(values["content-type"]) != 1 {
		return fail("version response requires application/json")
	}
	media, params, err := mime.ParseMediaType(values["content-type"][0])
	if err != nil || media != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		return fail("version response requires UTF-8 application/json")
	}
	versions, err := decodeEtcdVersions(wire[end+4:], end+4)
	if err != nil {
		return nil, nil, err
	}
	fields = append(fields, versions)
	return fields, info, nil
}
