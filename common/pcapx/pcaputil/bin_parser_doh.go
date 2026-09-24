package pcaputil

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// binDoH is the M0 session state for RFC 8484 DNS-over-HTTPS on HTTP/1.1
// or HTTP/2 plaintext. Ports 443/53 are never consulted. TLS decryption
// is out of scope; Feed is the inner HTTP.
type binDoH struct {
	pending map[uint16]string
}

func dohMedia(v string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(v, ";", 2)[0]))
	return m == "application/dns-message"
}

func dohPathEvidence(path string) bool {
	queryAt := strings.IndexByte(path, '?')
	if queryAt < 0 {
		return false
	}
	if !strings.Contains(path[queryAt+1:], "dns") {
		return false
	}
	query, err := url.ParseQuery(path[queryAt+1:])
	if err != nil {
		return false
	}
	values, ok := query["dns"]
	if !ok || len(values) != 1 {
		return false
	}
	// The endpoint URI is configured out of band. A common /dns-query path
	// gives enough context to report a malformed query; on an arbitrary HTTP
	// path, require an actual DNS wire question to avoid claiming application
	// parameters such as /search?dns=example.com.
	if strings.HasSuffix(path[:queryAt], "/dns-query") {
		return true
	}
	msg, err := base64.RawURLEncoding.DecodeString(values[0])
	if err != nil || len(msg) < 12 || msg[2]&0x80 != 0 || binary.BigEndian.Uint16(msg[4:6]) == 0 {
		return false
	}
	_, _, err = dnsQuestion(msg)
	return err == nil
}

func dohHTTP1RequestEvidence(method, path, contentType string) bool {
	switch method {
	case http.MethodGet:
		return dohPathEvidence(path)
	case http.MethodPost:
		return dohMedia(contentType)
	default:
		return false
	}
}

func (f *binFlow) consumeDoHHTTP1(raw []byte) (map[string]any, error) {
	if bytes.HasPrefix(raw, []byte("HTTP/")) {
		return f.dohHTTP1Response(raw)
	}
	return f.dohHTTP1Request(raw)
}

func (f *binFlow) dohHTTP1Request(raw []byte) (map[string]any, error) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return nil, nil
	}
	defer req.Body.Close()
	ctype := req.Header.Get("Content-Type")
	path := req.URL.RequestURI()
	if !dohHTTP1RequestEvidence(req.Method, path, ctype) {
		return nil, nil
	}
	f.ensureDoH()
	info := map[string]any{
		"DoH":                 true,
		"HTTP Version":        "1.1",
		"Method":              req.Method,
		"Path":                path,
		"Content Type":        ctype,
		"Protocol Transition": "http->doh",
		"Context Level":       "observed",
	}
	var msg []byte
	switch req.Method {
	case http.MethodGet:
		msg, err = dohDecodeGET(path)
		if err != nil {
			return info, err
		}
	case http.MethodPost:
		if !dohMedia(ctype) {
			return info, protocolError(ErrMalformedMessage, "DoH POST requires application/dns-message")
		}
		msg, err = io.ReadAll(io.LimitReader(req.Body, int64(f.a.config.MaxMessageBytes)+1))
		if err != nil {
			return info, err
		}
		if len(msg) > f.a.config.MaxMessageBytes {
			return info, protocolError(ErrResourceExceeded, "DoH POST body exceeds limit")
		}
	default:
		return info, protocolError(ErrUnsupportedFeature, "DoH method must be GET or POST")
	}
	return f.doh.attachDNS(info, msg, false, f.a.budget.MaxCollectionElements)
}

func (f *binFlow) dohHTTP1Response(raw []byte) (map[string]any, error) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), &http.Request{Method: http.MethodGet})
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	ctype := resp.Header.Get("Content-Type")
	if f.doh == nil && !dohMedia(ctype) {
		return nil, nil
	}
	if f.doh == nil {
		return nil, nil
	}
	info := map[string]any{
		"DoH":           true,
		"HTTP Version":  "1.1",
		"HTTP Status":   resp.StatusCode,
		"Content Type":  ctype,
		"Context Level": "observed",
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(f.a.config.MaxMessageBytes)+1))
	if err != nil {
		return info, err
	}
	if len(body) > f.a.config.MaxMessageBytes {
		return info, protocolError(ErrResourceExceeded, "DoH response body exceeds limit")
	}
	if resp.StatusCode >= 400 && !dohMedia(ctype) {
		info["Packet Name"] = "Error"
		info["Error Body"] = bytes.Clone(body)
		info["Association Status"] = "http-error"
		return info, nil
	}
	if !dohMedia(ctype) {
		return info, protocolError(ErrMalformedMessage, "DoH response requires application/dns-message")
	}
	return f.doh.attachDNS(info, body, true, f.a.budget.MaxCollectionElements)
}

func (f *binFlow) consumeDoHH2(dir int, e *ProtocolEvent, stream *binH2Stream) error {
	h := f.h2
	if h == nil || e.Session == nil {
		return nil
	}
	id, _ := e.Session["Stream ID"].(uint32)
	if stream == nil {
		stream = h.streams[id]
	}
	if stream == nil {
		return nil
	}
	headers, _ := e.Session["Headers"].([]map[string]any)
	path, ctype, method := "", "", stream.method
	for _, header := range headers {
		name, _ := header["Name"].(string)
		value, _ := header["Value"].(string)
		switch name {
		case ":path":
			path = value
		case "content-type":
			ctype = value
		case ":method":
			method = value
		}
	}
	kind, _ := e.Session["Header Kind"].(string)
	if kind == "request" && dohHTTP1RequestEvidence(method, path, ctype) || kind == "response" && dohMedia(ctype) {
		stream.doh[dir] = true
	}
	if !stream.doh[0] && !stream.doh[1] && !stream.doh[dir] {
		return nil
	}
	f.ensureDoH()
	if e.Session == nil {
		e.Session = map[string]any{}
	}
	e.Session["DoH"] = true
	e.Session["HTTP Version"] = "2"
	e.Session["Protocol Transition"] = "http2->doh"
	if path != "" {
		e.Session["Path"] = path
	}
	if method != "" {
		e.Session["Method"] = method
	}
	if ctype != "" {
		e.Session["Content Type"] = ctype
	}
	if kind == "request" && method == http.MethodGet {
		msg, err := dohDecodeGET(path)
		if err != nil {
			return err
		}
		if msg != nil {
			_, err = f.doh.attachDNS(e.Session, msg, false, f.a.budget.MaxCollectionElements)
			return err
		}
	}
	if kind == "response" {
		status, _ := dohPseudo(headers, ":status")
		if status != "" {
			e.Session["HTTP Status"] = status
			if status[0] >= '4' && !dohMedia(ctype) {
				e.Session["Packet Name"] = "Error"
				e.Session["Association Status"] = "http-error"
			}
		}
	}
	typ, _ := e.Session["Frame Type"].(byte)
	if typ == 0 && (stream.doh[0] || stream.doh[1] || stream.doh[dir]) {
		payload := grpcDATAPayload(e.Raw)
		old := stream.dohBuf[dir]
		size := len(old) + len(payload)
		if size > f.a.config.MaxMessageBytes {
			return protocolError(ErrResourceExceeded, "DoH buffered DATA exceeds limit")
		}
		buf := make([]byte, size)
		copy(buf, old)
		copy(buf[len(old):], payload)
		stream.dohBuf[dir] = buf
		if e.Session["End Stream"] == true {
			httpResp := dir != h.client
			if kind == "response" || httpResp {
				if status, _ := e.Session["HTTP Status"].(string); len(status) > 0 && status[0] >= '4' && !dohMedia(ctype) {
					e.Session["Packet Name"] = "Error"
					e.Session["Error Body"] = bytes.Clone(buf)
					e.Session["Association Status"] = "http-error"
					stream.dohBuf[dir] = nil
					return nil
				}
			}
			_, err := f.doh.attachDNS(e.Session, buf, httpResp, f.a.budget.MaxCollectionElements)
			stream.dohBuf[dir] = nil
			return err
		}
	}
	return nil
}

func dohPseudo(headers []map[string]any, name string) (string, bool) {
	for _, header := range headers {
		n, _ := header["Name"].(string)
		if n == name {
			v, _ := header["Value"].(string)
			return v, true
		}
	}
	return "", false
}

func dohDecodeGET(path string) ([]byte, error) {
	i := strings.Index(path, "?")
	if i < 0 {
		return nil, protocolError(ErrMalformedMessage, "DoH GET missing dns parameter")
	}
	query, err := url.ParseQuery(path[i+1:])
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, "DoH GET query is malformed")
	}
	values := query["dns"]
	if len(values) != 1 || values[0] == "" {
		return nil, protocolError(ErrMalformedMessage, "DoH GET requires exactly one dns parameter")
	}
	msg, err := base64.RawURLEncoding.DecodeString(values[0])
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, "DoH GET dns parameter is not base64url")
	}
	return msg, nil
}

func (f *binFlow) ensureDoH() {
	if f.doh == nil {
		f.doh = &binDoH{pending: map[uint16]string{}}
	}
}

func (d *binDoH) attachDNS(info map[string]any, msg []byte, response bool, max int) (map[string]any, error) {
	semantic, err := DecodeDNSMessage(msg, max)
	if err != nil {
		return info, err
	}
	info["DNS"] = semantic
	if len(msg) < 12 {
		return info, protocolError(ErrMalformedMessage, "DoH DNS message shorter than header")
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	qname, qtype, err := dnsQuestion(msg)
	if err != nil {
		return info, err
	}
	info["Transaction ID"] = id
	info["QR"] = flags>>15 != 0
	info["Opcode"] = (flags >> 11) & 0xf
	info["RCODE"] = flags & 0xf
	info["QNAME"] = qname
	info["QTYPE"] = qtype
	info["QTYPE Name"] = dnsTypeName(qtype)
	if !response {
		info["Packet Name"] = "Query"
		if max <= 0 {
			max = 4096
		}
		if len(d.pending) >= max {
			return info, protocolError(ErrResourceExceeded, "DoH outstanding transaction IDs exceed budget")
		}
		d.pending[id] = qname
		info["Outstanding"] = true
		return info, nil
	}
	info["Packet Name"] = "Response"
	if want, ok := d.pending[id]; ok {
		delete(d.pending, id)
		info["Matched Request"] = want
		info["Association Status"] = "matched"
	} else {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
	}
	if addrs := dnsARecords(msg); len(addrs) > 0 {
		info["A Records"] = addrs
	}
	return info, nil
}

func dohSummary(session map[string]any) string {
	return fmt.Sprintf("DoH %v %v %v", session["Method"], session["Packet Name"], session["QNAME"])
}
