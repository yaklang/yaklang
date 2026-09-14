package stream_parser

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const imapFieldsMaxBytes = 1 << 20
const imapFieldsMaxItems = 4096
const imapFieldsMaxLeaves = 32768

// Explicit IMAP4rev1 command/response layouts. No cached tag, mailbox, literal
// continuation or negotiation state is consulted. Complete literal bytes must
// already be present in the caller's bounded message, even when they span TCP.
type imapFieldsReader struct {
	wire     []byte
	at       int
	err      error
	fields   []tlsCertificateField
	literals int
	items    int
	leaves   int
}

type imapFieldsString struct {
	value  []byte
	ranges [][2]int
	kind   string
}

func (r *imapFieldsReader) fail(message string) {
	if r.err == nil {
		r.err = fmt.Errorf("imap-fields: %s at byte %d", message, r.at)
	}
}
func (r *imapFieldsReader) peek() byte {
	if r.at >= len(r.wire) {
		return 0
	}
	return r.wire[r.at]
}
func (r *imapFieldsReader) leaf(name, typ string, start, end int) {
	if r.err != nil {
		return
	}
	if r.leaves >= imapFieldsMaxLeaves {
		r.fail("field resource limit")
		return
	}
	r.leaves++
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, start, end))
}
func (r *imapFieldsReader) exact(name, s string) {
	if r.err != nil {
		return
	}
	if !bytes.HasPrefix(r.wire[r.at:], []byte(s)) {
		r.fail("missing " + name)
		return
	}
	r.leaf(name, "raw", r.at, r.at+len(s))
	r.at += len(s)
}
func (r *imapFieldsReader) sp() { r.exact("Separator", " ") }

func imapFieldsAtom(c byte, mode string) bool {
	if c < 33 || c > 126 || strings.ContainsRune("(){\"\\", rune(c)) {
		return false
	}
	if c == ']' {
		return mode == "astring" || mode == "pattern" || mode == "tag"
	}
	if c == '%' || c == '*' {
		return mode == "pattern"
	}
	return mode != "tag" || c != '+'
}
func (r *imapFieldsReader) atom(name, mode string) string {
	if r.err != nil {
		return ""
	}
	start := r.at
	for r.at < len(r.wire) && imapFieldsAtom(r.wire[r.at], mode) {
		r.at++
	}
	if start == r.at {
		r.fail("empty/invalid " + name)
		return ""
	}
	r.leaf(name, "string", start, r.at)
	return string(r.wire[start:r.at])
}
func (r *imapFieldsReader) number(name string, positive bool) uint64 {
	if r.err != nil {
		return 0
	}
	start := r.at
	for r.peek() >= '0' && r.peek() <= '9' {
		r.at++
	}
	v, err := strconv.ParseUint(string(r.wire[start:r.at]), 10, 32)
	if err != nil || positive && (v == 0 || r.wire[start] == '0') {
		r.fail("invalid 32-bit " + name)
		return 0
	}
	r.leaf(name, "string", start, r.at)
	return v
}
func (r *imapFieldsReader) stringValue(name, mode string) imapFieldsString {
	value := imapFieldsString{ranges: make([][2]int, 0)}
	if r.err != nil {
		return value
	}
	start, fieldStart := r.at, len(r.fields)
	switch r.peek() {
	case '"':
		value.kind = "quoted"
		r.exact("Quote Open", "\"")
		fragment := r.at
		for r.err == nil && r.peek() != '"' {
			if r.at == len(r.wire) || r.peek() == 0 || r.peek() == '\r' || r.peek() == '\n' || r.peek() > 127 {
				r.fail("invalid/incomplete quoted string")
				break
			}
			if r.peek() == '\\' {
				if fragment < r.at {
					r.leaf("String Data", "raw", fragment, r.at)
					value.value = append(value.value, r.wire[fragment:r.at]...)
					value.ranges = append(value.ranges, [2]int{fragment, r.at})
				}
				r.exact("Quote Escape", "\\")
				if r.peek() != '"' && r.peek() != '\\' {
					r.fail("invalid quoted escape")
					break
				}
				r.leaf("Escaped Character", "raw", r.at, r.at+1)
				value.value = append(value.value, r.peek())
				value.ranges = append(value.ranges, [2]int{r.at, r.at + 1})
				r.at++
				fragment = r.at
			} else {
				r.at++
			}
		}
		if r.err == nil {
			r.leaf("String Data", "raw", fragment, r.at)
			value.value = append(value.value, r.wire[fragment:r.at]...)
			if fragment < r.at {
				value.ranges = append(value.ranges, [2]int{fragment, r.at})
			}
			r.exact("Quote Close", "\"")
		}
	case '{':
		value.kind = "literal"
		r.exact("Literal Open", "{")
		size := r.number("Literal Octets", false)
		r.exact("Literal Close", "}")
		r.exact("Literal CRLF", "\r\n")
		if r.err != nil {
			break
		}
		if size > uint64(len(r.wire)-r.at) {
			r.fail("incomplete literal")
			break
		}
		end := r.at + int(size)
		if bytes.IndexByte(r.wire[r.at:end], 0) >= 0 {
			r.fail("NUL outside IMAP4rev1 CHAR8")
			break
		}
		r.leaf("Literal Data", "raw", r.at, end)
		value.value = bytes.Clone(r.wire[r.at:end])
		if end > r.at {
			value.ranges = append(value.ranges, [2]int{r.at, end})
		}
		r.at = end
		r.literals++
	default:
		if mode == "nstring" && len(r.wire)-r.at >= 3 && strings.EqualFold(string(r.wire[r.at:r.at+3]), "NIL") {
			value.kind = "nil"
			r.leaf("NIL", "string", r.at, r.at+3)
			r.at += 3
		} else if mode == "astring" || mode == "pattern" {
			value.kind = "atom"
			r.atom("String Data", mode)
			value.value = bytes.Clone(r.wire[start:r.at])
			if start < r.at {
				value.ranges = append(value.ranges, [2]int{start, r.at})
			}
		} else {
			r.fail("expected quoted string, literal or permitted NIL")
		}
	}
	if r.err == nil {
		children := append([]tlsCertificateField(nil), r.fields[fieldStart:]...)
		r.fields = r.fields[:fieldStart]
		r.fields = append(r.fields, tlsCertificateField{Name: name, Start: start, End: r.at, Children: children})
	}
	return value
}

// List delimiters are separate wire leaves; even empty item lists have explicit
// zero-width bounds. All loops share one local item budget.
func (r *imapFieldsReader) list(name string, minItems int, parse func() map[string]any) []map[string]any {
	r.exact("List Open", "(")
	list := tlsCertificateField{Name: name, Start: r.at, List: true}
	values := make([]map[string]any, 0)
	for r.err == nil && r.peek() != ')' {
		if r.items >= imapFieldsMaxItems {
			r.fail("list resource limit")
			break
		}
		r.items++
		start, fs := r.at, len(r.fields)
		if len(values) > 0 {
			r.sp()
		}
		v := parse()
		if r.err != nil {
			break
		}
		if r.at <= start {
			r.fail("list made no progress")
			break
		}
		v["Relative Byte Range"] = [2]int{start, r.at}
		values = append(values, v)
		list.Children = append(list.Children, tlsCertificateField{Name: "Item", Start: start, End: r.at, Children: append([]tlsCertificateField(nil), r.fields[fs:]...)})
		r.fields = r.fields[:fs]
	}
	list.End = r.at
	r.fields = append(r.fields, list)
	if len(values) < minItems {
		r.fail("empty required list")
	}
	r.exact("List Close", ")")
	return values
}
func (r *imapFieldsReader) flags(kind string) []map[string]any {
	return r.list("Flags", 0, func() map[string]any {
		start := r.at
		if r.peek() == '\\' {
			r.exact("Flag Prefix", "\\")
			if kind == "permanent" && r.peek() == '*' {
				r.exact("Flag Wildcard", "*")
			} else {
				r.atom("Flag Name", "atom")
			}
		} else {
			if kind == "mailbox" {
				r.fail("mailbox flag needs backslash")
			}
			r.atom("Flag Name", "atom")
		}
		return map[string]any{"Name": string(r.wire[start:r.at])}
	})
}
func (r *imapFieldsReader) sequence() []map[string]any {
	values := make([]map[string]any, 0)
	for r.err == nil {
		if r.items >= imapFieldsMaxItems {
			r.fail("sequence resource limit")
			break
		}
		r.items++
		start := r.at
		one := func() any {
			if r.peek() == '*' {
				r.exact("Sequence Wildcard", "*")
				return "*"
			}
			return r.number("Sequence Number", true)
		}
		v := map[string]any{"First": one()}
		if r.peek() == ':' {
			r.exact("Range Separator", ":")
			v["Last"] = one()
		}
		v["Relative Byte Range"] = [2]int{start, r.at}
		values = append(values, v)
		if r.peek() != ',' {
			break
		}
		r.exact("Sequence Separator", ",")
	}
	return values // Wildcards/ranges are never expanded into mailbox state.
}
func (r *imapFieldsReader) section() string {
	r.exact("Section Open", "[")
	start := r.at
	part := false
	partText := false
	if r.peek() >= '0' && r.peek() <= '9' {
		part = true
		r.number("Part Number", true)
		for r.err == nil && r.peek() == '.' {
			r.exact("Part Separator", ".")
			if r.peek() < '0' || r.peek() > '9' {
				partText = true
				break
			}
			r.number("Part Number", true)
		}
		if partText && r.peek() == ']' || !partText && r.peek() != ']' {
			r.fail("missing part number or part-text separator")
		}
	}
	if r.peek() != ']' && r.err == nil {
		from := r.at
		for r.peek() >= 'A' && r.peek() <= 'Z' || r.peek() >= 'a' && r.peek() <= 'z' || r.peek() == '.' {
			r.at++
		}
		word := strings.ToUpper(string(r.wire[from:r.at]))
		r.leaf("Section Kind", "string", from, r.at)
		switch word {
		case "HEADER", "TEXT":
		case "MIME":
			if !part {
				r.fail("MIME needs a part selector")
			}
		case "HEADER.FIELDS", "HEADER.FIELDS.NOT":
			r.sp()
			r.list("Header Names", 1, func() map[string]any {
				v := r.stringValue("Header Name", "astring")
				return map[string]any{"Value": v.value}
			})
		default:
			r.fail("unsupported section")
		}
	}
	value := strings.ToUpper(string(r.wire[start:r.at]))
	r.exact("Section Close", "]")
	return value
}
func (r *imapFieldsReader) fetchAttribute(request bool) map[string]any {
	start := r.at
	for r.peek() >= 'A' && r.peek() <= 'Z' || r.peek() >= 'a' && r.peek() <= 'z' || r.peek() >= '0' && r.peek() <= '9' || r.peek() == '.' {
		r.at++
	}
	name := strings.ToUpper(string(r.wire[start:r.at]))
	r.leaf("Attribute Name", "string", start, r.at)
	info := map[string]any{"Name": name}
	section, hasSection := "", false
	if name == "BODY" || name == "BODY.PEEK" {
		if r.peek() == '[' {
			hasSection = true
			section = r.section()
			info["Section"] = section
		}
		if name == "BODY.PEEK" && (!request || !hasSection) {
			r.fail("BODY.PEEK requires a command section")
		}
	}
	if hasSection && r.peek() == '<' {
		r.exact("Partial Open", "<")
		info["Partial Origin"] = r.number("Partial Origin", false)
		if request {
			r.exact("Partial Separator", ".")
			info["Partial Count"] = r.number("Partial Count", true)
		}
		r.exact("Partial Close", ">")
	}
	if request {
		switch name {
		case "FLAGS", "UID", "RFC822.SIZE", "INTERNALDATE", "ENVELOPE", "RFC822", "RFC822.HEADER", "RFC822.TEXT", "BODY", "BODYSTRUCTURE", "BODY.PEEK":
		default:
			r.fail("unsupported requested attribute")
		}
		return info
	}
	r.sp()
	switch name {
	case "FLAGS":
		info["Flags"] = r.flags("fetch")
	case "UID":
		info["Value"] = r.number("UID", true)
	case "RFC822.SIZE":
		info["Value"] = r.number("Message Octets", false)
	case "INTERNALDATE":
		v := r.stringValue("Internal Date", "string")
		if r.err == nil {
			x := string(v.value)
			parsed, err := time.Parse("_2-Jan-2006 15:04:05 -0700", x)
			if v.kind != "quoted" || len(x) != 26 || err != nil {
				r.fail("invalid INTERNALDATE")
			} else {
				info["Date Text"] = x
				info["Date Unix Seconds"] = parsed.Unix()
			}
		}
	case "BODY", "RFC822", "RFC822.HEADER", "RFC822.TEXT":
		if name == "BODY" && !hasSection {
			r.fail("BODY structure response not implemented in this profile")
			break
		}
		v := r.stringValue("Section Data", "nstring")
		info["String Kind"] = v.kind
		info["Octets"] = len(v.value)
		info["Value Relative Byte Ranges"] = v.ranges
		info["MIME Decoded"] = false
		_, partial := info["Partial Origin"]
		if r.err == nil && v.kind != "nil" && !partial && (section == "HEADER" || name == "RFC822.HEADER") {
			// HEADER is only a header section, never a complete message body.
			mr := &mimeFieldsReader{wire: v.value}
			for _, s := range v.ranges {
				for i := s[0]; i < s[1]; i++ {
					mr.origins = append(mr.origins, i)
				}
			}
			headers, _, bodyAt, err := mr.headers(0, len(v.value))
			if err != nil || bodyAt != len(v.value) {
				info["Header Layout Parsed"] = false
				info["Header Layout Error"] = "invalid or incomplete header-only section"
			} else {
				info["Header Layout Parsed"] = true
				info["Headers"] = headers
				info["Header Count"] = len(headers)
			}
		}
	default:
		r.fail("unsupported response attribute")
	}
	return info
}
func (r *imapFieldsReader) fetchRequest() []map[string]any {
	if r.peek() == '(' {
		return r.list("Requested Attributes", 1, func() map[string]any { return r.fetchAttribute(true) })
	}
	start := r.at
	for _, macro := range []string{"ALL", "FAST", "FULL"} {
		if len(r.wire)-r.at >= len(macro) && strings.EqualFold(string(r.wire[r.at:r.at+len(macro)]), macro) {
			r.at += len(macro)
			r.leaf("Fetch Macro", "string", start, r.at)
			return []map[string]any{{"Macro": macro}}
		}
	}
	return []map[string]any{r.fetchAttribute(true)}
}
func (r *imapFieldsReader) capabilities(terminal byte) []string {
	values := make([]string, 0)
	rev1 := false
	for r.err == nil && r.peek() != terminal {
		if r.items >= imapFieldsMaxItems {
			r.fail("capability resource limit")
			break
		}
		r.items++
		r.sp()
		value := r.atom("Capability", "atom")
		values = append(values, value)
		rev1 = rev1 || strings.EqualFold(value, "IMAP4rev1")
	}
	if !rev1 {
		r.fail("missing IMAP4rev1 capability")
	}
	return values
}
func (r *imapFieldsReader) statusText(info map[string]any) {
	r.sp()
	if r.peek() == '[' && r.err == nil {
		r.exact("Response Code Open", "[")
		code := strings.ToUpper(r.atom("Response Code", "atom"))
		info["Response Code"] = code
		info["Response Code Layout Validated"] = true
		switch code {
		case "UIDNEXT", "UIDVALIDITY", "UNSEEN":
			r.sp()
			info["Response Code Value"] = r.number("Response Code Number", true)
		case "PERMANENTFLAGS":
			r.sp()
			info["Permanent Flags"] = r.flags("permanent")
		case "CAPABILITY":
			info["Capabilities"] = r.capabilities(']')
		case "ALERT", "PARSE", "READ-ONLY", "READ-WRITE", "TRYCREATE":
		default:
			info["Response Code Layout Validated"] = false
			if r.peek() != ']' {
				r.sp()
				start := r.at
				for r.err == nil && r.peek() != ']' {
					if r.at >= len(r.wire) || r.peek() == '\r' || r.peek() == '\n' || r.peek() == 0 || r.peek() > 127 {
						r.fail("incomplete response code")
						break
					}
					r.at++
				}
				r.leaf("Response Code Data", "raw", start, r.at)
			}
		}
		r.exact("Response Code Close", "]")
		if r.peek() != '\r' {
			r.sp()
		}
	}
	start := r.at
	for r.err == nil && r.peek() != '\r' {
		if r.at >= len(r.wire) || r.peek() == 0 || r.peek() == '\n' || r.peek() > 127 {
			r.fail("invalid/incomplete response text")
			break
		}
		r.at++
	}
	r.leaf("Response Text", "string", start, r.at)
	// A retained original PERMANENTFLAGS response ends immediately after ].
	// Keep the receiver-readable layout without claiming RFC text conformance.
	info["Empty Response Text Observed"] = start == r.at
	info["Response Text Layout Validated"] = start != r.at
}
func (r *imapFieldsReader) command() map[string]any {
	info := map[string]any{}
	info["Tag"] = r.atom("Tag", "tag")
	r.sp()
	name := strings.ToUpper(r.atom("Command", "atom"))
	info["Command Name"] = name
	if name == "UID" {
		r.sp()
		name = strings.ToUpper(r.atom("UID Command", "atom"))
		info["UID Command"] = name
		if name != "FETCH" && name != "COPY" {
			r.fail("unsupported UID subcommand")
		}
	}
	switch name {
	case "CAPABILITY", "NOOP", "LOGOUT", "STARTTLS", "CHECK", "CLOSE", "EXPUNGE":
	case "LOGIN":
		r.sp()
		r.stringValue("User Name", "astring")
		r.sp()
		r.stringValue("Password", "astring")
	case "SELECT", "EXAMINE", "CREATE", "DELETE", "SUBSCRIBE", "UNSUBSCRIBE":
		r.sp()
		v := r.stringValue("Mailbox", "astring")
		info["Mailbox"] = v.value
	case "RENAME":
		r.sp()
		r.stringValue("Old Mailbox", "astring")
		r.sp()
		r.stringValue("New Mailbox", "astring")
	case "LIST", "LSUB":
		r.sp()
		a := r.stringValue("Reference Name", "astring")
		r.sp()
		b := r.stringValue("Mailbox Pattern", "pattern")
		info["Reference Name"] = a.value
		info["Mailbox Pattern"] = b.value
	case "FETCH", "COPY":
		r.sp()
		info["Sequence Set"] = r.sequence()
		r.sp()
		if name == "FETCH" {
			info["Requested Attributes"] = r.fetchRequest()
		} else {
			r.stringValue("Mailbox", "astring")
		}
	default:
		r.fail("unsupported command")
	}
	r.exact("CRLF", "\r\n")
	return info
}
func (r *imapFieldsReader) response() map[string]any {
	info := map[string]any{}
	tag := ""
	if r.peek() == '*' || r.peek() == '+' {
		tag = string(r.peek())
		r.exact("Response Marker", tag)
	} else {
		tag = r.atom("Tag", "tag")
	}
	info["Tag"] = tag
	if tag == "+" {
		info["Response Name"] = "continuation"
		r.statusText(info)
		r.exact("CRLF", "\r\n")
		return info
	}
	r.sp()
	numeric := tag == "*" && r.peek() >= '0' && r.peek() <= '9'
	numberStart := r.at
	var number uint64
	if numeric {
		number = r.number("Observed Number", false)
		r.sp()
	}
	name := strings.ToUpper(r.atom("Response Name", "atom"))
	info["Response Name"] = name
	if tag != "*" && name != "OK" && name != "NO" && name != "BAD" {
		r.fail("tagged response requires status")
	}
	if numeric {
		info["Observed Number"] = number
		switch name {
		case "EXISTS", "RECENT":
		case "EXPUNGE", "FETCH":
			if number == 0 || r.wire[numberStart] == '0' {
				r.fail("message sequence number needs nonzero first digit")
			}
			if name == "FETCH" {
				r.sp()
				info["Attributes"] = r.list("Attributes", 1, func() map[string]any { return r.fetchAttribute(false) })
			}
		default:
			r.fail("unsupported numeric response")
		}
	} else {
		switch name {
		case "OK", "NO", "BAD", "BYE", "PREAUTH":
			r.statusText(info)
		case "CAPABILITY":
			info["Capabilities"] = r.capabilities('\r')
		case "FLAGS":
			r.sp()
			info["Flags"] = r.flags("mailbox-state")
		case "LIST", "LSUB":
			r.sp()
			info["Mailbox Flags"] = r.flags("mailbox")
			r.sp()
			delim := r.stringValue("Hierarchy Delimiter", "nstring")
			if delim.kind != "nil" && (delim.kind != "quoted" || len(delim.value) != 1) {
				r.fail("hierarchy delimiter must be one quoted character or NIL")
			}
			info["Hierarchy Delimiter"] = delim.value
			info["Hierarchy Delimiter Kind"] = delim.kind
			r.sp()
			mailbox := r.stringValue("Mailbox", "astring")
			info["Mailbox"] = mailbox.value
		case "SEARCH":
			values := make([]uint64, 0)
			for r.err == nil && r.peek() != '\r' {
				if r.items >= imapFieldsMaxItems {
					r.fail("search resource limit")
					break
				}
				r.items++
				r.sp()
				values = append(values, r.number("Search Number", true))
			}
			info["Search Numbers"] = values
		default:
			r.fail("unsupported response")
		}
	}
	r.exact("CRLF", "\r\n")
	return info
}
func decodeIMAPFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) == 0 || len(wire) > imapFieldsMaxBytes || profile != "command" && profile != "response" && profile != "response-block" {
		return nil, nil, fmt.Errorf("imap-fields: invalid explicit profile/boundary")
	}
	r := &imapFieldsReader{wire: wire}
	info := map[string]any{"Layout Context": profile, "Protocol Revision Context": "IMAP4rev1", "Session State Validated": false, "Mailbox State Validated": false, "TCP Reassembly Performed": false, "Structured Generation Supported": false, "Mailbox Names Decoded": false, "MIME Decoded": false}
	if profile == "response-block" {
		list := tlsCertificateField{Name: "Responses", Start: 0, End: len(wire), List: true}
		messages := make([]map[string]any, 0)
		for r.err == nil && r.at < len(wire) {
			if len(messages) >= imapFieldsMaxItems {
				r.fail("response count resource limit")
				break
			}
			start := r.at
			r.fields = nil
			value := r.response()
			value["Relative Byte Range"] = [2]int{start, r.at}
			messages = append(messages, value)
			list.Children = append(list.Children, tlsCertificateField{Name: "Response", Start: start, End: r.at, Children: r.fields})
		}
		r.fields = []tlsCertificateField{list}
		info["Responses"] = messages
		info["Response Count"] = len(messages)
	} else {
		var value map[string]any
		if profile == "command" {
			value = r.command()
		} else {
			value = r.response()
		}
		for k, v := range value {
			info[k] = v
		}
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	if r.at != len(wire) {
		return nil, nil, fmt.Errorf("imap-fields: trailing bytes after complete message")
	}
	info["Literal Count"] = r.literals
	return r.fields, info, nil
}

func parseIMAPFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > imapFieldsMaxBytes*8 || profile != "command" && profile != "response" && profile != "response-block" {
		return fmt.Errorf("imap-fields: explicit nonempty byte boundary within profile limit required")
	}
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeIMAPFields(w, profile) }, "imap-fields")
}
