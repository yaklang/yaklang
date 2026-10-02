package stream_parser

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// RFC 3525 Annex B, version 1 textual structural profile. Decimal fields are
// strings on the wire, not binary integers; their checked value is metadata.
// Package values and SDP are preserved, never executed or semantically decoded.
const megacoMaxBytes = 65527
const megacoMaxNodes = 16384
const megacoMaxDepth = 32

type megacoField struct {
	Name, Type string
	Start, End int // octets relative to this message
	Children   []megacoField
	Info       map[string]any
}

type megacoReader struct {
	wire              []byte
	pos, nodes, depth int
	version           uint64
	err               error
	current           *megacoField
}

var megacoKeywords = map[string]string{
	"transaction": "Transaction", "t": "Transaction", "reply": "Reply", "p": "Reply", "pending": "Pending", "pn": "Pending", "transactionresponseack": "TransactionResponseAck", "k": "TransactionResponseAck",
	"context": "Context", "c": "Context", "immackrequired": "ImmAckRequired", "ia": "ImmAckRequired",
	"add": "Add", "a": "Add", "modify": "Modify", "mf": "Modify", "move": "Move", "mv": "Move", "subtract": "Subtract", "s": "Subtract", "auditvalue": "AuditValue", "av": "AuditValue", "auditcapability": "AuditCapability", "ac": "AuditCapability", "notify": "Notify", "n": "Notify", "servicechange": "ServiceChange", "sc": "ServiceChange",
	"media": "Media", "m": "Media", "stream": "Stream", "st": "Stream", "local": "Local", "l": "Local", "remote": "Remote", "r": "Remote", "localcontrol": "LocalControl", "o": "LocalControl", "terminationstate": "TerminationState", "ts": "TerminationState",
	"mode": "Mode", "mo": "Mode", "reservedvalue": "ReservedValue", "rv": "ReservedValue", "reservedgroup": "ReservedGroup", "rg": "ReservedGroup", "servicestates": "ServiceStates", "si": "ServiceStates", "buffer": "Buffer", "bf": "Buffer",
	"signals": "Signals", "sg": "Signals", "events": "Events", "e": "Events", "observedevents": "ObservedEvents", "oe": "ObservedEvents", "eventbuffer": "EventBuffer", "eb": "EventBuffer", "audit": "Audit", "at": "Audit", "error": "Error", "er": "Error", "services": "Services", "sv": "Services",
	"statistics": "Statistics", "sa": "Statistics", "packages": "Packages", "pg": "Packages", "digitmap": "DigitMap", "dm": "DigitMap", "modem": "Modem", "md": "Modem", "mux": "Mux", "mx": "Mux",
	"priority": "Priority", "pr": "Priority", "emergency": "Emergency", "eg": "Emergency", "topology": "Topology", "tp": "Topology", "contextaudit": "ContextAudit", "ca": "ContextAudit",
	"method": "Method", "mt": "Method", "reason": "Reason", "re": "Reason", "delay": "Delay", "dl": "Delay", "servicechangeaddress": "ServiceChangeAddress", "ad": "ServiceChangeAddress", "mgcidtotry": "MgcIdToTry", "mg": "MgcIdToTry", "profile": "Profile", "pf": "Profile", "version": "Version", "v": "Version",
	"signallist": "SignalList", "sl": "SignalList", "signaltype": "SignalType", "sy": "SignalType", "duration": "Duration", "dr": "Duration", "notifycompletion": "NotifyCompletion", "nc": "NotifyCompletion", "keepactive": "KeepActive", "ka": "KeepActive", "embed": "Embed", "em": "Embed",
}

func (p *megacoReader) fail(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf("megaco: offset %d: %s", p.pos, fmt.Sprintf(format, args...))
	}
}

func (p *megacoReader) add(f megacoField) {
	p.nodes++
	if p.nodes > megacoMaxNodes {
		p.fail("field-node resource limit exceeded")
		return
	}
	p.current.Children = append(p.current.Children, f)
}

func (p *megacoReader) leaf(name, typ string, start int, info map[string]any) {
	if p.err == nil {
		p.add(megacoField{Name: name, Type: typ, Start: start, End: p.pos, Info: info})
	}
}

func (p *megacoReader) group(name string, fn func()) {
	if p.err != nil {
		return
	}
	p.depth++
	if p.depth > megacoMaxDepth {
		p.fail("nesting resource limit exceeded")
		p.depth--
		return
	}
	parent := p.current
	child := &megacoField{Name: name, Start: p.pos}
	p.current = child
	fn()
	child.End = p.pos
	p.current = parent
	p.add(*child)
	p.depth--
}

func (p *megacoReader) ws() {
	if p.err != nil {
		return
	}
	start := p.pos
	for p.pos < len(p.wire) {
		c := p.wire[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.pos++
			continue
		}
		if c != ';' {
			break
		}
		p.pos++
		for p.pos < len(p.wire) && p.wire[p.pos] != '\r' && p.wire[p.pos] != '\n' {
			c = p.wire[p.pos]
			if c != '\t' && (c < 32 || c > 126) {
				p.fail("invalid comment octet")
				return
			}
			p.pos++
		}
		if p.pos == len(p.wire) {
			p.fail("comment requires end-of-line")
			return
		}
	}
	if p.pos > start {
		p.leaf("Layout", "raw", start, nil)
	}
}

func (p *megacoReader) sep() {
	start := p.pos
	p.ws()
	if p.pos == start {
		p.fail("required message separator")
	}
}

func (p *megacoReader) peek() byte {
	p.ws()
	if p.err != nil || p.pos >= len(p.wire) {
		return 0
	}
	return p.wire[p.pos]
}

func (p *megacoReader) punct(c byte) {
	if p.peek() != c {
		p.fail("expected %q", c)
		return
	}
	start := p.pos
	p.pos++
	p.leaf("Delimiter", "raw", start, nil)
}

func megacoSafe(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("+-&!_/'?@^`~*$\\()%|.", rune(c))
}

func (p *megacoReader) peekWord() string {
	p.ws()
	end := p.pos
	for end < len(p.wire) && megacoSafe(p.wire[end]) {
		end++
	}
	return string(p.wire[p.pos:end])
}

// Look through the same trivia as ws without changing the live cursor, field
// list, or error. This is only used to disambiguate a bare auditItem.
func (p *megacoReader) afterWord() byte {
	word := p.peekWord()
	probe := &megacoReader{wire: p.wire, pos: p.pos + len(word), current: &megacoField{}}
	return probe.peek()
}

func megacoDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func megacoIPv4(s string) (string, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return "", false
	}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 8)
		if !megacoDigits(part) || len(part) > 3 || err != nil {
			return "", false
		}
		parts[i] = strconv.FormatUint(n, 10)
	}
	return strings.Join(parts, "."), true
}

func (p *megacoReader) word(name string) string {
	w := p.peekWord()
	start := p.pos
	if w == "" {
		p.fail("expected %s", name)
		return ""
	}
	p.pos += len(w)
	p.leaf(name, "string", start, nil)
	return w
}

func (p *megacoReader) keyword(name string) string {
	w := p.word(name)
	key := megacoKeywords[strings.ToLower(w)]
	if key == "" {
		p.fail("unsupported %s %q", name, w)
	}
	return key
}

func megacoName(s string) bool {
	if len(s) < 1 || len(s) > 64 || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) {
		return false
	}
	for _, c := range []byte(s[1:]) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func megacoPath(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	parts := strings.Split(s, "@")
	if len(parts) > 2 {
		return false
	}
	path := strings.TrimPrefix(parts[0], "*")
	if len(path) == 0 || !megacoName(path[:1]) {
		return false
	}
	for _, c := range []byte(path) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("_/*$", rune(c))) {
			return false
		}
	}
	if len(parts) == 2 {
		d := parts[1]
		if len(d) == 0 {
			return false
		}
		for i, c := range []byte(d) {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '*' || i > 0 && (c == '-' || c == '.')) {
				return false
			}
		}
	}
	return true
}

func (p *megacoReader) termination() {
	w := p.word("Termination ID")
	if w != "*" && w != "$" && !megacoPath(w) {
		p.fail("invalid Termination ID")
	}
}

func (p *megacoReader) number(name string, bits int) uint64 {
	p.ws()
	start := p.pos
	for p.pos < len(p.wire) && p.wire[p.pos] >= '0' && p.wire[p.pos] <= '9' {
		p.pos++
	}
	w := string(p.wire[start:p.pos])
	limit := 10
	if bits == 16 {
		limit = 5
	}
	if bits == 8 {
		limit = 2
	}
	v, err := strconv.ParseUint(w, 10, bits)
	if len(w) == 0 || len(w) > limit || err != nil {
		p.fail("invalid %s", name)
		return 0
	}
	p.leaf(name, "string", start, map[string]any{"Numeric Value": v})
	return v
}

func (p *megacoReader) value(name string) string {
	if p.peek() != '"' {
		return p.word(name)
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.wire) && p.wire[p.pos] != '"' {
		c := p.wire[p.pos]
		if c != '\t' && (c < 32 || c > 126) {
			p.fail("invalid quoted-string octet")
			return ""
		}
		p.pos++
	}
	if p.pos == len(p.wire) {
		p.fail("unterminated quoted string")
		return ""
	}
	p.pos++
	p.leaf(name, "string", start, nil)
	return string(p.wire[start:p.pos])
}

func (p *megacoReader) enum(name string, choices ...string) {
	w := p.word(name)
	for _, choice := range choices {
		for _, v := range strings.Split(choice, "/") {
			if strings.EqualFold(w, v) {
				return
			}
		}
	}
	p.fail("unsupported %s %q", name, w)
}

func (p *megacoReader) mid() {
	p.group("Message Identifier", func() {
		c := p.peek()
		if c == '[' || c == '<' {
			end := byte(']')
			if c == '<' {
				end = '>'
			}
			p.punct(c)
			start := p.pos
			for p.pos < len(p.wire) && p.wire[p.pos] != end {
				p.pos++
			}
			if p.pos == len(p.wire) {
				p.fail("unterminated message identifier")
				return
			}
			value := string(p.wire[start:p.pos])
			if c == '[' {
				if strings.Contains(value, ":") {
					check := value
					if strings.Contains(value, ".") {
						i := strings.LastIndexByte(value, ':')
						v4, ok := megacoIPv4(value[i+1:])
						if !ok {
							p.fail("invalid embedded IPv4 message identifier")
						}
						check = value[:i+1] + v4
					}
					addr, err := netip.ParseAddr(check)
					if err != nil || !addr.Is6() || addr.Zone() != "" {
						p.fail("invalid IPv6 message identifier")
					}
				} else {
					if _, ok := megacoIPv4(value); !ok {
						p.fail("invalid IPv4 message identifier")
					}
				}
			} else {
				if len(value) < 1 || len(value) > 64 {
					p.fail("invalid domain message identifier")
				}
				for i, c := range []byte(value) {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || i > 0 && (c == '-' || c == '.')) {
						p.fail("invalid domain message identifier")
					}
				}
			}
			p.leaf("Address", "string", start, nil)
			p.punct(end)
			// A port colon has no surrounding LWSP in Annex B.
			if p.pos < len(p.wire) && p.wire[p.pos] == ':' {
				p.punct(':')
				before := p.pos
				p.number("Port", 16)
				if before < len(p.wire) && (p.wire[before] < '0' || p.wire[before] > '9') {
					p.fail("whitespace before port is unsupported")
				}
			}
		} else {
			w := p.word("Device Name")
			if !megacoPath(w) {
				p.fail("unsupported message identifier")
			}
		}
	})
}

func (p *megacoReader) list(empty bool, fn func()) {
	p.punct('{')
	if p.peek() == '}' {
		if !empty {
			p.fail("empty %s structure is not permitted", p.current.Name)
			return
		}
		p.punct('}')
		return
	}
	for p.err == nil {
		before := p.pos
		fn()
		if p.err != nil {
			return
		}
		if p.pos <= before {
			p.fail("nonprogressing structure")
			return
		}
		if p.peek() != ',' {
			break
		}
		p.punct(',')
	}
	p.punct('}')
}

func (p *megacoReader) pkg(name string) {
	w := p.word(name)
	parts := strings.Split(w, "/")
	if len(parts) != 2 || (parts[0] != "*" && !megacoName(parts[0])) || (parts[1] != "*" && !megacoName(parts[1])) || parts[0] == "*" && parts[1] != "*" {
		p.fail("invalid package/item name")
	}
}

func (p *megacoReader) parmValue() {
	c := p.peek()
	if c == '<' || c == '>' || c == '#' {
		p.punct(c)
		p.value("Value")
		return
	}
	p.punct('=')
	c = p.peek()
	if c != '[' && c != '{' {
		p.value("Value")
		return
	}
	end := byte(']')
	if c == '{' {
		end = '}'
	}
	p.punct(c)
	p.value("Value")
	before := p.pos
	if c == '[' && p.peek() == ':' {
		if p.pos != before {
			p.fail("whitespace inside property range")
		}
		p.punct(':')
		before = p.pos
		p.ws()
		if p.pos != before {
			p.fail("whitespace inside property range")
		}
		p.value("Range End")
	} else {
		for p.peek() == ',' && p.err == nil {
			p.punct(',')
			p.value("Value")
		}
	}
	p.punct(end)
}

func (p *megacoReader) errorDescriptor() {
	p.group("Error Descriptor", func() {
		if p.keyword("Descriptor Name") != "Error" {
			p.fail("expected Error descriptor")
		}
		p.punct('=')
		p.ws()
		start := p.pos
		p.number("Error Code", 16)
		if p.pos-start > 4 {
			p.fail("Error Code exceeds four digits")
		}
		p.punct('{')
		if p.peek() != '}' {
			if p.peek() != '"' {
				p.fail("error text must be quoted")
			}
			p.value("Error Text")
		}
		p.punct('}')
	})
}

func megacoAllowed(key, allowed string) bool { return strings.Contains("|"+allowed+"|", "|"+key+"|") }

func (p *megacoReader) descriptors(allowed string, reply, auditCap bool) {
	seen := map[string]bool{}
	p.list(false, func() {
		key := megacoKeywords[strings.ToLower(p.peekWord())]
		if key == "" || !megacoAllowed(key, allowed) {
			p.fail("unsupported descriptor in this command")
			return
		}
		if seen[key] {
			p.fail("duplicate descriptor %s", key)
			return
		}
		seen[key] = true
		p.descriptor(reply, auditCap)
	})
}

func (p *megacoReader) descriptor(reply, auditCap bool) {
	p.group("Descriptor", func() {
		key := p.keyword("Descriptor Name")
		switch key {
		case "Media":
			seen := map[string]bool{}
			mode := ""
			p.list(false, func() {
				k := megacoKeywords[strings.ToLower(p.peekWord())]
				if k != "Stream" && k != "TerminationState" && !megacoAllowed(k, "Local|Remote|LocalControl") {
					p.fail("unsupported Media parameter")
					return
				}
				if k != "Stream" && seen[k] {
					p.fail("duplicate Media parameter")
					return
				}
				seen[k] = true
				if k != "TerminationState" {
					next := "plain"
					if k == "Stream" {
						next = "streams"
					}
					if mode != "" && next != mode {
						p.fail("mixed Media stream layouts")
					}
					mode = next
				}
				p.descriptor(reply, auditCap)
			})
		case "Stream":
			p.punct('=')
			p.number("Stream ID", 16)
			p.descriptors("Local|Remote|LocalControl", reply, auditCap)
		case "Local", "Remote":
			p.punct('{')
			start := p.pos
			for p.pos < len(p.wire) {
				if p.wire[p.pos] == 0 {
					p.fail("zero octet in SDP envelope")
					break
				}
				if p.wire[p.pos] == '\\' && p.pos+1 < len(p.wire) && p.wire[p.pos+1] == '}' {
					p.pos += 2
					continue
				}
				if p.wire[p.pos] == '}' {
					break
				}
				p.pos++
			}
			p.leaf("SDP Text", "raw", start, nil)
			p.punct('}')
		case "LocalControl", "TerminationState":
			seen := map[string]bool{}
			p.list(false, func() {
				raw := p.peekWord()
				k := megacoKeywords[strings.ToLower(raw)]
				if strings.Contains(raw, "/") {
					p.pkg("Property Name")
					p.parmValue()
					return
				}
				if seen[k] {
					p.fail("duplicate control parameter")
				}
				seen[k] = true
				p.keyword("Parameter Name")
				p.punct('=')
				if key == "LocalControl" {
					switch k {
					case "Mode":
						p.enum("Mode", "SendOnly/SO", "ReceiveOnly/RC", "SendReceive/SR", "Inactive/IN", "Loopback/LB")
					case "ReservedValue", "ReservedGroup":
						p.enum("Flag", "ON", "OFF")
					default:
						p.fail("unsupported LocalControl parameter")
					}
				} else {
					switch k {
					case "ServiceStates":
						p.enum("Service State", "Test/TE", "OutOfService/OS", "InService/IV")
					case "Buffer":
						p.enum("Buffer Mode", "OFF", "LockStep/SP")
					default:
						p.fail("unsupported TerminationState parameter")
					}
				}
			})
		case "Audit":
			seen := map[string]bool{}
			p.list(true, func() {
				k := p.keyword("Audit Item")
				if !megacoAllowed(k, "Mux|Modem|Media|Signals|EventBuffer|DigitMap|Statistics|Events|ObservedEvents|Packages") || auditCap && (k == "DigitMap" || k == "Packages") {
					p.fail("unsupported audit item")
				}
				if seen[k] {
					p.fail("duplicate audit item")
				}
				seen[k] = true
			})
		case "Signals":
			p.list(true, func() { p.signal(true) })
		case "Events":
			if p.peek() == '=' {
				p.punct('=')
				p.requestID()
				p.list(false, func() { p.event(0) })
			}
		case "ObservedEvents":
			p.punct('=')
			p.requestID()
			p.list(false, func() { p.event(1) })
		case "EventBuffer":
			if p.peek() == '{' {
				p.list(false, func() { p.event(2) })
			}
		case "Statistics":
			seen := map[string]bool{}
			p.list(false, func() {
				name := strings.ToLower(p.peekWord())
				if seen[name] {
					p.fail("duplicate statistic parameter")
				}
				seen[name] = true
				p.pkg("Statistic Name")
				if p.peek() == '=' {
					p.punct('=')
					p.value("Statistic Value")
				}
			})
		case "Packages":
			p.list(false, func() {
				w := p.word("Package Version")
				parts := strings.Split(w, "-")
				if len(parts) != 2 || !megacoName(parts[0]) {
					p.fail("invalid package version")
					return
				}
				_, err := strconv.ParseUint(parts[1], 10, 16)
				if !megacoDigits(parts[1]) || len(parts[1]) > 5 || err != nil {
					p.fail("invalid package version")
				}
			})
		case "Services":
			p.services(reply)
		case "DigitMap":
			p.punct('=')
			if !megacoName(p.word("Digit Map Name")) {
				p.fail("unsupported inline DigitMap body")
			}
			if p.peek() == '{' {
				p.fail("unsupported inline DigitMap body")
			}
		case "Mux":
			p.punct('=')
			p.enum("Multiplex Type", "H221", "H223", "H226", "V76")
			p.list(false, p.termination)
		case "Modem":
			seen := map[string]bool{}
			modem := func() {
				name := strings.ToLower(p.peekWord())
				if name == "sn" {
					name = "synchisdn"
				}
				if seen[name] {
					p.fail("duplicate modem type")
				}
				seen[name] = true
				p.enum("Modem Type", "V32b", "V22b", "V18", "V22", "V32", "V34", "V90", "V91", "SynchISDN/SN")
			}
			if p.peek() == '=' {
				p.punct('=')
				modem()
			} else {
				p.punct('[')
				modem()
				for p.peek() == ',' && p.err == nil {
					p.punct(',')
					modem()
				}
				p.punct(']')
			}
			if p.peek() == '{' {
				p.list(false, func() { p.pkg("Property Name"); p.parmValue() })
			}
		default:
			p.fail("unsupported descriptor %s", key)
		}
	})
}

func (p *megacoReader) requestID() {
	if p.peekWord() == "*" {
		p.word("Request ID")
	} else {
		p.number("Request ID", 32)
	}
}

func megacoTimestamp(w string) bool {
	if len(w) != 17 || (w[8] != 'T' && w[8] != 't') {
		return false
	}
	for i, c := range []byte(w) {
		if i != 8 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func (p *megacoReader) event(mode int) {
	p.group("Event", func() {
		if mode == 1 && megacoTimestamp(p.peekWord()) {
			p.word("Timestamp")
			p.punct(':')
		}
		p.pkg("Event Name")
		if p.peek() != '{' {
			return
		}
		seen := map[string]bool{}
		p.list(false, func() {
			w := p.word("Event Parameter")
			k := megacoKeywords[strings.ToLower(w)]
			id := strings.ToLower(w)
			if k == "Stream" || mode == 0 && megacoAllowed(k, "KeepActive|DigitMap|Embed") {
				id = k
			}
			if seen[id] {
				p.fail("duplicate event parameter")
			}
			seen[id] = true
			switch k {
			case "Stream":
				p.punct('=')
				p.number("Stream ID", 16)
			case "KeepActive":
				if mode != 0 {
					p.parmValue()
				}
			case "DigitMap":
				if mode != 0 {
					p.parmValue()
					return
				}
				p.punct('=')
				if !megacoName(p.word("Digit Map Name")) {
					p.fail("unsupported event DigitMap")
				}
			case "Embed":
				if mode == 0 {
					p.fail("unsupported embedded event descriptor")
				} else {
					p.parmValue()
				}
			default:
				if !megacoName(w) {
					p.fail("invalid event parameter name")
				}
				p.parmValue()
			}
		})
	})
}

func (p *megacoReader) signal(allowList bool) {
	p.group("Signal", func() {
		if megacoKeywords[strings.ToLower(p.peekWord())] == "SignalList" {
			if !allowList {
				p.fail("nested SignalList is not permitted")
				return
			}
			p.keyword("Signal List")
			p.punct('=')
			p.number("Signal List ID", 16)
			p.list(false, func() { p.signal(false) })
			return
		}
		p.pkg("Signal Name")
		if p.peek() != '{' {
			if !allowList {
				p.fail("SignalList item requires SignalType")
			}
			return
		}
		seen := map[string]bool{}
		p.list(false, func() {
			w := p.word("Signal Parameter")
			k := megacoKeywords[strings.ToLower(w)]
			id := strings.ToLower(w)
			if megacoAllowed(k, "KeepActive|Stream|Duration|SignalType|NotifyCompletion") {
				id = k
			}
			if seen[id] {
				p.fail("duplicate signal parameter")
			}
			seen[id] = true
			switch k {
			case "KeepActive":
			case "Stream":
				p.punct('=')
				p.number("Stream ID", 16)
			case "Duration":
				p.punct('=')
				p.number("Duration", 16)
			case "SignalType":
				p.punct('=')
				p.enum("Signal Type", "OnOff/OO", "TimeOut/TO", "Brief/BR")
			case "NotifyCompletion":
				p.punct('=')
				p.list(false, func() {
					p.enum("Completion Reason", "TimeOut/TO", "IntByEvent/IBE", "IntBySigDescr/IBS", "OtherReason/OR")
				})
			default:
				if !megacoName(w) {
					p.fail("invalid signal parameter name")
				}
				p.parmValue()
			}
		})
		if !allowList && !seen["SignalType"] {
			p.fail("SignalList item requires SignalType")
		}
	})
}

func (p *megacoReader) services(reply bool) {
	seen := map[string]bool{}
	p.list(false, func() {
		if megacoTimestamp(p.peekWord()) {
			if seen["Timestamp"] {
				p.fail("duplicate timestamp")
			}
			seen["Timestamp"] = true
			p.word("Timestamp")
			return
		}
		k := p.keyword("Service Change Parameter")
		if seen[k] {
			p.fail("duplicate service-change parameter")
		}
		seen[k] = true
		if reply && !megacoAllowed(k, "ServiceChangeAddress|MgcIdToTry|Profile|Version") {
			p.fail("unsupported service-change reply parameter")
		}
		p.punct('=')
		switch k {
		case "Method":
			p.enum("Service Change Method", "Failover/FL", "Forced/FO", "Graceful/GR", "Restart/RS", "Disconnected/DC", "HandOff/HO")
		case "Reason":
			if p.peek() != '"' {
				p.fail("service-change reason must be quoted")
				return
			}
			s := p.value("Service Change Reason")
			if len(s) < 3 {
				p.fail("invalid service-change reason code")
				return
			}
			s = s[1 : len(s)-1]
			i := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i == 0 || i < len(s) && s[i] != ' ' {
				p.fail("invalid service-change reason code")
			}
		case "Delay":
			p.number("Service Change Delay", 32)
		case "Version":
			p.number("Service Change Version", 8)
		case "Profile":
			w := p.word("Service Change Profile")
			parts := strings.Split(w, "/")
			if len(parts) != 2 || !megacoName(parts[0]) {
				p.fail("invalid service-change profile")
				return
			}
			_, err := strconv.ParseUint(parts[1], 10, 8)
			if !megacoDigits(parts[1]) || len(parts[1]) > 2 || err != nil {
				p.fail("invalid profile version")
			}
		case "ServiceChangeAddress":
			if p.peek() >= '0' && p.peek() <= '9' {
				p.number("Service Change Port", 16)
			} else {
				p.mid()
			}
		case "MgcIdToTry":
			p.mid()
		default:
			p.fail("unsupported service-change parameter")
		}
	})
	if !reply && (!seen["Method"] || !seen["Reason"]) {
		p.fail("service-change Method and Reason are required")
	}
	if seen["ServiceChangeAddress"] && seen["MgcIdToTry"] {
		p.fail("conflicting service-change addresses")
	}
}

func (p *megacoReader) command(reply bool) {
	p.group("Command", func() {
		w := p.word("Command Name")
		if !reply {
			if strings.HasPrefix(strings.ToUpper(w), "O-") {
				w = w[2:]
			}
			if strings.HasPrefix(strings.ToUpper(w), "W-") {
				w = w[2:]
			}
		}
		k := megacoKeywords[strings.ToLower(w)]
		if !megacoAllowed(k, "Add|Move|Modify|Subtract|AuditValue|AuditCapability|Notify|ServiceChange") {
			p.fail("unsupported command %q", w)
			return
		}
		if p.version == 2 && (!reply || !megacoAllowed(k, "Add|Move|Modify|Subtract")) {
			p.fail("unsupported v2 command outside AMMS reply profile")
			return
		}
		p.current.Info = map[string]any{"Command Kind": k, "Reply Form": reply}
		p.punct('=')
		p.termination()
		if p.version == 2 && p.peek() == '{' {
			p.fail("unsupported v2 reply descriptors")
			return
		}
		if p.peek() != '{' {
			if !reply && megacoAllowed(k, "AuditValue|AuditCapability|Notify|ServiceChange") {
				p.fail("required command descriptor")
			}
			return
		}
		if k == "Notify" {
			p.punct('{')
			if !reply {
				if megacoKeywords[strings.ToLower(p.peekWord())] != "ObservedEvents" {
					p.fail("Notify requires ObservedEvents")
					return
				}
				p.descriptor(reply, false)
				if p.peek() == ',' {
					p.punct(',')
					p.errorDescriptor()
				}
			} else {
				p.errorDescriptor()
			}
			p.punct('}')
			return
		}
		if k == "ServiceChange" {
			p.punct('{')
			if reply && megacoKeywords[strings.ToLower(p.peekWord())] == "Error" {
				p.errorDescriptor()
			} else {
				if megacoKeywords[strings.ToLower(p.peekWord())] != "Services" {
					p.fail("ServiceChange requires Services")
					return
				}
				p.descriptor(reply, false)
			}
			p.punct('}')
			return
		}
		if !reply && megacoAllowed(k, "Subtract|AuditValue|AuditCapability") {
			p.punct('{')
			if megacoKeywords[strings.ToLower(p.peekWord())] != "Audit" {
				p.fail("command requires Audit descriptor")
				return
			}
			p.descriptor(false, k == "AuditCapability")
			p.punct('}')
			return
		}
		if !reply {
			p.descriptors("Media|Modem|Mux|Events|Signals|DigitMap|EventBuffer|Audit", false, false)
			return
		}
		seen := map[string]bool{}
		p.list(false, func() {
			key := megacoKeywords[strings.ToLower(p.peekWord())]
			if seen[key] {
				p.fail("duplicate audit return parameter")
			}
			seen[key] = true
			if key == "Error" {
				p.errorDescriptor()
				return
			}
			if !megacoAllowed(key, "Media|Modem|Mux|Events|Signals|DigitMap|EventBuffer|ObservedEvents|Statistics|Packages") {
				p.fail("unsupported audit return parameter")
				return
			}
			// A bare auditItem is permitted in a reply; distinguish it from
			// descriptors by the following delimiter without swallowing data.
			if after := p.afterWord(); after == ',' || after == '}' {
				if k == "AuditCapability" && (key == "DigitMap" || key == "Packages") {
					p.fail("unsupported audit item")
					return
				}
				p.keyword("Audit Item")
			} else {
				p.descriptor(true, k == "AuditCapability")
			}
		})
	})
}

func (p *megacoReader) context(reply bool) {
	p.group("Context", func() {
		if p.keyword("Context Token") != "Context" {
			p.fail("expected Context")
			return
		}
		p.punct('=')
		w := p.peekWord()
		if w == "-" || w == "*" || w == "$" {
			p.word("Context ID")
		} else {
			n := p.number("Context ID", 32)
			if n == 0 || n >= 0xfffffffe {
				p.fail("reserved numeric Context ID")
			}
		}
		seen := map[string]bool{}
		commands := false
		ended := false
		p.list(false, func() {
			if ended {
				p.fail("Error descriptor must end Context reply")
				return
			}
			key := megacoKeywords[strings.ToLower(p.peekWord())]
			if p.version == 2 && megacoAllowed(key, "Error|Priority|Emergency|Topology|ContextAudit") {
				p.fail("unsupported v2 Context outside command-only reply profile")
				return
			}
			if reply && key == "Error" {
				p.errorDescriptor()
				ended = true
				return
			}
			if megacoAllowed(key, "Priority|Emergency|Topology|ContextAudit") {
				if commands || seen[key] || seen["ContextAudit"] || reply && key == "ContextAudit" {
					p.fail("invalid Context property placement or duplicate")
					return
				}
				seen[key] = true
				p.keyword("Context Property")
				switch key {
				case "Priority":
					p.punct('=')
					p.number("Priority", 16)
				case "Emergency":
				case "Topology":
					p.list(false, func() {
						p.termination()
						p.punct(',')
						p.termination()
						p.punct(',')
						p.enum("Topology Direction", "Bothway/BW", "Isolate/IS", "Oneway/OW")
					})
				case "ContextAudit":
					audited := map[string]bool{}
					p.list(false, func() {
						k := megacoKeywords[strings.ToLower(p.peekWord())]
						if audited[k] {
							p.fail("duplicate Context audit property")
						}
						audited[k] = true
						p.enum("Context Audit Property", "Topology/TP", "Emergency/EG", "Priority/PR")
					})
				}
				return
			}
			commands = true
			p.command(reply)
		})
	})
}

func decodeMegacoMessage(wire []byte) ([]megacoField, error) {
	if len(wire) == 0 || len(wire) > megacoMaxBytes {
		return nil, fmt.Errorf("megaco: explicit message boundary must be 1..65527 bytes")
	}
	root := &megacoField{Name: "Megaco"}
	p := &megacoReader{wire: wire, current: root}
	p.ws()
	start := p.pos
	for p.pos < len(wire) && wire[p.pos] != '/' && megacoSafe(wire[p.pos]) {
		p.pos++
	}
	marker := string(wire[start:p.pos])
	if !strings.EqualFold(marker, "MEGACO") && marker != "!" {
		p.fail("unsupported textual protocol marker")
	}
	p.leaf("Protocol Marker", "string", start, nil)
	if p.pos >= len(wire) || wire[p.pos] != '/' {
		p.fail("expected protocol version slash")
	} else {
		start = p.pos
		p.pos++
		p.leaf("Delimiter", "raw", start, nil)
	}
	start = p.pos
	version := p.number("Version", 8)
	p.version = version
	if start < len(wire) && (wire[start] < '0' || wire[start] > '9') {
		p.fail("whitespace before Version")
	}
	if version != 1 && version != 2 {
		p.fail("unsupported version outside v1/v2 profiles")
	}
	p.sep()
	p.mid()
	p.sep()
	if megacoKeywords[strings.ToLower(p.peekWord())] == "Error" {
		if p.version == 2 {
			p.fail("unsupported v2 message outside AMMS reply profile")
		}
		p.errorDescriptor()
	} else {
		count := 0
		for p.err == nil && p.peek() != 0 {
			count++
			p.group("Transaction", func() {
				key := p.keyword("Transaction Type")
				p.current.Info = map[string]any{"Transaction Kind": key}
				if p.version == 2 && key != "Reply" {
					p.fail("unsupported v2 transaction outside AMMS reply profile")
					return
				}
				if key == "TransactionResponseAck" {
					p.list(false, func() {
						p.number("First Transaction ID", 32)
						before := p.pos
						if p.peek() == '-' {
							if p.pos != before {
								p.fail("whitespace inside transaction acknowledgment range")
							}
							p.punct('-')
							before = p.pos
							p.number("Last Transaction ID", 32)
							if before < len(wire) && (wire[before] < '0' || wire[before] > '9') {
								p.fail("whitespace inside transaction acknowledgment range")
							}
						}
					})
					return
				}
				if !megacoAllowed(key, "Transaction|Reply|Pending") {
					p.fail("unsupported transaction type")
					return
				}
				p.punct('=')
				p.number("Transaction ID", 32)
				p.punct('{')
				if key == "Pending" {
					p.punct('}')
					return
				}
				if key == "Reply" && megacoKeywords[strings.ToLower(p.peekWord())] == "ImmAckRequired" {
					p.keyword("Immediate Acknowledgment")
					p.punct(',')
				}
				if key == "Reply" && megacoKeywords[strings.ToLower(p.peekWord())] == "Error" {
					if p.version == 2 {
						p.fail("unsupported v2 transaction Error reply")
					}
					p.errorDescriptor()
				} else {
					p.context(key == "Reply")
					for p.err == nil && p.peek() == ',' {
						p.punct(',')
						p.context(key == "Reply")
					}
				}
				p.punct('}')
			})
		}
		if count == 0 {
			p.fail("message body requires a transaction")
		}
	}
	p.ws()
	if p.err == nil && p.pos != len(wire) {
		p.fail("unexpected trailing message bytes")
	}
	if p.err != nil {
		return nil, p.err
	}
	return root.Children, nil
}
