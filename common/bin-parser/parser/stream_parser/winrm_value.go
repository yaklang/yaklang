package stream_parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"mime"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Passive bounded HTTP/1.1 and SOAP 1.2 syntax. No request is issued, URI
// fetched, extension executed or resource schema loaded. DMTF DSP0226 1.1.1
// sections 7.3, 11 and Annex F; MS-WSMV 2.2.1 and 4.1.1; SOAP 1.2 Part 1.
const (
	winrmWSMan        = "http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
	winrmIdentifyMS   = "http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd"
	winrmIdentifyDMTF = "http://schemas.dmtf.org/wbem/wsman/identity/1/wsmanidentity.xsd"
	winrmTransfer     = "http://schemas.xmlsoap.org/ws/2004/09/transfer/"
)

type winrmField struct {
	Name, Type string
	Start, End int
	Children   []winrmField
	Value      any
	Decoded    bool
	Info       map[string]any
}

func winrmTextField(name string, start, end int, value any) winrmField {
	return winrmField{Name: name, Type: "raw", Start: start, End: end, Value: value, Decoded: true}
}

func winrmToken(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b)) {
			continue
		}
		return false
	}
	return true
}

func decodeWinRMRecord(wire []byte) ([]winrmField, map[string]any, error) {
	fail := func(s string) ([]winrmField, map[string]any, error) { return nil, nil, fmt.Errorf("winrm: %s", s) }
	if len(wire) == 0 || len(wire) > 1<<20 {
		return fail("record size outside 1..1048576 bytes")
	}
	end := bytes.Index(wire, []byte("\r\n\r\n"))
	if end < 0 || end > 65536 {
		return fail("missing or oversized HTTP header")
	}
	lineEnd := bytes.Index(wire[:end+2], []byte("\r\n"))
	parts := strings.SplitN(string(wire[:lineEnd]), " ", 3)
	if len(parts) != 3 {
		return fail("invalid HTTP start line")
	}
	response := strings.HasPrefix(parts[0], "HTTP/")
	fields := []winrmField{}
	if response {
		if parts[0] != "HTTP/1.1" || len(parts[1]) != 3 {
			return fail("unsupported HTTP response line")
		}
		status, e := strconv.ParseUint(parts[1], 10, 16)
		if e != nil || status < 200 || status > 599 || status == 204 || status == 304 || strings.HasPrefix(parts[1], "+") {
			return fail("invalid HTTP response status")
		}
		fields = append(fields, winrmTextField("Version", 0, 8, parts[0]), winrmTextField("Status", 8, 12, uint64(status)), winrmTextField("Reason", 12, lineEnd+2, parts[2]))
	} else {
		if parts[0] != "POST" || parts[2] != "HTTP/1.1" {
			return fail("profile requires HTTP/1.1 POST")
		}
		u, e := url.ParseRequestURI(parts[1])
		if e != nil || u.Fragment != "" || u.Path == "" {
			return fail("invalid HTTP request target")
		}
		fields = append(fields, winrmTextField("Method", 0, 4, parts[0]), winrmTextField("Path", 4, 5+len(parts[1]), parts[1]), winrmTextField("Version", 5+len(parts[1]), lineEnd+2, parts[2]))
	}
	for _, c := range wire[:lineEnd] {
		if c < 32 || c == 127 {
			return fail("HTTP start-line control octet")
		}
	}
	headers := winrmField{Name: "HTTP Headers", Start: lineEnd + 2, End: end + 4}
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
		name := "HTTP " + key
		var decoded any = val
		if key == "content-length" {
			if val == "" {
				return fail("invalid Content-Length")
			}
			for _, c := range []byte(val) {
				if c < '0' || c > '9' {
					return fail("invalid Content-Length")
				}
			}
			n, e := strconv.ParseUint(val, 10, 32)
			if e != nil || n > 1<<20 {
				return fail("Content-Length limit")
			}
			decoded, name = n, "Content Length"
		}
		headers.Children = append(headers.Children, winrmTextField(name, pos, pos+n+2, decoded))
		pos += n + 2
	}
	headers.Children = append(headers.Children, winrmField{Name: "Header Terminator", Type: "raw", Start: end + 2, End: end + 4})
	fields = append(fields, headers)
	for _, key := range []string{"content-length", "content-type", "host", "soapaction", "content-encoding"} {
		if len(values[key]) > 1 {
			return fail("duplicate " + key + " outside exact profile")
		}
	}
	if len(values["transfer-encoding"]) != 0 {
		return fail("transfer coding outside Content-Length profile")
	}
	if len(values["content-encoding"]) != 0 && !strings.EqualFold(values["content-encoding"][0], "identity") {
		return fail("encoded body outside plaintext profile")
	}
	if !response && (len(values["host"]) != 1 || values["host"][0] == "") {
		return fail("HTTP/1.1 request requires Host")
	}
	if len(values["content-length"]) != 1 {
		return fail("explicit Content-Length required")
	}
	size, _ := strconv.ParseUint(values["content-length"][0], 10, 32)
	if int(size) != len(wire)-end-4 {
		return fail("Content-Length does not match exact record body")
	}
	if len(values["content-type"]) != 1 {
		return fail("SOAP media type required")
	}
	media, params, err := mime.ParseMediaType(values["content-type"][0])
	if err != nil || media != "application/soap+xml" {
		return fail("SOAP 1.2 application/soap+xml required")
	}
	if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		return fail("non-UTF-8 body outside profile")
	}
	body := wire[end+4:]
	if !utf8.Valid(body) {
		return fail("invalid UTF-8 body")
	}
	p := winrmXML{labels: map[*wsDiscoveryElement]winrmField{}}
	xmlRoot, err := wsDiscoveryReadXML(string(body), 48)
	if err != nil {
		return fail("SOAP XML: " + err.Error())
	}
	info, err := p.envelope(xmlRoot, response)
	if err != nil {
		return nil, nil, err
	}
	action, _ := info["Action"].(string)
	if v, present := params["action"]; present {
		if err := wsDiscoveryURI(v, true); err != nil || action != v {
			return fail("SOAP media action differs from WS-Addressing Action")
		}
	}
	if len(values["soapaction"]) == 1 {
		v := strings.Trim(values["soapaction"][0], "\"")
		if v != action {
			return fail("SOAPAction differs from WS-Addressing Action")
		}
	}
	// Prefix/suffix and every XML lexical gap have their own nonoverlapping raw
	// spans. A scalar's decoded value is attached to its entire original element.
	soap := winrmField{Name: "SOAP", Start: end + 4, End: len(wire)}
	if xmlRoot.start > 0 {
		soap.Children = append(soap.Children, winrmField{Name: "XML Prolog", Type: "raw", Start: end + 4, End: end + 4 + xmlRoot.start})
	}
	soap.Children = append(soap.Children, p.tree(xmlRoot, end+4))
	if xmlRoot.end < len(body) {
		soap.Children = append(soap.Children, winrmField{Name: "XML Suffix", Type: "raw", Start: end + 4 + xmlRoot.end, End: len(wire)})
	}
	fields = append(fields, soap)
	info["Profile"] = "HTTP/1.1 Content-Length UTF-8 SOAP 1.2; Identify and WS-Transfer Get"
	info["Extension Semantics"] = "Opaque; namespaces and raw XML retained"
	info["Remote Operations Executed"] = false
	return fields, info, nil
}

type winrmXML struct {
	labels map[*wsDiscoveryElement]winrmField
}

func (p *winrmXML) err(s string) error { return fmt.Errorf("winrm: SOAP %s", s) }
func (p *winrmXML) container(n *wsDiscoveryElement, name string) error {
	if !xmlrpcSpace(n.text) {
		return p.err("mixed content in " + n.name.Local)
	}
	p.labels[n] = winrmField{Name: name}
	return nil
}
func (p *winrmXML) scalar(n *wsDiscoveryElement, uri bool) (string, error) {
	if len(n.children) != 0 {
		return "", p.err("nested scalar " + n.name.Local)
	}
	s := n.text
	if uri {
		s = strings.Trim(s, " \t\r\n")
		if e := wsDiscoveryURI(s, true); e != nil {
			return "", p.err("invalid URI in " + n.name.Local)
		}
	}
	p.labels[n] = winrmTextField(n.name.Local, 0, 0, s)
	return s, nil
}
func (p *winrmXML) tree(n *wsDiscoveryElement, offset int) winrmField {
	f, known := p.labels[n]
	if !known {
		f = winrmField{Name: "Extension " + n.name.Local, Type: "raw"}
	}
	f.Start, f.End = offset+n.start, offset+n.end
	f.Info = map[string]any{"Namespace": n.name.Space, "Local Name": n.name.Local, "Attributes": append([]xml.Attr(nil), n.attrs...), "Namespaces": wsDiscoveryCloneNS(n.ns)}
	if f.Type != "" {
		return f
	}
	pos := n.start
	for _, child := range n.children {
		if pos < child.start {
			f.Children = append(f.Children, winrmField{Name: "XML Syntax", Type: "raw", Start: offset + pos, End: offset + child.start})
		}
		f.Children = append(f.Children, p.tree(child, offset))
		pos = child.end
	}
	if pos < n.end {
		f.Children = append(f.Children, winrmField{Name: "XML Syntax", Type: "raw", Start: offset + pos, End: offset + n.end})
	}
	return f
}

func (p *winrmXML) envelope(root *wsDiscoveryElement, response bool) (map[string]any, error) {
	if root.name != (xml.Name{Space: wsDiscoverySOAP12, Local: "Envelope"}) {
		return nil, p.err("expected SOAP 1.2 Envelope")
	}
	if err := p.container(root, "Envelope"); err != nil {
		return nil, err
	}
	var header, body *wsDiscoveryElement
	for i, n := range root.children {
		if n.name.Space != wsDiscoverySOAP12 {
			return nil, p.err("foreign Envelope child")
		}
		switch n.name.Local {
		case "Header":
			if i != 0 || header != nil || body != nil {
				return nil, p.err("duplicate or misplaced Header")
			}
			header = n
		case "Body":
			if body != nil || i != len(root.children)-1 {
				return nil, p.err("duplicate or misplaced Body")
			}
			body = n
		default:
			return nil, p.err("unexpected Envelope child")
		}
	}
	if body == nil {
		return nil, p.err("missing Body")
	}
	for _, n := range []*wsDiscoveryElement{root, header, body} {
		if n == nil {
			continue
		}
		for _, a := range n.attrs {
			if a.Name.Space == "" || a.Name.Space == wsDiscoverySOAP12 {
				return nil, p.err("invalid attribute on " + n.name.Local)
			}
		}
	}
	if e := p.container(body, "Body"); e != nil {
		return nil, e
	}
	headerValues := map[string]string{}
	if header != nil {
		if e := p.container(header, "Header"); e != nil {
			return nil, e
		}
		seen := map[xml.Name]bool{}
		for _, n := range header.children {
			if n.name.Space == "" {
				return nil, p.err("unqualified Header block")
			}
			if n.name.Space == wsDiscoverySOAP12 {
				return nil, p.err("SOAP control header outside profile")
			}
			for _, a := range n.attrs {
				if a.Name.Space == wsDiscoverySOAP12 {
					switch a.Name.Local {
					case "mustUnderstand", "relay":
						if a.Value != "0" && a.Value != "1" && a.Value != "true" && a.Value != "false" {
							return nil, p.err("invalid SOAP boolean")
						}
					case "role":
						if e := wsDiscoveryURI(a.Value, true); e != nil {
							return nil, p.err("invalid SOAP role")
						}
					default:
						return nil, p.err("unknown SOAP header attribute")
					}
				}
			}
			if n.name.Space != wsDiscoveryAddressing && n.name.Space != winrmWSMan {
				continue
			}
			for _, a := range n.attrs {
				if a.Name.Space == "" {
					if n.name.Space == wsDiscoveryAddressing && n.name.Local == "RelatesTo" && a.Name.Local == "RelationshipType" {
						if _, err := wsDiscoveryQName(a.Value, n.ns); err != nil {
							return nil, p.err("invalid RelationshipType QName")
						}
					} else {
						return nil, p.err("unexpected unqualified header attribute")
					}
				}
			}
			if seen[n.name] {
				return nil, p.err("duplicate " + n.name.Local)
			}
			seen[n.name] = true
			if n.name.Space == wsDiscoveryAddressing {
				switch n.name.Local {
				case "Action", "To", "MessageID", "RelatesTo":
					s, e := p.scalar(n, true)
					if e != nil {
						return nil, e
					}
					headerValues[n.name.Local] = s
				case "ReplyTo", "From", "FaultTo":
					if e := p.endpoint(n); e != nil {
						return nil, e
					}
				default:
					return nil, p.err("unsupported WS-Addressing block " + n.name.Local)
				}
			} else {
				switch n.name.Local {
				case "ResourceURI":
					s, e := p.scalar(n, true)
					if e != nil {
						return nil, e
					}
					headerValues[n.name.Local] = s
				case "MaxEnvelopeSize":
					s, e := p.scalar(n, false)
					if e != nil {
						return nil, e
					}
					v, e := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(s), "+"), 10, 32)
					if e != nil || v == 0 {
						return nil, p.err("invalid MaxEnvelopeSize")
					}
					f := p.labels[n]
					f.Value = v
					p.labels[n] = f
				case "OperationTimeout":
					s, e := p.scalar(n, false)
					if e != nil {
						return nil, e
					}
					if !winrmDuration(strings.TrimSpace(s)) {
						return nil, p.err("invalid OperationTimeout duration")
					}
				case "SelectorSet":
					if e := p.selectors(n); e != nil {
						return nil, e
					}
				default: // Explicit raw extension, not an assertion of its semantics.
				}
			}
		}
	}
	info := map[string]any{"SOAP Namespace": wsDiscoverySOAP12, "Action": headerValues["Action"]}
	if len(body.children) == 1 {
		n := body.children[0]
		if n.name.Space == winrmIdentifyMS || n.name.Space == winrmIdentifyDMTF {
			if e := p.identify(n, response); e != nil {
				return nil, e
			}
			info["Kind"] = n.name.Local
			info["Identify Namespace"] = n.name.Space
			return info, nil
		}
	}
	action := headerValues["Action"]
	if action != winrmTransfer+"Get" && action != winrmTransfer+"GetResponse" {
		return nil, p.err("operation outside Identify/Get profile")
	}
	if headerValues["To"] == "" || headerValues["MessageID"] == "" {
		return nil, p.err("Get requires To and MessageID")
	}
	if action == winrmTransfer+"Get" {
		if response || len(body.children) != 0 || headerValues["ResourceURI"] == "" {
			return nil, p.err("Get request requires ResourceURI and empty Body")
		}
		info["Kind"] = "Get"
	} else {
		if !response || len(body.children) != 1 || headerValues["RelatesTo"] == "" {
			return nil, p.err("GetResponse requires RelatesTo and one resource representation")
		}
		if body.children[0].name.Space == "" || body.children[0].name.Space == wsDiscoverySOAP12 {
			return nil, p.err("invalid Get resource namespace")
		}
		p.resource(body.children[0])
		info["Kind"] = "GetResponse"
		info["Resource Schema"] = "Not validated; XML names and scalar text only"
	}
	return info, nil
}

func (p *winrmXML) endpoint(n *wsDiscoveryElement) error {
	if e := p.container(n, n.name.Local); e != nil {
		return e
	}
	if len(n.children) == 0 || n.children[0].name != (xml.Name{Space: wsDiscoveryAddressing, Local: "Address"}) {
		return p.err("endpoint requires Address first")
	}
	if _, e := p.scalar(n.children[0], true); e != nil {
		return e
	}
	for _, c := range n.children[1:] {
		if c.name == (xml.Name{Space: wsDiscoveryAddressing, Local: "Address"}) {
			return p.err("duplicate endpoint Address")
		}
	}
	return nil
}
func (p *winrmXML) selectors(n *wsDiscoveryElement) error {
	if e := p.container(n, "SelectorSet"); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, c := range n.children {
		if c.name != (xml.Name{Space: winrmWSMan, Local: "Selector"}) {
			return p.err("unexpected SelectorSet child")
		}
		name := ""
		for _, a := range c.attrs {
			if a.Name.Space == "" && a.Name.Local == "Name" {
				name = a.Value
			} else if a.Name.Space == "" || a.Name.Space == winrmWSMan {
				return p.err("invalid Selector attribute")
			}
		}
		if name == "" || seen[name] {
			return p.err("empty or duplicate selector name")
		}
		seen[name] = true
		if len(c.children) == 0 {
			if _, e := p.scalar(c, false); e != nil {
				return e
			}
			f := p.labels[c]
			f.Name = "Selector " + name
			p.labels[c] = f
		} else {
			if !xmlrpcSpace(c.text) || len(c.children) != 1 || c.children[0].name != (xml.Name{Space: wsDiscoveryAddressing, Local: "EndpointReference"}) {
				return p.err("invalid selector endpoint")
			}
			if e := p.container(c, "Selector "+name); e != nil {
				return e
			}
			if e := p.endpoint(c.children[0]); e != nil {
				return e
			}
		}
	}
	return nil
}
func (p *winrmXML) identify(n *wsDiscoveryElement, response bool) error {
	kind := "Identify"
	if response {
		kind = "IdentifyResponse"
	}
	if n.name.Local != kind {
		return p.err("Identify request/response direction mismatch")
	}
	if e := p.container(n, kind); e != nil {
		return e
	}
	for _, a := range n.attrs {
		if a.Name.Space == "" || a.Name.Space == n.name.Space {
			return p.err("invalid Identify attribute")
		}
	}
	if !response {
		for _, c := range n.children {
			if c.name.Space == "" || c.name.Space == n.name.Space {
				return p.err("Identify accepts only foreign extension children")
			}
		}
		return nil
	}
	phase, protocols := 0, 0
	seen := map[string]bool{}
	for _, c := range n.children {
		if c.name.Space != n.name.Space {
			if c.name.Space == "" || phase > 4 {
				return p.err("misplaced Identify extension")
			}
			phase = 4
			continue
		}
		if len(c.attrs) != 0 {
			return p.err("unexpected IdentifyResponse field attribute")
		}
		rank := map[string]int{"ProtocolVersion": 0, "ProductVendor": 1, "ProductVersion": 2, "InitiativeSupport": 3, "IntiativeSupport": 3, "SecurityProfiles": 5, "AddressingVersionURI": 6}[c.name.Local]
		if rank < phase {
			return p.err("IdentifyResponse element order")
		}
		phase = rank
		switch c.name.Local {
		case "ProtocolVersion":
			protocols++
			if _, e := p.scalar(c, true); e != nil {
				return e
			}
		case "ProductVendor", "ProductVersion":
			if seen[c.name.Local] {
				return p.err("duplicate " + c.name.Local)
			}
			seen[c.name.Local] = true
			if _, e := p.scalar(c, false); e != nil {
				return e
			}
		case "AddressingVersionURI":
			if _, e := p.scalar(c, true); e != nil {
				return e
			}
		case "InitiativeSupport", "IntiativeSupport":
			if e := p.container(c, c.name.Local); e != nil {
				return e
			}
			last := -1
			for _, v := range c.children {
				rank := 0
				if v.name.Local == "InitiativeVersion" {
					rank = 1
				} else if v.name.Local != "InitiativeName" {
					return p.err("unknown initiative child")
				}
				if v.name.Space != n.name.Space || rank <= last {
					return p.err("initiative order")
				}
				last = rank
				if _, e := p.scalar(v, false); e != nil {
					return e
				}
			}
		case "SecurityProfiles":
			if seen[c.name.Local] {
				return p.err("duplicate profile list")
			}
			seen[c.name.Local] = true
			if e := p.container(c, c.name.Local); e != nil {
				return e
			}
			for _, v := range c.children {
				if v.name != (xml.Name{Space: n.name.Space, Local: "SecurityProfileName"}) {
					return p.err("unknown profile entry")
				}
				if _, e := p.scalar(v, true); e != nil {
					return e
				}
			}
		default:
			return p.err("unknown IdentifyResponse field")
		}
	}
	if protocols == 0 {
		return p.err("IdentifyResponse requires ProtocolVersion")
	}
	return nil
}
func (p *winrmXML) resource(n *wsDiscoveryElement) {
	if len(n.children) == 0 {
		p.labels[n] = winrmTextField("Resource "+n.name.Local, 0, 0, n.text)
		return
	}
	p.labels[n] = winrmField{Name: "Resource " + n.name.Local}
	for _, c := range n.children {
		p.resource(c)
	}
}

// XML Schema non-negative duration syntax, checked without executing a regular
// expression or converting an unbounded decimal into a duration integer.
func winrmDuration(s string) bool {
	if len(s) < 3 || s[0] != 'P' {
		return false
	}
	s = s[1:]
	timePart, any, last := false, false, -1
	for len(s) > 0 {
		if s[0] == 'T' {
			if timePart {
				return false
			}
			timePart = true
			last = -1
			s = s[1:]
			if s == "" {
				return false
			}
			continue
		}
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 {
			return false
		}
		fraction := false
		if i < len(s) && s[i] == '.' {
			fraction = true
			i++
			first := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i == first {
				return false
			}
		}
		if i >= len(s) {
			return false
		}
		rank := -1
		if timePart {
			rank = strings.IndexByte("HMS", s[i])
		} else {
			rank = strings.IndexByte("YMD", s[i])
		}
		if rank < 0 || rank <= last || fraction && (!timePart || s[i] != 'S') {
			return false
		}
		last = rank
		any = true
		s = s[i+1:]
	}
	return any
}
