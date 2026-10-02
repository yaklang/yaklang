package stream_parser

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	wsDiscoveryNamespace    = "http://schemas.xmlsoap.org/ws/2005/04/discovery"
	wsDiscoveryAddressing   = "http://schemas.xmlsoap.org/ws/2004/08/addressing"
	wsDiscoverySOAP11       = "http://schemas.xmlsoap.org/soap/envelope/"
	wsDiscoverySOAP12       = "http://www.w3.org/2003/05/soap-envelope"
	wsDiscoveryXMLNamespace = "http://www.w3.org/XML/1998/namespace"
)

// WSDiscoveryMessage is a passive description of the April 2005 vocabulary.
// It does not contact endpoints, match scopes, correlate exchanges, or verify
// extension semantics. SOAP faults and the 2009/01 vocabulary are unsupported.
// Sources: https://specs.xmlsoap.org/ws/2005/04/discovery/ws-discovery.pdf
// and https://schemas.xmlsoap.org/ws/2005/04/discovery/ws-discovery.xsd .
type WSDiscoveryMessage struct {
	Version             string
	SOAPNamespace       string
	AddressingNamespace string
	Kind                string
	Header              WSDiscoveryHeader
	Probe               WSDiscoveryEndpoint
	Endpoints           []WSDiscoveryEndpoint
	Extensions          []WSDiscoveryExtension
	ExtensionAttributes []WSDiscoveryAttributes
}

type WSDiscoveryHeader struct {
	Action      string
	MessageID   string
	To          string
	RelatesTo   []WSDiscoveryRelationship
	ReplyTo     *WSDiscoveryEPR
	From        *WSDiscoveryEPR
	FaultTo     *WSDiscoveryEPR
	AppSequence *WSDiscoverySequence
}

type WSDiscoveryRelationship struct {
	MessageID string
	Type      xml.Name
}

type WSDiscoverySequence struct {
	InstanceID    uint32
	MessageNumber uint32
	SequenceID    string
}

type WSDiscoveryEPR struct {
	Address     string
	PortType    *xml.Name
	ServiceName *xml.Name
	PortName    string
}

type WSDiscoveryEndpoint struct {
	EPR             *WSDiscoveryEPR
	Types           []xml.Name
	TypesPresent    bool
	Scopes          []string
	ScopesPresent   bool
	MatchBy         string
	XAddrs          []string
	XAddrsPresent   bool
	MetadataVersion *uint32
}

// XML is the original bounded subtree. Namespaces preserves inherited bindings
// needed to interpret its prefixes; extension XML is never evaluated or fetched.
// Path includes the one-based element-sibling ordinal at each level, so opaque
// extensions under separate matches are not conflated.
type WSDiscoveryExtension struct {
	Path       string
	Name       xml.Name
	XML        string
	Namespaces map[string]string
}

type WSDiscoveryAttributes struct {
	Path       string
	Attributes []xml.Attr
	Namespaces map[string]string
}

type wsDiscoveryElement struct {
	name       xml.Name
	attrs      []xml.Attr
	ns         map[string]string
	text       string
	children   []*wsDiscoveryElement
	start, end int
	path       string
}

// XML 1.0 fifth-edition NCName ranges, without the colon allowed in Name.
func wsDiscoveryNCName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		start := c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' ||
			c >= 0xc0 && c <= 0xd6 || c >= 0xd8 && c <= 0xf6 || c >= 0xf8 && c <= 0x2ff ||
			c >= 0x370 && c <= 0x37d || c >= 0x37f && c <= 0x1fff || c >= 0x200c && c <= 0x200d ||
			c >= 0x2070 && c <= 0x218f || c >= 0x2c00 && c <= 0x2fef || c >= 0x3001 && c <= 0xd7ff ||
			c >= 0xf900 && c <= 0xfdcf || c >= 0xfdf0 && c <= 0xfffd || c >= 0x10000 && c <= 0xeffff
		if !start && !(i > 0 && (c == '-' || c == '.' || c >= '0' && c <= '9' || c == 0xb7 || c >= 0x300 && c <= 0x36f || c >= 0x203f && c <= 0x2040)) {
			return false
		}
	}
	return true
}

func wsDiscoveryQName(value string, ns map[string]string) (xml.Name, error) {
	parts := strings.Split(value, ":")
	if len(parts) > 2 || !wsDiscoveryNCName(parts[0]) || len(parts) == 2 && !wsDiscoveryNCName(parts[1]) {
		return xml.Name{}, fmt.Errorf("ws-discovery: invalid QName %q", value)
	}
	if len(parts) == 1 {
		return xml.Name{Space: ns[""], Local: parts[0]}, nil
	}
	uri, ok := ns[parts[0]]
	if !ok || uri == "" {
		return xml.Name{}, fmt.Errorf("ws-discovery: unbound QName prefix %q", parts[0])
	}
	return xml.Name{Space: uri, Local: parts[1]}, nil
}

func wsDiscoveryResolveName(name xml.Name, ns map[string]string, attribute bool) (xml.Name, error) {
	if !wsDiscoveryNCName(name.Local) || name.Space != "" && !wsDiscoveryNCName(name.Space) {
		return xml.Name{}, fmt.Errorf("ws-discovery: invalid XML name")
	}
	if name.Space == "" && attribute {
		return name, nil
	}
	uri, ok := ns[name.Space]
	if name.Space != "" && (!ok || uri == "") {
		return xml.Name{}, fmt.Errorf("ws-discovery: unbound namespace prefix %q", name.Space)
	}
	return xml.Name{Space: uri, Local: name.Local}, nil
}

func wsDiscoveryCloneNS(ns map[string]string) map[string]string {
	copy := make(map[string]string, len(ns))
	for k, v := range ns {
		copy[k] = v
	}
	return copy
}

// RawToken lets us reject unbound prefixes and mismatched lexical end tags,
// rather than accepting encoding/xml's permissive unknown-prefix translation.
// No DTD or non-initial PI is accepted and no entity resolver is installed.
func wsDiscoveryReadXML(text string, depthLimit int) (*wsDiscoveryElement, error) {
	decoder := xml.NewDecoder(strings.NewReader(text))
	nodes := 0
	var read func(xml.StartElement, int, int, map[string]string, string, int) (*wsDiscoveryElement, error)
	read = func(start xml.StartElement, offset, depth int, inherited map[string]string, path string, ordinal int) (*wsDiscoveryElement, error) {
		nodes++
		if depth > depthLimit || nodes > 8192 || len(start.Attr) > 64 {
			return nil, fmt.Errorf("ws-discovery: XML structure limit exceeded")
		}
		ns := inherited
		declarations := map[string]bool{}
		for _, attr := range start.Attr {
			prefix := attr.Name.Local
			if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
				prefix = ""
			} else if attr.Name.Space != "xmlns" {
				continue
			}
			if declarations[prefix] || prefix == "xmlns" || prefix != "" && !wsDiscoveryNCName(prefix) ||
				prefix == "xml" && attr.Value != wsDiscoveryXMLNamespace || prefix != "xml" && attr.Value == wsDiscoveryXMLNamespace ||
				attr.Value == "http://www.w3.org/2000/xmlns/" || prefix != "" && attr.Value == "" {
				return nil, fmt.Errorf("ws-discovery: invalid or duplicate namespace declaration")
			}
			if len(declarations) == 0 {
				ns = wsDiscoveryCloneNS(inherited)
			}
			declarations[prefix] = true
			ns[prefix] = attr.Value
		}
		if len(ns) > 64 {
			return nil, fmt.Errorf("ws-discovery: namespace limit exceeded")
		}
		name, err := wsDiscoveryResolveName(start.Name, ns, false)
		if err != nil {
			return nil, err
		}
		node := &wsDiscoveryElement{name: name, ns: ns, start: offset, path: fmt.Sprintf("%s/%s[%d]", path, name.Local, ordinal)}
		seen := map[xml.Name]bool{}
		for _, attr := range start.Attr {
			if attr.Name.Space == "xmlns" || attr.Name.Space == "" && attr.Name.Local == "xmlns" {
				continue
			}
			attr.Name, err = wsDiscoveryResolveName(attr.Name, ns, true)
			if err != nil {
				return nil, err
			}
			if seen[attr.Name] {
				return nil, fmt.Errorf("ws-discovery: duplicate attribute")
			}
			seen[attr.Name] = true
			node.attrs = append(node.attrs, attr)
		}
		var content strings.Builder
		for {
			childOffset := int(decoder.InputOffset())
			token, err := decoder.RawToken()
			if err != nil {
				return nil, fmt.Errorf("ws-discovery: XML: %w", err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				child, err := read(token, childOffset, depth+1, ns, node.path, len(node.children)+1)
				if err != nil {
					return nil, err
				}
				node.children = append(node.children, child)
			case xml.EndElement:
				if token.Name != start.Name {
					return nil, fmt.Errorf("ws-discovery: mismatched XML end tag")
				}
				node.text, node.end = content.String(), int(decoder.InputOffset())
				return node, nil
			case xml.CharData:
				content.Write(token)
			case xml.Comment:
			default:
				return nil, fmt.Errorf("ws-discovery: directives and processing instructions are unsupported")
			}
		}
	}
	var root *wsDiscoveryElement
	outsideComment, standaloneNo := false, false
	for {
		offset := int(decoder.InputOffset())
		token, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ws-discovery: XML: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if root != nil {
				return nil, fmt.Errorf("ws-discovery: multiple XML roots")
			}
			root, err = read(token, offset, 1, map[string]string{"xml": wsDiscoveryXMLNamespace}, "", 1)
			if err != nil {
				return nil, err
			}
		case xml.CharData:
			if !xmlrpcSpace(string(token)) {
				return nil, fmt.Errorf("ws-discovery: text outside XML root")
			}
		case xml.Comment:
			outsideComment = true
		case xml.ProcInst:
			if offset != 0 || token.Target != "xml" {
				return nil, fmt.Errorf("ws-discovery: unsupported processing instruction")
			}
			if err := validateXMLRPCDeclaration(xmlrpcDeclaration, text[:int(decoder.InputOffset())]); err != nil {
				return nil, fmt.Errorf("ws-discovery: %w", err)
			}
			// The declaration grammar above has already fixed all literal
			// values and attribute order, so only its standalone value remains.
			instruction := string(token.Inst)
			if index := strings.Index(instruction, "standalone"); index >= 0 {
				value := strings.TrimLeft(instruction[index+len("standalone"):], " \t\r\n=")
				standaloneNo = strings.HasPrefix(value, `"no"`) || strings.HasPrefix(value, "'no'")
			}
		default:
			return nil, fmt.Errorf("ws-discovery: unsupported XML token")
		}
	}
	if root == nil {
		return nil, fmt.Errorf("ws-discovery: missing XML root")
	}
	// SOAP 1.2 Part 1 section 5 permits comments only inside the Envelope,
	// and its document infoset permits standalone=yes or an absent value.
	// SOAP 1.1 does not impose these additional XML-document restrictions.
	if root.name.Space == wsDiscoverySOAP12 && (outsideComment || standaloneNo) {
		return nil, fmt.Errorf("ws-discovery: SOAP 1.2 disallows outside comments and standalone=no")
	}
	return root, nil
}

type wsDiscoveryParser struct {
	message *WSDiscoveryMessage
	text    string
}

func (p *wsDiscoveryParser) extension(n *wsDiscoveryElement) {
	p.message.Extensions = append(p.message.Extensions, WSDiscoveryExtension{n.path, n.name, p.text[n.start:n.end], wsDiscoveryCloneNS(n.ns)})
}

func (p *wsDiscoveryParser) attributes(n *wsDiscoveryElement, allowed ...string) (map[string]string, error) {
	values := map[string]string{}
	var extras []xml.Attr
	for _, a := range n.attrs {
		if a.Name.Space == "" {
			ok := false
			for _, key := range allowed {
				if a.Name.Local == key {
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("ws-discovery: unsupported attribute on %s", n.path)
			}
			values[a.Name.Local] = a.Value
		} else if a.Name.Space == n.name.Space {
			return nil, fmt.Errorf("ws-discovery: unexpected vocabulary attribute on %s", n.path)
		} else {
			extras = append(extras, a)
		}
	}
	if len(extras) > 0 {
		p.message.ExtensionAttributes = append(p.message.ExtensionAttributes, WSDiscoveryAttributes{n.path, extras, wsDiscoveryCloneNS(n.ns)})
	}
	return values, nil
}

func wsDiscoveryContainer(n *wsDiscoveryElement) error {
	if !xmlrpcSpace(n.text) {
		return fmt.Errorf("ws-discovery: mixed content in %s", n.path)
	}
	return nil
}

func wsDiscoveryLeaf(n *wsDiscoveryElement) (string, error) {
	if len(n.children) != 0 {
		return "", fmt.Errorf("ws-discovery: nested content in %s", n.path)
	}
	return strings.Trim(n.text, " \t\r\n"), nil
}

func wsDiscoveryURI(value string, absolute bool) error {
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("ws-discovery: empty URI or URI whitespace")
	}
	uri, err := url.Parse(value)
	if err != nil || absolute && uri.Scheme == "" {
		return fmt.Errorf("ws-discovery: invalid URI %q", value)
	}
	return nil
}

func wsDiscoveryUint(value string) (uint32, error) {
	value = strings.Trim(value, " \t\r\n")
	// The unsignedInt restriction is value-based: integer lexical -0 is zero.
	if strings.HasPrefix(value, "-") && len(value) > 1 && strings.Trim(value[1:], "0") == "" {
		return 0, nil
	}
	number, err := strconv.ParseUint(strings.TrimPrefix(value, "+"), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("ws-discovery: invalid unsignedInt %q", value)
	}
	return uint32(number), nil
}

func (p *wsDiscoveryParser) epr(n *wsDiscoveryElement) (*WSDiscoveryEPR, error) {
	if _, err := p.attributes(n); err != nil {
		return nil, err
	}
	if err := wsDiscoveryContainer(n); err != nil {
		return nil, err
	}
	out := &WSDiscoveryEPR{}
	rank := 0
	for _, child := range n.children {
		order := 6
		if child.name.Space == wsDiscoveryAddressing {
			switch child.name.Local {
			case "Address":
				order = 1
			case "ReferenceProperties":
				order = 2
			case "ReferenceParameters":
				order = 3
			case "PortType":
				order = 4
			case "ServiceName":
				order = 5
			default:
				return nil, fmt.Errorf("ws-discovery: unknown EPR field")
			}
		} else if child.name.Space == "" {
			return nil, fmt.Errorf("ws-discovery: unqualified EPR extension")
		}
		if order < rank || order == rank && order != 6 {
			return nil, fmt.Errorf("ws-discovery: duplicate or out-of-order EPR field")
		}
		rank = order
		if order == 6 {
			p.extension(child)
			continue
		}
		if order == 2 || order == 3 {
			if len(child.attrs) > 0 {
				return nil, fmt.Errorf("ws-discovery: attributes on EPR reference container")
			}
			if err := wsDiscoveryContainer(child); err != nil {
				return nil, err
			}
			p.extension(child)
			continue
		}
		attrs, err := p.attributes(child, "PortName")
		if err != nil {
			return nil, err
		}
		if order != 5 && len(attrs) > 0 {
			return nil, fmt.Errorf("ws-discovery: PortName outside ServiceName")
		}
		value, err := wsDiscoveryLeaf(child)
		if err != nil {
			return nil, err
		}
		if order == 1 {
			if err := wsDiscoveryURI(value, true); err != nil {
				return nil, err
			}
			out.Address = value
		} else {
			name, err := wsDiscoveryQName(value, child.ns)
			if err != nil {
				return nil, err
			}
			if order == 4 {
				out.PortType = &name
			} else {
				out.ServiceName, out.PortName = &name, attrs["PortName"]
			}
			if _, present := attrs["PortName"]; present && !wsDiscoveryNCName(out.PortName) {
				return nil, fmt.Errorf("ws-discovery: invalid PortName")
			}
		}
	}
	if out.Address == "" {
		return nil, fmt.Errorf("ws-discovery: missing EPR Address")
	}
	return out, nil
}

func (p *wsDiscoveryParser) endpoint(n *wsDiscoveryElement, kind string) (WSDiscoveryEndpoint, error) {
	out := WSDiscoveryEndpoint{}
	if _, err := p.attributes(n); err != nil {
		return out, err
	}
	if err := wsDiscoveryContainer(n); err != nil {
		return out, err
	}
	rank := 0
	for _, child := range n.children {
		order := 6
		if child.name == (xml.Name{Space: wsDiscoveryAddressing, Local: "EndpointReference"}) {
			order = 1
		} else if child.name.Space == wsDiscoveryNamespace {
			switch child.name.Local {
			case "Types":
				order = 2
			case "Scopes":
				order = 3
			case "XAddrs":
				order = 4
			case "MetadataVersion":
				order = 5
			default:
				return out, fmt.Errorf("ws-discovery: unknown discovery field")
			}
		} else if child.name.Space == "" {
			return out, fmt.Errorf("ws-discovery: unqualified discovery extension")
		}
		if order < rank || order == rank && order != 6 {
			return out, fmt.Errorf("ws-discovery: duplicate or out-of-order discovery field")
		}
		rank = order
		if order == 6 {
			p.extension(child)
			continue
		}
		if kind == "Resolve" && order != 1 || kind == "Probe" && order != 2 && order != 3 {
			return out, fmt.Errorf("ws-discovery: field not allowed in %s", kind)
		}
		if order == 1 {
			epr, err := p.epr(child)
			if err != nil {
				return out, err
			}
			out.EPR = epr
			continue
		}
		var allowed []string
		if order == 3 {
			allowed = []string{"MatchBy"}
		}
		attrs, err := p.attributes(child, allowed...)
		if err != nil {
			return out, err
		}
		// Types, XAddrs and MetadataVersion are schema simple types, not
		// open attributed types. Only Scopes has an attribute extension point.
		if order != 3 && len(child.attrs) != 0 {
			return out, fmt.Errorf("ws-discovery: attributes on simple discovery field")
		}
		value, err := wsDiscoveryLeaf(child)
		if err != nil {
			return out, err
		}
		switch order {
		case 2:
			out.TypesPresent = true
			for _, v := range strings.FieldsFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }) {
				name, err := wsDiscoveryQName(v, child.ns)
				if err != nil {
					return out, err
				}
				out.Types = append(out.Types, name)
			}
		case 3, 4:
			values := strings.FieldsFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' })
			for _, v := range values {
				if err := wsDiscoveryURI(v, order == 3); err != nil {
					return out, err
				}
			}
			if order == 3 {
				out.Scopes, out.ScopesPresent, out.MatchBy = values, true, attrs["MatchBy"]
				if _, ok := attrs["MatchBy"]; ok {
					if err := wsDiscoveryURI(out.MatchBy, true); err != nil {
						return out, err
					}
				}
				if out.MatchBy == "" {
					out.MatchBy = wsDiscoveryNamespace + "/rfc2396"
				}
			} else {
				out.XAddrs, out.XAddrsPresent = values, true
			}
		case 5:
			number, err := wsDiscoveryUint(value)
			if err != nil {
				return out, err
			}
			out.MetadataVersion = &number
		}
	}
	if kind != "Probe" && out.EPR == nil {
		return out, fmt.Errorf("ws-discovery: missing EndpointReference")
	}
	if (kind == "Hello" || kind == "ProbeMatch" || kind == "ResolveMatch") && out.MetadataVersion == nil {
		return out, fmt.Errorf("ws-discovery: missing MetadataVersion")
	}
	if kind == "ResolveMatch" && !out.XAddrsPresent {
		return out, fmt.Errorf("ws-discovery: missing XAddrs")
	}
	return out, nil
}

func (p *wsDiscoveryParser) header(n *wsDiscoveryElement) error {
	if _, err := p.attributes(n); err != nil {
		return err
	}
	if err := wsDiscoveryContainer(n); err != nil {
		return err
	}
	out := &p.message.Header
	seen := map[xml.Name]bool{}
	for _, child := range n.children {
		if child.name.Space != wsDiscoveryAddressing && child.name.Space != wsDiscoveryNamespace {
			if child.name.Space == "" || child.name.Space == p.message.SOAPNamespace {
				return fmt.Errorf("ws-discovery: invalid header namespace")
			}
			p.extension(child)
			continue
		}
		if seen[child.name] && child.name.Local != "RelatesTo" {
			return fmt.Errorf("ws-discovery: duplicate header")
		}
		seen[child.name] = true
		if child.name.Space == wsDiscoveryNamespace {
			if child.name.Local != "AppSequence" {
				return fmt.Errorf("ws-discovery: unsupported discovery header")
			}
			attrs, err := p.attributes(child, "InstanceId", "MessageNumber", "SequenceId")
			if err != nil {
				return err
			}
			value, err := wsDiscoveryLeaf(child)
			if err != nil {
				return err
			}
			if value != "" {
				return fmt.Errorf("ws-discovery: AppSequence must be empty")
			}
			instance, err := wsDiscoveryUint(attrs["InstanceId"])
			if err != nil {
				return err
			}
			number, err := wsDiscoveryUint(attrs["MessageNumber"])
			if err != nil {
				return err
			}
			if sequence, ok := attrs["SequenceId"]; ok {
				if err := wsDiscoveryURI(sequence, false); err != nil {
					return err
				}
			}
			out.AppSequence = &WSDiscoverySequence{instance, number, attrs["SequenceId"]}
			continue
		}
		switch child.name.Local {
		case "ReplyTo", "From", "FaultTo":
			epr, err := p.epr(child)
			if err != nil {
				return err
			}
			switch child.name.Local {
			case "ReplyTo":
				out.ReplyTo = epr
			case "From":
				out.From = epr
			case "FaultTo":
				out.FaultTo = epr
			}
		case "Action", "MessageID", "To", "RelatesTo":
			var allowed []string
			if child.name.Local == "RelatesTo" {
				allowed = []string{"RelationshipType"}
			}
			attrs, err := p.attributes(child, allowed...)
			if err != nil {
				return err
			}
			value, err := wsDiscoveryLeaf(child)
			if err != nil {
				return err
			}
			if err := wsDiscoveryURI(value, true); err != nil {
				return err
			}
			switch child.name.Local {
			case "Action":
				out.Action = value
			case "MessageID":
				out.MessageID = value
			case "To":
				out.To = value
			case "RelatesTo":
				kind := xml.Name{Space: wsDiscoveryAddressing, Local: "Reply"}
				if value, exists := attrs["RelationshipType"]; exists {
					kind, err = wsDiscoveryQName(strings.Trim(value, " \t\r\n"), child.ns)
					if err != nil {
						return err
					}
				}
				out.RelatesTo = append(out.RelatesTo, WSDiscoveryRelationship{value, kind})
			}
		default:
			return fmt.Errorf("ws-discovery: unsupported addressing header")
		}
	}
	if out.Action == "" || out.MessageID == "" || out.To == "" {
		return fmt.Errorf("ws-discovery: missing required addressing header")
	}
	return nil
}

func decodeWSDiscoveryText(text string, depthLimit int) (*WSDiscoveryMessage, error) {
	if len(text) > 1<<20 || !utf8.ValidString(text) || depthLimit < 1 || depthLimit > 128 {
		return nil, fmt.Errorf("ws-discovery: invalid text size, encoding or depth limit")
	}
	text = strings.TrimPrefix(text, "\ufeff")
	root, err := wsDiscoveryReadXML(text, depthLimit)
	if err != nil {
		return nil, err
	}
	if root.name.Local != "Envelope" || root.name.Space != wsDiscoverySOAP11 && root.name.Space != wsDiscoverySOAP12 {
		return nil, fmt.Errorf("ws-discovery: unsupported SOAP envelope")
	}
	out := &WSDiscoveryMessage{Version: "2005/04", SOAPNamespace: root.name.Space, AddressingNamespace: wsDiscoveryAddressing}
	p := &wsDiscoveryParser{out, text}
	if _, err := p.attributes(root); err != nil {
		return nil, err
	}
	if err := wsDiscoveryContainer(root); err != nil {
		return nil, err
	}
	if len(root.children) != 2 || root.children[0].name != (xml.Name{Space: root.name.Space, Local: "Header"}) || root.children[1].name != (xml.Name{Space: root.name.Space, Local: "Body"}) {
		return nil, fmt.Errorf("ws-discovery: expected SOAP Header then Body")
	}
	if err := p.header(root.children[0]); err != nil {
		return nil, err
	}
	body := root.children[1]
	if _, err := p.attributes(body); err != nil {
		return nil, err
	}
	if err := wsDiscoveryContainer(body); err != nil {
		return nil, err
	}
	if len(body.children) != 1 || body.children[0].name.Space != wsDiscoveryNamespace {
		return nil, fmt.Errorf("ws-discovery: expected one 2005/04 discovery body")
	}
	body = body.children[0]
	out.Kind = body.name.Local
	if out.Header.Action != wsDiscoveryNamespace+"/"+out.Kind {
		return nil, fmt.Errorf("ws-discovery: Action and body differ")
	}
	switch out.Kind {
	case "Hello", "Bye", "Probe", "Resolve":
		endpoint, err := p.endpoint(body, out.Kind)
		if err != nil {
			return nil, err
		}
		if out.Kind == "Probe" {
			out.Probe = endpoint
		} else {
			out.Endpoints = append(out.Endpoints, endpoint)
		}
	case "ProbeMatches", "ResolveMatches":
		if _, err := p.attributes(body); err != nil {
			return nil, err
		}
		if err := wsDiscoveryContainer(body); err != nil {
			return nil, err
		}
		extensionSeen := false
		for _, child := range body.children {
			if child.name == (xml.Name{Space: wsDiscoveryNamespace, Local: strings.TrimSuffix(out.Kind, "es")}) {
				if extensionSeen || out.Kind == "ResolveMatches" && len(out.Endpoints) > 0 {
					return nil, fmt.Errorf("ws-discovery: unexpected match cardinality or order")
				}
				endpoint, err := p.endpoint(child, child.name.Local)
				if err != nil {
					return nil, err
				}
				out.Endpoints = append(out.Endpoints, endpoint)
			} else {
				if child.name.Space == "" || child.name.Space == wsDiscoveryNamespace {
					return nil, fmt.Errorf("ws-discovery: unexpected match element")
				}
				extensionSeen = true
				p.extension(child)
			}
		}
	default:
		return nil, fmt.Errorf("ws-discovery: unsupported discovery message kind")
	}
	if out.Kind == "Hello" || out.Kind == "Bye" || out.Kind == "ProbeMatches" || out.Kind == "ResolveMatches" {
		if out.Header.AppSequence == nil {
			return nil, fmt.Errorf("ws-discovery: missing AppSequence")
		}
	}
	if out.Kind == "ProbeMatches" || out.Kind == "ResolveMatches" {
		reply := false
		for _, relation := range out.Header.RelatesTo {
			if relation.Type == (xml.Name{Space: wsDiscoveryAddressing, Local: "Reply"}) {
				reply = true
			}
		}
		if !reply {
			return nil, fmt.Errorf("ws-discovery: missing reply RelatesTo")
		}
	}
	return out, nil
}
