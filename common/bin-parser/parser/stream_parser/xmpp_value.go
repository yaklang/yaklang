package stream_parser

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	xmppStreamNS = "http://etherx.jabber.org/streams"
	xmppClientNS = "jabber:client"
	xmppServerNS = "jabber:server"
	xmppSASLNS   = "urn:ietf:params:xml:ns:xmpp-sasl"
	xmppTLSNS    = "urn:ietf:params:xml:ns:xmpp-tls"
	xmppBindNS   = "urn:ietf:params:xml:ns:xmpp-bind"
	xmppStanzaNS = "urn:ietf:params:xml:ns:xmpp-stanzas"
)

// XMPPElement retains both expanded and lexical names, ordered attributes and
// mixed content. Offsets are bytes relative to the caller's bounded input.
// Extensions are parsed as namespace-aware fields, not claimed as implemented
// XEP operations. No external action or network I/O is performed.
type XMPPElement struct {
	Name, LexicalName xml.Name
	Attributes        []xml.Attr
	NamespaceAttrs    []xml.Attr
	Namespaces        map[string]string
	Language          string
	Text, Raw         string
	Children          []*XMPPElement
	Content           []XMPPContent
	Offset, End       int
}

type XMPPContent struct {
	Text  string
	Child *XMPPElement
}

type XMPPLanguageText struct{ Language, Text string }

type XMPPStanza struct {
	Kind, Type, ID, From, To, Language string
	Bodies, Subjects, Status           []XMPPLanguageText
	Thread, ThreadParent, Show         string
	ThreadPresent                      bool
	Priority                           int8
	PriorityPresent                    bool
	Payloads                           []*XMPPElement
	Error                              *XMPPError
	BindResource, BindJID              string
	BindPresent, SessionPresent        bool
}

type XMPPError struct {
	Type, By, LegacyCode, Condition, ConditionText string
	Texts                                          []XMPPLanguageText
	Extensions                                     []*XMPPElement
}

type XMPPEvent struct {
	Kind       string
	Element    *XMPPElement
	Stanza     *XMPPStanza
	Mechanism  string
	Mechanisms []string
	Token      []byte // Base64 decoded only; mechanism-specific token is opaque.
	Condition  string
	Texts      []XMPPLanguageText
	Extensions []*XMPPElement
}

// XMPPMessage is a bounded sequence of complete framing events, not a generic
// XML document and not a validated session. An open stream at EOF is legal, but
// a partial stanza/prolog is not. StreamHeader is an explicit continuation
// token owned by the caller; the decoder never stores cross-input state.
// Sources: RFC 6120 sections 4, 5, 6, 7, 8, 11; RFC 6121 sections 4.7 and 5.2.
type XMPPMessage struct {
	Events                   []XMPPEvent
	Declarations             []string
	StreamHeader             string
	CallerContextUsed        bool
	ContextRequired          bool // Wire XML only; no namespace or stanza is invented.
	StreamOpen, StreamClosed bool
	Restarts                 int
	ConnectionStateValidated bool
}

type xmppReader struct {
	text    string
	decoder *xml.Decoder
	nodes   int
}

func xmppFailure(message string) error { return fmt.Errorf("xmpp: %s", message) }

func xmppAttribute(n *XMPPElement, space, local string) (string, bool) {
	for _, attr := range n.Attributes {
		if attr.Name == (xml.Name{Space: space, Local: local}) {
			return attr.Value, true
		}
	}
	return "", false
}

func (r *xmppReader) start(token xml.StartElement, offset, depth int, inherited map[string]string, language string) (*XMPPElement, error) {
	r.nodes++
	if depth > 64 || r.nodes > 8192 || len(token.Attr) > 64 {
		return nil, xmppFailure("XML structure limit exceeded")
	}
	ns := inherited
	declared := map[string]bool{}
	n := &XMPPElement{LexicalName: token.Name, Offset: offset, Language: language}
	for _, attr := range token.Attr {
		prefix := attr.Name.Local
		if attr.Name.Space == "" && prefix == "xmlns" {
			prefix = ""
		} else if attr.Name.Space != "xmlns" {
			continue
		}
		if declared[prefix] || prefix == "xmlns" || prefix != "" && !wsDiscoveryNCName(prefix) ||
			prefix == "xml" && attr.Value != wsDiscoveryXMLNamespace || prefix != "xml" && attr.Value == wsDiscoveryXMLNamespace ||
			attr.Value == "http://www.w3.org/2000/xmlns/" || prefix != "" && attr.Value == "" {
			return nil, xmppFailure("invalid or duplicate namespace declaration")
		}
		if len(declared) == 0 {
			ns = wsDiscoveryCloneNS(inherited)
		}
		declared[prefix] = true
		ns[prefix] = attr.Value
		n.NamespaceAttrs = append(n.NamespaceAttrs, attr)
	}
	if len(ns) > 64 {
		return nil, xmppFailure("namespace limit exceeded")
	}
	name, err := wsDiscoveryResolveName(token.Name, ns, false)
	if err != nil {
		return nil, fmt.Errorf("xmpp: namespace context required or invalid name: %w", err)
	}
	n.Name, n.Namespaces = name, ns
	if (name.Space == xmppClientNS || name.Space == xmppServerNS) && token.Name.Space != "" {
		return nil, xmppFailure("content namespace elements must be unprefixed")
	}
	seen := map[xml.Name]bool{}
	for _, attr := range token.Attr {
		if attr.Name.Space == "xmlns" || attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			continue
		}
		attr.Name, err = wsDiscoveryResolveName(attr.Name, ns, true)
		if err != nil {
			return nil, fmt.Errorf("xmpp: attribute namespace: %w", err)
		}
		if seen[attr.Name] {
			return nil, xmppFailure("duplicate attribute")
		}
		seen[attr.Name] = true
		n.Attributes = append(n.Attributes, attr)
		if attr.Name == (xml.Name{Space: wsDiscoveryXMLNamespace, Local: "lang"}) {
			n.Language = attr.Value
		}
	}
	n.End = int(r.decoder.InputOffset())
	n.Raw = r.text[n.Offset:n.End]
	return n, nil
}

func (r *xmppReader) element(start xml.StartElement, offset, depth int, ns map[string]string, lang string) (*XMPPElement, error) {
	n, err := r.start(start, offset, depth, ns, lang)
	if err != nil {
		return nil, err
	}
	var text strings.Builder
	for {
		begin := int(r.decoder.InputOffset())
		token, err := r.decoder.RawToken()
		if err != nil {
			return nil, fmt.Errorf("xmpp: incomplete or malformed element: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			child, err := r.element(token, begin, depth+1, n.Namespaces, n.Language)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
			n.Content = append(n.Content, XMPPContent{Child: child})
		case xml.EndElement:
			if token.Name != start.Name {
				return nil, xmppFailure("mismatched lexical end tag")
			}
			n.Text, n.End = text.String(), int(r.decoder.InputOffset())
			n.Raw = r.text[n.Offset:n.End]
			return n, nil
		case xml.CharData:
			part := string(token)
			text.WriteString(part)
			n.Content = append(n.Content, XMPPContent{Text: part})
		default:
			return nil, xmppFailure("restricted XML comment, directive or processing instruction")
		}
	}
}

func decodeXMPPText(text, streamHeader string) (*XMPPMessage, error) {
	if len(text) == 0 || len(text) > 1<<20 || !utf8.ValidString(text) || strings.HasPrefix(text, "\ufeff") || len(streamHeader) > 64<<10 {
		return nil, xmppFailure("invalid text size, UTF-8 or context size")
	}
	var initial *XMPPElement
	if streamHeader != "" {
		context, err := decodeXMPPText(streamHeader, "")
		if err != nil || len(context.Events) != 1 || context.Events[0].Kind != "stream-open" || !context.StreamOpen {
			return nil, xmppFailure("caller context must contain exactly one stream opening")
		}
		initial = context.Events[0].Element
	}
	r := &xmppReader{text: text, decoder: xml.NewDecoder(strings.NewReader(text))}
	m := &XMPPMessage{CallerContextUsed: initial != nil, StreamOpen: initial != nil, StreamHeader: streamHeader}
	current := initial
	pendingDeclaration := false
	for {
		offset := int(r.decoder.InputOffset())
		token, err := r.decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xmpp: incomplete or malformed framing: %w", err)
		}
		ns, lang := map[string]string{"xml": wsDiscoveryXMLNamespace}, ""
		if current != nil {
			ns, lang = current.Namespaces, current.Language
		}
		switch token := token.(type) {
		case xml.StartElement:
			if len(m.Events) >= 4096 {
				return nil, xmppFailure("event limit exceeded")
			}
			// A stream restart replaces inherited namespace bindings. The
			// opposite direction may contain the SASL success, so this single
			// direction parser never claims to validate restart negotiation.
			if token.Name.Local == "stream" {
				if m.ContextRequired {
					return nil, xmppFailure("unresolved fragments cannot be mixed with stream framing")
				}
				n, err := r.start(token, offset, 1, map[string]string{"xml": wsDiscoveryXMLNamespace}, "")
				if err != nil {
					return nil, err
				}
				version, _ := xmppAttribute(n, "", "version")
				defaultNS := n.Namespaces[""]
				if n.Name != (xml.Name{Space: xmppStreamNS, Local: "stream"}) ||
					(defaultNS != xmppClientNS && defaultNS != xmppServerNS && defaultNS != xmppStreamNS) || version != "1.0" || strings.HasSuffix(n.Raw, "/>") || len(n.Raw) > 64<<10 {
					return nil, xmppFailure("unsupported or invalid XMPP 1.0 stream opening")
				}
				if current != nil {
					m.Restarts++
				}
				current, m.StreamHeader, m.StreamOpen, m.StreamClosed = n, n.Raw, true, false
				m.Events = append(m.Events, XMPPEvent{Kind: "stream-open", Element: n})
				pendingDeclaration = false
				continue
			}
			if pendingDeclaration || m.StreamClosed {
				return nil, xmppFailure("element outside open stream framing")
			}
			n, err := r.element(token, offset, 1, ns, lang)
			if err != nil {
				return nil, err
			}
			if current == nil && n.Name.Space == "" && n.LexicalName.Space == "" && xmppMember(n.Name.Local, "iq", "message", "presence") {
				if len(m.Events) != 0 && !m.ContextRequired {
					return nil, xmppFailure("resolved events cannot be mixed with unresolved fragments")
				}
				for _, attr := range n.NamespaceAttrs {
					if attr.Name == (xml.Name{Local: "xmlns"}) {
						return nil, xmppFailure("explicit empty content namespace is not XMPP context")
					}
				}
				m.ContextRequired = true
				m.Events = append(m.Events, XMPPEvent{Kind: "xml-fragment", Element: n})
				continue
			}
			if m.ContextRequired {
				return nil, xmppFailure("unresolved fragments cannot be mixed with resolved events")
			}
			event, err := xmppDecodeEvent(n)
			if err != nil {
				return nil, err
			}
			m.Events = append(m.Events, event)
		case xml.EndElement:
			if len(m.Events) >= 4096 {
				return nil, xmppFailure("event limit exceeded")
			}
			if current == nil && !m.StreamClosed && !pendingDeclaration && len(m.Events) == 0 && token.Name == (xml.Name{Space: "stream", Local: "stream"}) {
				end := int(r.decoder.InputOffset())
				m.ContextRequired, m.StreamClosed = true, true
				m.Events = append(m.Events, XMPPEvent{Kind: "stream-close-context-required", Element: &XMPPElement{LexicalName: token.Name, Offset: offset, End: end, Raw: text[offset:end]}})
				continue
			}
			if current == nil || token.Name != current.LexicalName || pendingDeclaration {
				return nil, xmppFailure("stream closing requires matching caller or input context")
			}
			end := int(r.decoder.InputOffset())
			m.Events = append(m.Events, XMPPEvent{Kind: "stream-close", Element: &XMPPElement{Name: current.Name, LexicalName: token.Name, Offset: offset, End: end, Raw: text[offset:end], Namespaces: current.Namespaces}})
			current, m.StreamHeader, m.StreamOpen, m.StreamClosed = nil, "", false, true
		case xml.CharData:
			if !xmlrpcSpace(string(token)) {
				return nil, xmppFailure("non-whitespace text outside stanza")
			}
		case xml.ProcInst:
			declaration := text[offset:int(r.decoder.InputOffset())]
			if token.Target != "xml" || pendingDeclaration || strings.Contains(declaration, "standalone") || len(m.Events) == 0 && offset != 0 {
				return nil, xmppFailure("restricted processing instruction or standalone declaration")
			}
			if err := validateXMLRPCDeclaration(xmlrpcDeclaration, declaration); err != nil {
				return nil, fmt.Errorf("xmpp: invalid declaration: %w", err)
			}
			pendingDeclaration = true
			m.Declarations = append(m.Declarations, declaration)
		default:
			return nil, xmppFailure("restricted XML comment or directive")
		}
	}
	if pendingDeclaration || len(m.Events) == 0 {
		return nil, xmppFailure("incomplete XMPP framing or namespace context required")
	}
	return m, nil
}

func xmppEmpty(n *XMPPElement) bool { return len(n.Children) == 0 && xmlrpcSpace(n.Text) }

func xmppDecodeEvent(n *XMPPElement) (XMPPEvent, error) {
	e := XMPPEvent{Element: n}
	switch n.Name.Space {
	case xmppClientNS, xmppServerNS:
		stanza, err := xmppDecodeStanza(n)
		e.Kind, e.Stanza = "stanza", stanza
		return e, err
	case xmppStreamNS:
		if !xmlrpcSpace(n.Text) {
			return e, xmppFailure("text outside stream feature/error child")
		}
		switch n.Name.Local {
		case "features":
			e.Kind = "features"
			for _, child := range n.Children {
				if child.Name == (xml.Name{Space: xmppSASLNS, Local: "mechanisms"}) {
					if !xmlrpcSpace(child.Text) || len(child.Children) == 0 {
						return e, xmppFailure("invalid mechanism list")
					}
					count := 0
					for _, mechanism := range child.Children {
						if mechanism.Name.Space != xmppSASLNS {
							e.Extensions = append(e.Extensions, mechanism)
							continue
						}
						if mechanism.Name != (xml.Name{Space: xmppSASLNS, Local: "mechanism"}) || len(mechanism.Children) != 0 || !xmppMechanism(mechanism.Text) {
							return e, xmppFailure("invalid mechanism name")
						}
						e.Mechanisms = append(e.Mechanisms, mechanism.Text)
						count++
					}
					if count == 0 {
						return e, xmppFailure("missing mechanism name")
					}
				} else {
					e.Extensions = append(e.Extensions, child)
				}
			}
		case "error":
			e.Kind = "stream-error"
			error, err := xmppDecodeError(n, "urn:ietf:params:xml:ns:xmpp-streams")
			if err != nil {
				return e, err
			}
			e.Condition, e.Texts, e.Extensions = error.Condition, error.Texts, error.Extensions
		default:
			return e, xmppFailure("unsupported stream element")
		}
	case xmppSASLNS:
		e.Kind = "sasl-" + n.Name.Local
		switch n.Name.Local {
		case "auth", "challenge", "response", "success":
			if len(n.Children) != 0 {
				return e, xmppFailure("SASL token contains elements")
			}
			if n.Name.Local == "auth" {
				e.Mechanism, _ = xmppAttribute(n, "", "mechanism")
				if !xmppMechanism(e.Mechanism) {
					return e, xmppFailure("SASL auth mechanism is required")
				}
			}
			if n.Text != "" && n.Text != "=" {
				if strings.ContainsAny(n.Text, " \t\r\n") {
					return e, xmppFailure("whitespace in SASL base64 token")
				}
				var err error
				e.Token, err = base64.StdEncoding.Strict().DecodeString(n.Text)
				if err != nil {
					return e, xmppFailure("invalid SASL base64 token")
				}
			}
		case "failure":
			error, err := xmppDecodeError(n, xmppSASLNS)
			if err != nil {
				return e, err
			}
			e.Condition, e.Texts, e.Extensions = error.Condition, error.Texts, error.Extensions
		case "abort":
			if !xmppEmpty(n) {
				return e, xmppFailure("SASL abort is not empty")
			}
		default:
			return e, xmppFailure("unsupported SASL element")
		}
	case xmppTLSNS:
		if n.Name.Local != "starttls" && n.Name.Local != "proceed" && n.Name.Local != "failure" || !xmppEmpty(n) {
			return e, xmppFailure("unsupported or nonempty TLS negotiation element")
		}
		e.Kind = "tls-" + n.Name.Local
	default:
		return e, xmppFailure("namespace context required or unsupported top-level vocabulary")
	}
	return e, nil
}

func xmppDecodeStanza(n *XMPPElement) (*XMPPStanza, error) {
	s := &XMPPStanza{Kind: n.Name.Local, Language: n.Language}
	var typePresent bool
	s.Type, typePresent = xmppAttribute(n, "", "type")
	s.ID, _ = xmppAttribute(n, "", "id")
	s.From, _ = xmppAttribute(n, "", "from")
	s.To, _ = xmppAttribute(n, "", "to")
	if !xmlrpcSpace(n.Text) || n.Name.Space == xmppServerNS && (s.From == "" || s.To == "") {
		return nil, xmppFailure("invalid stanza text or server addressing")
	}
	switch s.Kind {
	case "message":
		if !typePresent {
			s.Type = "normal"
		}
		if !xmppMember(s.Type, "normal", "chat", "groupchat", "headline", "error") {
			return nil, xmppFailure("unsupported message type")
		}
	case "presence":
		if !typePresent {
			s.Type = "available"
		}
		if typePresent && !xmppMember(s.Type, "unavailable", "subscribe", "subscribed", "unsubscribe", "unsubscribed", "probe", "error") {
			return nil, xmppFailure("unsupported presence type")
		}
	case "iq":
		if s.ID == "" || !xmppMember(s.Type, "get", "set", "result", "error") {
			return nil, xmppFailure("IQ requires id and supported type")
		}
	default:
		return nil, xmppFailure("unsupported core stanza")
	}
	seen := map[string]bool{}
	for _, child := range n.Children {
		if child.Name.Space == n.Name.Space && child.Name.Local == "error" {
			if s.Error != nil {
				return nil, xmppFailure("duplicate stanza error")
			}
			var err error
			s.Error, err = xmppDecodeError(child, xmppStanzaNS)
			if err != nil {
				return nil, err
			}
			if !xmppMember(s.Error.Type, "auth", "cancel", "continue", "modify", "wait") {
				return nil, xmppFailure("invalid stanza error type")
			}
			continue
		}
		if s.Kind == "message" && child.Name.Space == n.Name.Space && xmppMember(child.Name.Local, "body", "subject", "thread") ||
			s.Kind == "presence" && child.Name.Space == n.Name.Space && xmppMember(child.Name.Local, "show", "status", "priority") {
			key := child.Name.Local
			if xmppMember(key, "body", "subject", "status") {
				key += "/" + strings.ToLower(child.Language)
			}
			if seen[key] || len(child.Children) != 0 {
				return nil, xmppFailure("duplicate or nested scalar stanza field")
			}
			seen[key] = true
			value := XMPPLanguageText{child.Language, child.Text}
			switch child.Name.Local {
			case "body":
				s.Bodies = append(s.Bodies, value)
			case "subject":
				s.Subjects = append(s.Subjects, value)
			case "status":
				s.Status = append(s.Status, value)
			case "thread":
				s.Thread, s.ThreadPresent = child.Text, true
				s.ThreadParent, _ = xmppAttribute(child, "", "parent")
			case "show":
				if !xmppMember(child.Text, "away", "chat", "dnd", "xa") {
					return nil, xmppFailure("invalid presence show")
				}
				s.Show = child.Text
			case "priority":
				value, err := strconv.ParseInt(child.Text, 10, 8)
				if err != nil {
					return nil, xmppFailure("invalid presence priority")
				}
				s.Priority, s.PriorityPresent = int8(value), true
			}
			continue
		}
		s.Payloads = append(s.Payloads, child)
		if s.Kind == "iq" && child.Name == (xml.Name{Space: xmppBindNS, Local: "bind"}) {
			if s.BindPresent || !xmlrpcSpace(child.Text) || len(child.Children) > 1 {
				return nil, xmppFailure("invalid binding payload")
			}
			s.BindPresent = true
			for _, value := range child.Children {
				if value.Name.Space != xmppBindNS || !xmppMember(value.Name.Local, "resource", "jid") || len(value.Children) != 0 {
					return nil, xmppFailure("invalid binding value")
				}
				if value.Name.Local == "resource" {
					s.BindResource = value.Text
				} else {
					s.BindJID = value.Text
				}
			}
			if s.Type == "get" || s.Type == "result" && (len(child.Children) != 1 || s.BindJID == "") || s.Type == "set" && len(child.Children) != 0 && (s.BindResource == "" || s.BindJID != "") {
				return nil, xmppFailure("binding request/result value mismatch")
			}
		}
		if s.Kind == "iq" && child.Name == (xml.Name{Space: "urn:ietf:params:xml:ns:xmpp-session", Local: "session"}) {
			s.SessionPresent = true // Legacy optional session feature; no state claim.
		}
	}
	if s.Type == "error" && s.Error == nil || s.Type != "error" && s.Error != nil {
		return nil, xmppFailure("stanza error/type mismatch")
	}
	if s.Kind == "iq" && (xmppMember(s.Type, "get", "set") && len(s.Payloads) != 1 || xmppMember(s.Type, "result", "error") && len(s.Payloads) > 1) {
		return nil, xmppFailure("IQ payload cardinality")
	}
	return s, nil
}

func xmppMember(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

// RFC 4422 section 3.1, shared by the advertised and selected mechanisms.
func xmppMechanism(value string) bool {
	if len(value) < 1 || len(value) > 20 {
		return false
	}
	for i := range value {
		c := value[i]
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func xmppDecodeError(n *XMPPElement, namespace string) (*XMPPError, error) {
	e := &XMPPError{}
	e.Type, _ = xmppAttribute(n, "", "type")
	e.By, _ = xmppAttribute(n, "", "by")
	e.LegacyCode, _ = xmppAttribute(n, "", "code")
	if !xmlrpcSpace(n.Text) {
		return nil, xmppFailure("text outside error child")
	}
	for _, child := range n.Children {
		if child.Name.Space != namespace {
			e.Extensions = append(e.Extensions, child)
			continue
		}
		if len(child.Children) != 0 {
			return nil, xmppFailure("nested error condition")
		}
		if child.Name.Local == "text" {
			e.Texts = append(e.Texts, XMPPLanguageText{child.Language, child.Text})
			continue
		}
		if e.Condition != "" {
			return nil, xmppFailure("multiple error conditions")
		}
		e.Condition, e.ConditionText = child.Name.Local, child.Text
	}
	if e.Condition == "" {
		return nil, xmppFailure("missing error condition")
	}
	return e, nil
}
