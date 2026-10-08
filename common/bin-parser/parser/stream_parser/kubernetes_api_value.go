package stream_parser

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This is a caller-selected, passive core Namespace collection request profile,
// not endpoint detection or an assertion that a server accepted a request.
// Conversion rules are pinned to Kubernetes v1.35.0:
// staging/src/k8s.io/apimachinery/pkg/apis/meta/v1/{types.go,zz_generated.conversion.go}
// staging/src/k8s.io/apimachinery/pkg/runtime/conversion.go
// The public request path comes from client-go's typed/core/v1/namespace.go
// and gentype/type.go. No selector, credential, object or URI is evaluated.
type kubernetesAPIField struct {
	Name, Type string
	Start, End int
	Children   []kubernetesAPIField
	Value      any
	Decoded    bool
	Info       map[string]any
}

func kubernetesAPIText(name string, start, end int, value any) kubernetesAPIField {
	return kubernetesAPIField{Name: name, Type: "raw", Start: start, End: end, Value: value, Decoded: true}
}

func kubernetesAPIRaw(name string, start, end int) kubernetesAPIField {
	return kubernetesAPIField{Name: name, Type: "raw", Start: start, End: end}
}

func kubernetesAPIOption(key, value string) (any, bool, error) {
	switch key {
	case "watch", "allowWatchBookmarks", "sendInitialEvents":
		// runtime.Convert_Slice_string_To_bool: every present value except
		// case-insensitive false or literal 0 is true, including an empty value.
		return value != "0" && !strings.EqualFold(value, "false"), true, nil
	case "timeoutSeconds", "limit":
		n, err := strconv.ParseInt(value, 10, 64)
		return n, true, err
	case "labelSelector", "fieldSelector", "resourceVersion", "resourceVersionMatch", "continue":
		return value, true, nil
	default:
		return value, false, nil
	}
}

func kubernetesAPIQuery(wire []byte, start, end int) (kubernetesAPIField, map[string]any, error) {
	query := kubernetesAPIField{Name: "Query", Start: start, End: end}
	query.Children = append(query.Children, kubernetesAPIRaw("Query Delimiter", start, start+1))
	effective := map[string]any{}
	seen := map[string]bool{}
	for pos, count := start+1, 0; pos < end; count++ {
		if count >= 256 {
			return query, nil, fmt.Errorf("kubernetes api: query occurrence count limit")
		}
		next := bytes.IndexByte(wire[pos:end], '&')
		if next < 0 {
			next = end - pos
		}
		stop := pos + next
		if stop > pos {
			encoded := string(wire[pos:stop])
			if strings.Contains(encoded, ";") {
				return query, nil, fmt.Errorf("kubernetes api: unescaped semicolon in query")
			}
			eq := strings.IndexByte(encoded, '=')
			nameEnd, valueStart := stop, stop
			if eq >= 0 {
				nameEnd, valueStart = pos+eq, pos+eq+1
			}
			key, err := url.QueryUnescape(string(wire[pos:nameEnd]))
			if err != nil {
				return query, nil, fmt.Errorf("kubernetes api: invalid query name escape")
			}
			value, err := url.QueryUnescape(string(wire[valueStart:stop]))
			if err != nil {
				return query, nil, fmt.Errorf("kubernetes api: invalid query value escape")
			}
			first := !seen[key]
			seen[key] = true
			var decoded any = value
			_, known, _ := kubernetesAPIOption(key, "")
			if first && known {
				decoded, _, err = kubernetesAPIOption(key, value)
				if err != nil {
					return query, nil, fmt.Errorf("kubernetes api: %s first value is not int64", key)
				}
				effective[key] = decoded
			}
			entry := kubernetesAPIField{Name: "Query Parameter", Start: pos, End: stop, Info: map[string]any{
				"Name": key, "Known ListOption": known, "Effective First Value": first && known,
			}}
			entry.Children = append(entry.Children, kubernetesAPIText("Parameter Name", pos, nameEnd, key))
			if eq >= 0 {
				entry.Children = append(entry.Children, kubernetesAPIRaw("Equals", nameEnd, nameEnd+1))
			}
			fieldName := "Parameter Value"
			if first && known {
				fieldName = key
			}
			entry.Children = append(entry.Children, kubernetesAPIText(fieldName, valueStart, stop, decoded))
			query.Children = append(query.Children, entry)
		}
		if stop < end {
			query.Children = append(query.Children, kubernetesAPIRaw("Parameter Delimiter", stop, stop+1))
		}
		pos = stop + 1
	}
	return query, effective, nil
}

func decodeKubernetesAPIRequest(wire []byte) ([]kubernetesAPIField, map[string]any, error) {
	fail := func(s string) ([]kubernetesAPIField, map[string]any, error) {
		return nil, nil, fmt.Errorf("kubernetes api: %s", s)
	}
	if len(wire) == 0 || len(wire) > 65536 {
		return fail("exact request size outside 1..65536 bytes")
	}
	end := bytes.Index(wire, []byte("\r\n\r\n"))
	if end < 0 {
		return fail("missing HTTP header terminator")
	}
	if end+4 != len(wire) {
		return fail("body or trailing bytes outside bodyless request profile")
	}
	lineEnd := bytes.Index(wire, []byte("\r\n"))
	if lineEnd > 16384 {
		return fail("request line limit")
	}
	parts := strings.Split(string(wire[:lineEnd]), " ")
	if len(parts) != 3 || parts[0] != "GET" || parts[2] != "HTTP/1.1" {
		return fail("profile requires GET target HTTP/1.1")
	}
	for _, c := range wire[:lineEnd] {
		if c < 32 || c > 126 {
			return fail("request line requires visible ASCII")
		}
	}
	target := parts[1]
	path, _, _ := strings.Cut(target, "?")
	if path != "/api/v1/namespaces" || strings.Contains(target, "#") {
		return fail("profile requires literal /api/v1/namespaces collection target")
	}
	info := map[string]any{
		"Profile":   "Caller-selected core Namespace collection request syntax, Kubernetes v1.35.0",
		"API Group": "", "API Version": "v1", "Resource": "namespaces", "Scope": "cluster",
		"Operation": "ListNamespace", "Request Target": target,
		"Server Identity": "Not established", "Authentication": "Not verified",
		"Option Semantics": "First-value conversion only; server validation, feature gates and selector matching not evaluated",
		"Response":         "Not decoded", "Unknown Parameters": "Preserved, not interpreted",
	}
	fields := []kubernetesAPIField{kubernetesAPIText("Method", 0, 3, "GET"), kubernetesAPIRaw("Method Separator", 3, 4)}
	pathField := kubernetesAPIField{Name: "Path", Start: 4, End: 4 + len(path), Info: map[string]any{"Value": path}}
	pathField.Children = []kubernetesAPIField{
		kubernetesAPIRaw("Path Separator", 4, 5), kubernetesAPIText("API Prefix", 5, 8, "api"),
		kubernetesAPIRaw("Path Separator", 8, 9), kubernetesAPIText("API Version", 9, 11, "v1"),
		kubernetesAPIRaw("Path Separator", 11, 12), kubernetesAPIText("Resource", 12, 4+len(path), "namespaces"),
	}
	fields = append(fields, pathField)
	if len(target) > len(path) {
		query, opts, err := kubernetesAPIQuery(wire, 4+len(path), 4+len(target))
		if err != nil {
			return nil, nil, err
		}
		fields = append(fields, query)
		info["ListOptions"] = opts
		if watch, _ := opts["watch"].(bool); watch {
			info["Operation"] = "WatchNamespace"
		}
	} else {
		info["ListOptions"] = map[string]any{}
	}
	fields = append(fields, kubernetesAPIRaw("Target Separator", 4+len(target), 5+len(target)),
		kubernetesAPIText("Version", 5+len(target), lineEnd, "HTTP/1.1"), kubernetesAPIRaw("Request Line End", lineEnd, lineEnd+2))
	headers := kubernetesAPIField{Name: "HTTP Headers", Start: lineEnd + 2, End: end + 4}
	values := map[string][]string{}
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
		if key == "authorization" {
			entry := kubernetesAPIField{Name: "Authorization", Start: pos, End: pos + n + 2,
				Info: map[string]any{"Meaning": "Supplied text only; identity and authentication not verified"}}
			begin, finish := pos+colon+1, pos+n
			for begin < finish && (wire[begin] == ' ' || wire[begin] == '\t') {
				begin++
			}
			for finish > begin && (wire[finish-1] == ' ' || wire[finish-1] == '\t') {
				finish--
			}
			entry.Children = append(entry.Children, kubernetesAPIRaw("Authorization Header Prefix", pos, begin))
			schemeEnd := begin
			for schemeEnd < finish && wire[schemeEnd] != ' ' && wire[schemeEnd] != '\t' {
				schemeEnd++
			}
			// A scheme and following bytes are lexical fields only. Unknown or
			// incomplete schemes remain raw rather than being called credentials.
			if schemeEnd > begin && schemeEnd < finish && winrmToken(string(wire[begin:schemeEnd])) {
				entry.Children = append(entry.Children, kubernetesAPIText("Authentication Scheme", begin, schemeEnd, string(wire[begin:schemeEnd])))
				credential := schemeEnd
				for credential < finish && (wire[credential] == ' ' || wire[credential] == '\t') {
					credential++
				}
				entry.Children = append(entry.Children, kubernetesAPIRaw("Authentication Separator", schemeEnd, credential), kubernetesAPIRaw("Credential", credential, finish))
			} else {
				entry.Children = append(entry.Children, kubernetesAPIRaw("Authorization Value", begin, finish))
			}
			entry.Children = append(entry.Children, kubernetesAPIRaw("Authorization Header Suffix", finish, pos+n+2))
			headers.Children = append(headers.Children, entry)
		} else {
			var value any = val
			name := "HTTP " + key
			if key == "content-length" {
				if val == "" || strings.Trim(val, "0") != "" {
					return fail("bodyless profile requires zero Content-Length")
				}
				name, value = "Content Length", uint64(0)
			}
			headers.Children = append(headers.Children, kubernetesAPIText(name, pos, pos+n+2, value))
		}
		pos += n + 2
	}
	for _, key := range []string{"host", "authorization", "content-length"} {
		if len(values[key]) > 1 {
			return fail("duplicate " + key + " outside exact profile")
		}
	}
	if len(values["host"]) != 1 || values["host"][0] == "" {
		return fail("HTTP/1.1 request requires one nonempty Host")
	}
	if len(values["transfer-encoding"]) != 0 {
		return fail("transfer coding outside bodyless profile")
	}
	// UTF-8 is a profile boundary, not an assumption about a peer's identity.
	if !utf8.Valid(wire) {
		return fail("non-UTF-8 header outside text profile")
	}
	headers.Children = append(headers.Children, kubernetesAPIRaw("Header Terminator", end+2, end+4))
	fields = append(fields, headers)
	return fields, info, nil
}
