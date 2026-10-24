package stream_parser

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// UDA 2.0 discovery core: exact bodyless UDP message, ordered extension
// headers and unverified device assertions. No HTTP GET of LOCATION is made.
// Syntax/identity evidence is separate from optional UDA conformance warnings.
const ssdpMaxBytes = 65507

type ssdpField = etcdField

func decodeSSDPMessage(w []byte) ([]ssdpField, map[string]any, error) {
	if len(w) < 21 || len(w) > ssdpMaxBytes {
		return nil, nil, fmt.Errorf("ssdp: message outside 21..65507 byte profile")
	}
	if !bytes.HasSuffix(w, []byte("\r\n\r\n")) {
		return nil, nil, fmt.Errorf("ssdp: incomplete header block or unsupported body")
	}
	end := bytes.Index(w, []byte("\r\n\r\n"))
	trailing := w[end+4:]
	if len(trailing)%2 != 0 || len(bytes.ReplaceAll(trailing, []byte("\r\n"), nil)) != 0 {
		return nil, nil, fmt.Errorf("ssdp: body or concatenated datagrams unsupported")
	}
	var fields []ssdpField
	info := map[string]any{"Protocol": "SSDP", "HTTP Version": "HTTP/1.1", "Observation": "unverified discovery advertisement", "Identity Verified": false, "Device Description Retrieved": false}
	lineEnd := bytes.Index(w, []byte("\r\n"))
	if lineEnd < 0 || lineEnd > 8192 {
		return nil, nil, fmt.Errorf("ssdp: start line exceeds profile")
	}
	line := string(w[:lineEnd])
	kind, method := "", ""
	text := func(name string, start, end int) ssdpField { return etcdText(name, start, end, string(w[start:end])) }
	switch line {
	case "NOTIFY * HTTP/1.1", "M-SEARCH * HTTP/1.1":
		method = strings.SplitN(line, " ", 2)[0]
		kind = "Notification"
		if method == "M-SEARCH" {
			kind = "Search Request"
		}
		fields = append(fields, text("Method", 0, len(method)), text("Method Separator", len(method), len(method)+1), text("Target", len(method)+1, len(method)+2), text("Target Separator", len(method)+2, len(method)+3), text("Version", len(method)+3, lineEnd))
	default:
		if !strings.HasPrefix(line, "HTTP/1.1 200 ") {
			return nil, nil, fmt.Errorf("ssdp: unsupported discovery start line")
		}
		for _, b := range []byte(line[13:]) {
			if b < 32 || b == 127 {
				return nil, nil, fmt.Errorf("ssdp: invalid reason phrase")
			}
		}
		kind = "Search Response"
		fields = append(fields, text("Version", 0, 8), text("Version Separator", 8, 9), text("Status", 9, 12), text("Status Separator", 12, 13), text("Reason", 13, lineEnd))
		info["Status Code"] = 200
	}
	info["Message Kind"], info["Method"] = kind, method
	fields = append(fields, text("Start Line CRLF", lineEnd, lineEnd+2))
	values := map[string]string{}
	present := map[string]bool{}
	var headers []map[string]any
	var headerFields []ssdpField
	singletons := " host man mx st nt nts usn ext cache-control location server bootid.upnp.org configid.upnp.org nextbootid.upnp.org searchport.upnp.org content-length transfer-encoding "
	for at := lineEnd + 2; at < end+2; {
		if len(headers) >= 128 {
			return nil, nil, fmt.Errorf("ssdp: header count exceeds 128")
		}
		n := bytes.Index(w[at:], []byte("\r\n"))
		if n < 0 || n > 8192 {
			return nil, nil, fmt.Errorf("ssdp: header line exceeds profile")
		}
		last := at + n
		colon := bytes.IndexByte(w[at:last], ':')
		if colon <= 0 || colon > 128 {
			return nil, nil, fmt.Errorf("ssdp: invalid header name")
		}
		colon += at
		name := string(w[at:colon])
		for _, b := range []byte(name) {
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
				return nil, nil, fmt.Errorf("ssdp: invalid header name")
			}
		}
		for _, b := range w[colon+1 : last] {
			if b < 32 && b != 9 || b == 127 {
				return nil, nil, fmt.Errorf("ssdp: invalid header value")
			}
		}
		normal := strings.ToLower(name)
		value := strings.Trim(string(w[colon+1:last]), " \t")
		if strings.Contains(singletons, " "+normal+" ") && present[normal] {
			return nil, nil, fmt.Errorf("ssdp: duplicate singleton %s", normal)
		}
		present[normal] = true
		values[normal] = value
		headers = append(headers, map[string]any{"Name": name, "Value": value, "Normalized Name": normal, "Value Byte Range": []int{colon + 1, last}})
		headerFields = append(headerFields, ssdpField{Name: fmt.Sprintf("Header %d", len(headers)), Start: at, End: last + 2, Children: []ssdpField{text("Name", at, colon), text("Colon", colon, colon+1), text("Value", colon+1, last), text("CRLF", last, last+2)}})
		at = last + 2
	}
	fields = append(fields, ssdpField{Name: "Headers", Start: lineEnd + 2, End: end + 2, Children: headerFields}, text("Message End", end+2, end+4))
	if len(trailing) > 0 {
		fields = append(fields, text("Trailing Empty Lines", end+4, len(w)))
		info["Trailing Empty Lines"] = len(trailing) / 2
	}
	info["Headers"] = headers
	decimal := func(s string, limit uint64) (uint64, error) {
		if s == "" || strings.Trim(s, "0123456789") != "" {
			return 0, fmt.Errorf("ssdp: invalid decimal")
		}
		v, e := strconv.ParseUint(s, 10, 64)
		if e != nil || v > limit {
			return 0, fmt.Errorf("ssdp: decimal exceeds profile")
		}
		return v, nil
	}
	if present["transfer-encoding"] {
		return nil, nil, fmt.Errorf("ssdp: transfer encoding unsupported")
	}
	if present["content-length"] {
		n, e := decimal(values["content-length"], 0)
		if e != nil || n != 0 {
			return nil, nil, fmt.Errorf("ssdp: nonzero or invalid body length")
		}
	}
	require := func(names ...string) error {
		for _, name := range names {
			if !present[name] || values[name] == "" {
				return fmt.Errorf("ssdp: missing %s", name)
			}
		}
		return nil
	}
	var required error
	target := ""
	var warnings []string
	if len(trailing) > 0 {
		warnings = append(warnings, "extra empty lines after the discovery header block")
	}
	switch kind {
	case "Notification":
		required = require("host", "nt", "nts", "usn")
		target = values["nt"]
		if values["nts"] != "ssdp:alive" && values["nts"] != "ssdp:byebye" && values["nts"] != "ssdp:update" {
			return nil, nil, fmt.Errorf("ssdp: invalid notification subtype")
		}
		if present["st"] || present["man"] || present["mx"] {
			return nil, nil, fmt.Errorf("ssdp: search fields in notification")
		}
		info["Notification Subtype"] = values["nts"]
		if values["nts"] == "ssdp:alive" {
			required = require("host", "nt", "nts", "usn", "location", "cache-control")
			if !present["server"] {
				warnings = append(warnings, "alive SERVER not observed")
			}
		}
		if values["nts"] == "ssdp:update" {
			required = require("host", "nt", "nts", "usn", "location", "bootid.upnp.org", "nextbootid.upnp.org")
			if !present["configid.upnp.org"] {
				warnings = append(warnings, "update CONFIGID not observed")
			}
		}
	case "Search Request":
		required = require("host", "man", "st")
		target = values["st"]
		if values["man"] != "\"ssdp:discover\"" {
			return nil, nil, fmt.Errorf("ssdp: invalid discovery MAN")
		}
		if present["nt"] || present["nts"] {
			return nil, nil, fmt.Errorf("ssdp: notification fields in search")
		}
	case "Search Response":
		required = require("st", "usn", "location", "cache-control")
		target = values["st"]
		if present["nt"] || present["nts"] || present["man"] || present["mx"] {
			return nil, nil, fmt.Errorf("ssdp: request fields in response")
		}
		if !present["ext"] {
			warnings = append(warnings, "response EXT not observed")
		} else if values["ext"] != "" {
			return nil, nil, fmt.Errorf("ssdp: response EXT must be empty")
		}
		if !present["server"] {
			warnings = append(warnings, "response SERVER not observed")
		}
	}
	if required != nil {
		return nil, nil, required
	}
	validTarget := func(s string) bool {
		return s == "upnp:rootdevice" || strings.HasPrefix(s, "uuid:") && len(s) > 5 || strings.HasPrefix(s, "urn:") && len(strings.Split(s, ":")) >= 4
	}
	if !(target == "ssdp:all" && (kind == "Search Request" || kind == "Notification")) && !validTarget(target) || strings.ContainsAny(target, " \t\r\n") {
		return nil, nil, fmt.Errorf("ssdp: invalid discovery target")
	}
	info["Discovery Target"] = target
	if kind == "Notification" && target == "ssdp:all" {
		warnings = append(warnings, "ssdp:all in NOTIFY NT is outside the UDA notification target profile")
	}
	if present["usn"] {
		usn := values["usn"]
		parts := strings.SplitN(usn, "::", 2)
		if !strings.HasPrefix(parts[0], "uuid:") || len(parts[0]) <= 5 || strings.ContainsAny(usn, " \t\r\n") {
			return nil, nil, fmt.Errorf("ssdp: invalid USN")
		}
		info["USN"], info["Device UUID"] = usn, parts[0][5:]
		if strings.HasPrefix(target, "uuid:") {
			if usn != target {
				return nil, nil, fmt.Errorf("ssdp: UUID target differs from USN")
			}
		} else if len(parts) != 2 || parts[1] != target {
			return nil, nil, fmt.Errorf("ssdp: USN target mismatch")
		}
	}
	if present["host"] {
		if _, e := decodeHTTPServiceURL("http://" + values["host"] + "/"); e != nil || strings.ContainsAny(values["host"], "/?#@") {
			return nil, nil, fmt.Errorf("ssdp: invalid HOST authority")
		}
		host, port, e := net.SplitHostPort(values["host"])
		if e != nil || host == "" {
			return nil, nil, fmt.Errorf("ssdp: invalid HOST authority")
		}
		if _, e = decimal(port, 65535); e != nil || port == "0" {
			return nil, nil, fmt.Errorf("ssdp: invalid HOST port")
		}
		ip := net.ParseIP(host)
		multicast := ip != nil && ip.IsMulticast()
		info["HOST"], info["Multicast"] = values["host"], multicast
		if kind == "Search Request" && multicast && !present["mx"] {
			return nil, nil, fmt.Errorf("ssdp: multicast search requires MX")
		}
		if kind == "Notification" && !multicast {
			warnings = append(warnings, "notification HOST is not multicast")
		}
	}
	for _, item := range []struct {
		key, label string
		max        uint64
	}{{"mx", "Maximum Wait Seconds", 4294967295}, {"bootid.upnp.org", "Boot ID", 2147483647}, {"nextbootid.upnp.org", "Next Boot ID", 2147483647}, {"configid.upnp.org", "Config ID", 16777215}, {"searchport.upnp.org", "Search Port", 65535}} {
		if present[item.key] {
			n, e := decimal(values[item.key], item.max)
			if e != nil || item.key == "mx" && n == 0 {
				return nil, nil, fmt.Errorf("ssdp: invalid %s", item.key)
			}
			info[item.label] = n
		}
	}
	if present["location"] {
		u, e := decodeHTTPServiceURL(values["location"])
		if e != nil {
			return nil, nil, fmt.Errorf("ssdp: invalid LOCATION URL")
		}
		info["Location"], info["Location URL"] = values["location"], u
	}
	if present["cache-control"] {
		age := ""
		quoted, escaped := false, false
		start := 0
		var parts []string
		cache := values["cache-control"]
		for i, b := range []byte(cache) {
			if escaped {
				escaped = false
				continue
			}
			if quoted && b == '\\' {
				escaped = true
				continue
			}
			if b == '"' {
				quoted = !quoted
			}
			if b == ',' && !quoted {
				parts = append(parts, cache[start:i])
				start = i + 1
			}
		}
		if quoted || escaped {
			return nil, nil, fmt.Errorf("ssdp: malformed CACHE-CONTROL")
		}
		parts = append(parts, cache[start:])
		for _, part := range parts {
			p := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if strings.EqualFold(strings.TrimSpace(p[0]), "max-age") {
				if len(p) != 2 || age != "" {
					return nil, nil, fmt.Errorf("ssdp: ambiguous max-age")
				}
				age = strings.TrimSpace(p[1])
			}
		}
		n, e := decimal(age, 4294967295)
		if e != nil {
			return nil, nil, fmt.Errorf("ssdp: invalid max-age")
		}
		info["Maximum Age Seconds"] = n
	}
	if present["server"] {
		info["Server"] = values["server"]
		if !strings.Contains(values["server"], "UPnP/") {
			warnings = append(warnings, "SERVER has no UPnP version token")
		}
	}
	info["Conformance Warnings"], info["Full UDA Conformance Validated"] = warnings, false
	return fields, info, nil
}

// DecodeSSDPMessage is shared by the compiled rule and native UDP ingress.
// Each datagram is validated independently; no port-based sticky session exists.
func DecodeSSDPMessage(w []byte) (map[string]any, error) {
	_, info, e := decodeSSDPMessage(w)
	return info, e
}
