package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net"
	"net/http/httputil"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// DisplayFrameObservation describes a physical capture record, independently
// of whether it contributed an application PDU. It never borrows CompletedIn.
type DisplayFrameObservation struct {
	Number         uint64
	Time           time.Time
	CapturedLength uint32
}

func packetDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, label string, typ DisplayFieldType, bits int, fn func(*ProtocolEvent) any) {
		b := displayCustom("", name, label, typ, bits, "event", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if v := fn(e); v != nil {
				emit(v)
			}
		})
		b.protocols = ""
		out = append(out, b)
	}
	for _, source := range []bool{true, false} {
		name := "ip.dst"
		port := "tcp.dstport"
		if source {
			name, port = "ip.src", "tcp.srcport"
		}
		endpoint := func(e *ProtocolEvent) string {
			if source {
				return e.Source
			}
			return e.Destination
		}
		add(name, "Observed endpoint IP address", DisplayAddress, 0, func(e *ProtocolEvent) any {
			host := endpoint(e)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if a, err := netip.ParseAddr(host); err == nil {
				return a.Unmap().String()
			}
			return nil
		})
		add(port, "Observed TCP endpoint port", DisplayUnsigned, 16, func(e *ProtocolEvent) any {
			if e.Transport != "tcp" {
				return nil
			}
			_, port, err := net.SplitHostPort(endpoint(e))
			if err != nil {
				return nil
			}
			if n, err := strconv.ParseUint(port, 10, 16); err == nil {
				return n
			}
			return nil
		})
	}
	add("tcp.port", "Either observed TCP port", DisplayUnsigned, 16, func(*ProtocolEvent) any { return nil })
	out[len(out)-1].custom = func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		if e.Transport != "tcp" {
			return
		}
		for _, endpoint := range []string{e.Source, e.Destination} {
			if _, port, err := net.SplitHostPort(endpoint); err == nil {
				if n, err := strconv.ParseUint(port, 10, 16); err == nil {
					emit(n)
				}
			}
		}
	}
	add("tcp.stream", "Captured TCP generation identifier (tcp.flow.id)", DisplayUnsigned, 64, func(e *ProtocolEvent) any {
		if e.TCPFlowID != 0 {
			return e.TCPFlowID
		}
		return nil
	})
	add("frame.number", "Physical capture record number", DisplayUnsigned, 64, func(e *ProtocolEvent) any {
		if e.Frame != nil {
			return e.Frame.Number
		}
		return nil
	})
	add("frame.time_epoch", "Physical capture timestamp in epoch seconds", DisplayString, 0, func(e *ProtocolEvent) any {
		if e.Frame == nil || e.Frame.Time.IsZero() {
			return nil
		}
		seconds, nanos := e.Frame.Time.Unix(), e.Frame.Time.Nanosecond()
		if seconds < 0 && nanos != 0 {
			return "-" + strconv.FormatInt(-(seconds+1), 10) + "." + fmtNanos(1e9-nanos)
		}
		return strconv.FormatInt(seconds, 10) + "." + fmtNanos(nanos)
	})
	add("frame.len", "Physical captured length in bytes", DisplayUnsigned, 32, func(e *ProtocolEvent) any {
		if e.Frame != nil {
			return e.Frame.CapturedLength
		}
		return nil
	})
	return out
}

func fmtNanos(n int) string { return strconv.FormatInt(int64(n)+1e9, 10)[1:] }

type httpMessageEvidence struct {
	fields            map[string]any
	header            int
	response, chunked bool
	complete          bool
	digest            [32]byte
}

// HTTPResponseBody returns only the body of a fully admitted HTTP/1 message.
// Chunk framing is decoded over that exact message's bytes; no stream recovery
// or inference of a missing request method is performed.
func (e *ProtocolEvent) HTTPResponseBody() (io.Reader, *int64, bool) {
	if e == nil || e.nativeHTTPMessage == nil || e.nativeDiagnosticError != "" || e.Completeness != "message" && e.Completeness != "transaction" {
		return nil, nil, false
	}
	h := e.nativeHTTPMessage
	status, _ := displayUint(h.fields["Status Code"])
	if !h.response || !h.complete || status < 200 || h.header > len(e.Raw) || h.header < 0 || e.Status == "malformed" || e.Status == "encrypted" || sha256.Sum256(e.Raw) != h.digest {
		return nil, nil, false
	}
	var length *int64
	// Content-Length is literal metadata; HEAD/304 may declare a representation
	// length even though framing correctly admitted an empty response body.
	displayHTTPHeaderField(e, "Content-Length", func(v any) {
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n >= 0 {
				length = &n
			}
		}
	}, &displayProjection{snapshot: &DisplayFieldSnapshot{}, remaining: maxDisplayFieldValues * 4})
	var body io.Reader = bytes.NewReader(e.Raw[h.header:])
	if h.chunked {
		body = httputil.NewChunkedReader(body)
	}
	return body, length, true
}

// Both export manifests and display fields use the same accepted literal
// headers. HTTP/1 maps and HTTP/2/3 Name/Value lists remain separate shapes.
func displayHTTPHeaderField(e *ProtocolEvent, name string, emit func(any), p *displayProjection) {
	if e.nativeDiagnosticError != "" {
		return
	}
	m := e.Session
	if e.nativeHTTPHeaderObservation {
		m = e.semanticFields
	} else if e.nativeHTTPMessage != nil {
		m = e.nativeHTTPMessage.fields
	}
	visit := func(value any) {
		switch headers := value.(type) {
		case map[string]any:
			for key, value := range headers {
				if !p.spend() {
					return
				}
				if strings.EqualFold(key, name) {
					displayScalarValues(value, emit, p)
				}
			}
		case map[string][]string:
			for key, values := range headers {
				if !p.spend() {
					return
				}
				if strings.EqualFold(key, name) {
					for _, value := range values {
						if !p.spend() {
							return
						}
						emit(value)
					}
				}
			}
		default:
			p.maps(value, func(h map[string]any) {
				if key, ok := h["Name"].(string); ok && strings.EqualFold(key, name) {
					if v := h["Value"]; v != nil {
						emit(v)
					}
				}
			})
		}
	}
	if e.Protocol == "http3" {
		displayHTTP3FrameValues(e, p, func(f map[string]any) { visit(f["Headers"]) })
		return
	}
	visit(m["Headers"])
}

func analysisDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, name := range []string{"pdu.content", "stream.content"} {
		origin := "pdu"
		if name == "stream.content" {
			origin = "context"
		}
		b := displayCustom("", name, "Bounded reassembled content search predicate (contains or hex:)", DisplayString, 0, origin, func(*ProtocolEvent, func(any), *displayProjection) {})
		b.protocols = ""
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		header    bool
		typ       DisplayFieldType
		bits      int
	}{
		{"http.host", "Host", false, DisplayString, 0}, {"http.request.uri", "Request URI", false, DisplayString, 0}, {"http.request.version", "Protocol", false, DisplayString, 0},
		{"http.user_agent", "User-Agent", true, DisplayString, 0}, {"http.response.location", "Location", true, DisplayString, 0}, {"http.response.content_type", "Content-Type", true, DisplayString, 0},
		{"http.request.header.length", "Content-Length", true, DisplayUnsigned, 16}, {"http.cookie", "Cookie", true, DisplayString, 0}, {"http.request.authorization", "Authorization", true, DisplayString, 0},
	} {
		b := displayCustom("http", f.name, "Literal HTTP "+f.key, f.typ, f.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if e.nativeDiagnosticError != "" {
				return
			}
			if f.header {
				displayHTTPHeaderField(e, f.key, emit, p)
				return
			}
			m := e.Session
			if e.nativeHTTPHeaderObservation {
				m = e.semanticFields
			} else if e.nativeHTTPMessage != nil {
				m = e.nativeHTTPMessage.fields
			}
			if _, h2 := m["Stream ID"]; h2 || e.Protocol == "http3" {
				key := ":authority"
				if f.key == "Request URI" {
					key = ":path"
				}
				if f.key != "Protocol" {
					displayHTTPHeaderField(e, key, emit, p)
				}
				return
			}
			if f.key == "Protocol" && m["Method"] == nil {
				return
			}
			if v := m[f.key]; v != nil {
				emit(v)
			}
		})
		b.protocols = "|http|ipp|http2|grpc|doh|http3|"
		b.descriptor.AllowEmpty = f.header && f.typ == DisplayString
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
		rr        uint64
	}{
		{"dns.rr.name", "Name", DisplayString, 0, 0}, {"dns.rr.type", "Type", DisplayUnsigned, 16, 0}, {"dns.rr.ttl", "TTL", DisplayUnsigned, 32, 0},
		{"dns.a", "Address", DisplayString, 0, 1}, {"dns.aaaa", "Address", DisplayString, 0, 28}, {"dns.cname", "Target", DisplayString, 0, 5},
		{"dns.txt", "Text", DisplayString, 0, 16}, {"dns.mx", "Target", DisplayString, 0, 15}, {"dns.srv.target", "Target", DisplayString, 0, 33}, {"dns.srv.port", "Port", DisplayUnsigned, 16, 33}, {"dns.soa.mname", "MNAME", DisplayString, 0, 6},
	} {
		b := displayCustom("dns", f.name, "DNS answer "+f.key, f.typ, f.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			p.walk(e.Session, []string{"DNS", "Answers", "*"}, func(v any) {
				if row, ok := v.(map[string]any); ok {
					typ, _ := displayUint(row["Type"])
					if f.rr == 0 || typ == f.rr {
						displayScalarValues(row[f.key], emit, p)
					}
				}
			})
		})
		b.protocols = "|dns|mdns|llmnr|dot|doh|doq|"
		b.descriptor.AllowEmpty = f.name == "dns.txt" || f.name == "dns.rr.name"
		out = append(out, b)
	}
	b := displayRoot("dns", "dns.rcode", "DNS response code", DisplayUnsigned, 8, "pdu", "")
	b.protocols, b.path = "|dns|mdns|llmnr|dot|doh|doq|", []string{"DNS", "RCODE"}
	out = append(out, b)
	out = append(out, lanAnalysisDisplayBindings()...)
	return append(out, kerberosAnalysisDisplayBindings()...)
}

func kerberosAnalysisDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, f := range []struct {
		name, label string
		typ         DisplayFieldType
		bits        int
		origin      string
	}{
		{"kerberos.msg_type", "Kerberos message type", DisplayUnsigned, 8, "pdu"}, {"kerberos.realm", "Kerberos realm", DisplayString, 0, "pdu"}, {"kerberos.client", "Kerberos client principal", DisplayString, 0, "pdu"}, {"kerberos.server", "Kerberos server principal", DisplayString, 0, "pdu"}, {"kerberos.etypes", "Kerberos encryption types (signed decimal strings)", DisplayString, 0, "pdu"}, {"kerberos.decrypt_status", "Kerberos passive decryption status", DisplayString, 0, "derived"}, {"kerberos.identity_verified", "Kerberos peer identity validated", DisplayBoolean, 0, "derived"},
	} {
		out = append(out, displayCustom("kerberos", f.name, f.label, f.typ, f.bits, f.origin, func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if e.nativeDiagnosticError != "" || e.Status != "decoded" && e.Status != "deferred" {
				return
			}
			if !p.kerberosLoaded {
				p.kerberosLoaded = true
				p.kerberosFacts = e.kerberosSummaryFacts
				if p.kerberosFacts == nil {
					if metadata, ok := e.Metadata.(map[string]any); ok && p.kerberosFacts == nil {
						p.kerberosFacts, _ = metadata["Semantic Summary"].(map[string]any)
					}
					if p.kerberosFacts == nil {
						p.kerberosFacts, _ = stream_parser.DecodeKerberosSummaryFacts(e.Raw, e.Entry == "KerberosTCPFields")
					}
				}
			}
			m := p.kerberosFacts
			if m == nil {
				return
			}
			switch f.name {
			case "kerberos.msg_type":
				if value := m["Message Type"]; value != nil {
					emit(value)
				}
			case "kerberos.realm":
				if value := firstSummaryString(m, "Realm", "Client Realm", "Ticket Realm"); value != "" {
					emit(value)
				}
			case "kerberos.client":
				if value := m["Client Principal"]; value != nil {
					emit(value)
				}
			case "kerberos.server":
				if value := firstSummaryString(m, "Server Principal", "Ticket Server"); value != "" {
					emit(value)
				}
			case "kerberos.etypes":
				for _, value := range mergeSummaryIntegers(summaryInt64Slice(m["Requested Encryption Types"]), summaryInt64Slice(m["Observed Encryption Types"])) {
					if !p.spend() {
						return
					}
					emit(strconv.FormatInt(value, 10))
				}
			case "kerberos.decrypt_status":
				emit("no")
			case "kerberos.identity_verified":
				emit(false)
			}
		}))
	}
	return out
}

func displayScalarValues(value any, emit func(any), p *displayProjection) {
	switch values := value.(type) {
	case []string:
		for _, v := range values {
			if !p.spend() {
				return
			}
			emit(v)
		}
	case []int64:
		for _, v := range values {
			if !p.spend() {
				return
			}
			emit(v)
		}
	case []any:
		for _, v := range values {
			if !p.spend() {
				return
			}
			if v != nil {
				emit(v)
			}
		}
	default:
		if value != nil {
			emit(value)
		}
	}
}

func dhcpText(value any) (text, encoding string) {
	var wire []byte
	switch v := value.(type) {
	case []byte:
		wire = v
	case string:
		wire = []byte(v)
	default:
		return "", ""
	}
	if len(wire) == 0 {
		return "", ""
	}
	printable := func(b []byte) bool {
		for _, c := range b {
			if c < 32 || c > 126 {
				return false
			}
		}
		return len(b) > 0
	}
	if printable(wire) {
		text = string(wire)
		if decoded, err := base64.StdEncoding.Strict().DecodeString(text); err == nil && len(text)%4 == 0 && printable(decoded) && base64.StdEncoding.EncodeToString(decoded) == text {
			return text, "base64"
		}
		return text, "ascii"
	}
	return hex.EncodeToString(wire), "hex"
}

func lanAnalysisDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"dhcp.msg_type", "Message Type", DisplayUnsigned, 8}, {"dhcp.requested_ip", "Requested IP", DisplayAddress, 0}, {"dhcp.lease_time", "Lease Seconds", DisplayUnsigned, 32},
	} {
		out = append(out, displayRoot("dhcp", f.name, "DHCP "+f.key, f.typ, f.bits, "pdu", f.key))
	}
	out = append(out, displayCustom("dhcp", "dhcp.client_mac", "DHCP client hardware address", DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		if b, ok := e.Session["CHADDR"].([]byte); ok && len(b) > 0 {
			emit(net.HardwareAddr(b).String())
		}
	}))
	for _, name := range []string{"dhcp.hostname", "dhcp.hostname.encoding", "dhcp.hostname.encoded", "dhcp.vendor_class"} {
		out = append(out, displayCustom("dhcp", name, "Literal DHCP text; encoded hostnames are separate", DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			key := "Hostname"
			if name == "dhcp.vendor_class" {
				key = "Vendor Class"
			}
			text, encoding := dhcpText(e.Session[key])
			if text == "" {
				return
			}
			switch name {
			case "dhcp.hostname.encoding":
				emit(encoding)
			case "dhcp.hostname.encoded":
				if encoding != "ascii" {
					emit(text)
				}
			default:
				if encoding == "ascii" {
					emit(text)
				}
			}
		}))
	}
	for _, protocol := range []string{"mdns", "nbns"} {
		out = append(out, displayCustom(protocol, protocol+".name", "Observed name service question or answer name", DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			root := e.Session
			if protocol == "mdns" {
				root, _ = root["DNS"].(map[string]any)
			}
			for _, section := range []string{"Questions", "Answers"} {
				p.walk(root, []string{section, "*", "Name"}, func(v any) {
					if v != nil {
						emit(v)
					}
				})
			}
		}))
	}
	return out
}
