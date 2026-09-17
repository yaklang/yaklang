package stream_parser

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const smtpFieldsMaxBytes = 1 << 20
const smtpFieldsMaxLines = 8192
const smtpFieldsMaxHeaders = 1024

func smtpFieldsAlphaNum(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

func smtpFieldsAtom(s string) bool {
	if s == "" {
		return false
	}
	for i := range s {
		if !smtpFieldsAlphaNum(s[i]) && !strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", rune(s[i])) {
			return false
		}
	}
	return true
}

func smtpFieldsQuoted(s string) bool {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '\\' {
			i++
			if i >= len(s)-1 {
				return false
			}
		} else if s[i] == '"' {
			return false
		}
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}

func smtpFieldsDomain(s string, literal bool) bool {
	if len(s) == 0 || len(s) > 255 {
		return false
	}
	if s[0] == '[' {
		if !literal || s[len(s)-1] != ']' {
			return false
		}
		s = s[1 : len(s)-1]
		if len(s) > 5 && strings.EqualFold(s[:5], "IPv6:") {
			return strings.Contains(s[5:], ":") && net.ParseIP(s[5:]) != nil
		}
		// RFC 5321 Snum permits leading zeros; do not apply net.ParseIP's
		// narrower textual IPv4 policy to these original wire octets.
		parts := strings.Split(s, ".")
		if len(parts) != 4 {
			return false
		}
		for _, part := range parts {
			if len(part) < 1 || len(part) > 3 {
				return false
			}
			n := 0
			for i := range part {
				if part[i] < '0' || part[i] > '9' {
					return false
				}
				n = n*10 + int(part[i]-'0')
			}
			if n > 255 {
				return false
			}
		}
		return true
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || !smtpFieldsAlphaNum(label[0]) || !smtpFieldsAlphaNum(label[len(label)-1]) {
			return false
		}
		for i := range label {
			if !smtpFieldsAlphaNum(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

// This is a base ASCII command profile, not negotiated extension semantics.
// General address-literal tags and extension commands use the raw carrier.
func decodeSMTPCommandFields(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	bad := func() ([]tlsCertificateField, map[string]any, error) {
		return nil, nil, fmt.Errorf("smtp-command-fields: invalid or unsupported base command layout")
	}
	if len(wire) < 6 || len(wire) > 512 || !bytes.HasSuffix(wire, []byte("\r\n")) {
		return bad()
	}
	end := len(wire) - 2
	for _, c := range wire[:end] {
		if (c < 32 && c != '\t') || c > 126 {
			return bad()
		}
	}
	logicalEnd := end
	for logicalEnd > 0 && (wire[logicalEnd-1] == ' ' || wire[logicalEnd-1] == '\t') {
		logicalEnd--
	}
	line := string(wire[:logicalEnd])
	wordEnd := strings.IndexByte(line, ' ')
	if wordEnd < 0 {
		wordEnd = len(line)
	}
	command := strings.ToUpper(line[:wordEnd])
	fields := []tlsCertificateField{tlsCertificateLeaf("Command", "string", 0, wordEnd)}
	add := func(name, typ string, a, b int) { fields = append(fields, tlsCertificateLeaf(name, typ, a, b)) }
	at := wordEnd
	if at < len(line) {
		add("Command Separator", "raw", at, at+1)
		at++
	}
	arg := line[at:]
	params := 0
	switch command {
	case "HELO", "EHLO":
		if !smtpFieldsDomain(arg, command == "EHLO") {
			return bad()
		}
		add("Client Domain or Literal", "string", at, len(line))
	case "DATA", "RSET", "QUIT":
		if arg != "" {
			return bad()
		}
	case "VRFY", "EXPN", "HELP", "NOOP":
		if arg == "" {
			if command == "VRFY" || command == "EXPN" {
				return bad()
			}
		} else {
			if !smtpFieldsAtom(arg) && !smtpFieldsQuoted(arg) {
				return bad()
			}
			add("Argument", "string", at, len(line))
		}
	case "MAIL", "RCPT":
		prefix := "FROM:"
		if command == "RCPT" {
			prefix = "TO:"
		}
		if len(arg) <= len(prefix) || !strings.EqualFold(arg[:len(prefix)], prefix) {
			return bad()
		}
		add("Path Keyword", "string", at, at+len(prefix)-1)
		at += len(prefix) - 1
		add("Path Colon", "raw", at, at+1)
		at++
		if line[at] != '<' {
			return bad()
		}
		pathStart := at
		add("Path Open", "raw", at, at+1)
		at++
		if at >= len(line) {
			return bad()
		}
		if line[at] == '@' {
			routeStart := at
			for {
				if at >= len(line) || line[at] != '@' {
					return bad()
				}
				at++
				domainStart := at
				for at < len(line) && line[at] != ',' && line[at] != ':' {
					at++
				}
				if at >= len(line) || !smtpFieldsDomain(line[domainStart:at], false) {
					return bad()
				}
				separator := line[at]
				at++
				if separator == ':' {
					break
				}
			}
			add("Source Route", "string", routeStart, at)
		}
		localStart := at
		if at < len(line) && line[at] == '"' {
			at++
			for at < len(line) && line[at] != '"' {
				if line[at] == '\\' {
					at++
				}
				at++
			}
			if at >= len(line) {
				return bad()
			}
			at++
		} else {
			for at < len(line) && line[at] != '@' && line[at] != '>' {
				at++
			}
		}
		if at >= len(line) {
			return bad()
		}
		local := line[localStart:at]
		if line[at] == '>' {
			if !(command == "MAIL" && local == "" && localStart == pathStart+1) && !(command == "RCPT" && strings.EqualFold(local, "Postmaster")) {
				return bad()
			}
		} else {
			if len(local) > 64 {
				return bad()
			}
			if !smtpFieldsQuoted(local) {
				for _, atom := range strings.Split(local, ".") {
					if !smtpFieldsAtom(atom) {
						return bad()
					}
				}
			}
		}
		add("Local Part", "string", localStart, at)
		if line[at] == '@' {
			add("Mailbox Separator", "raw", at, at+1)
			at++
			domainStart := at
			for at < len(line) && line[at] != '>' {
				at++
			}
			if at >= len(line) || !smtpFieldsDomain(line[domainStart:at], true) {
				return bad()
			}
			add("Mailbox Domain or Literal", "string", domainStart, at)
		}
		add("Path Close", "raw", at, at+1)
		at++
		if at-pathStart > 256 {
			return bad()
		}
		parameters := tlsCertificateField{Name: "Parameters", Start: at, End: len(line), List: true}
		for at < len(line) {
			itemStart, fieldStart := at, len(fields)
			if line[at] != ' ' {
				return bad()
			}
			add("Parameter Separator", "raw", at, at+1)
			at++
			start := at
			for at < len(line) && (smtpFieldsAlphaNum(line[at]) || line[at] == '-') {
				at++
			}
			if at == start || !smtpFieldsAlphaNum(line[start]) {
				return bad()
			}
			add("Parameter Keyword", "string", start, at)
			if at < len(line) && line[at] == '=' {
				add("Parameter Equals", "raw", at, at+1)
				at++
				start = at
				for at < len(line) && line[at] >= 33 && line[at] <= 126 && line[at] != '=' {
					at++
				}
				if at == start {
					return bad()
				}
				add("Parameter Value", "string", start, at)
			}
			parameters.Children = append(parameters.Children, tlsCertificateField{Name: "Parameter", Start: itemStart, End: at, Children: append([]tlsCertificateField(nil), fields[fieldStart:]...)})
			fields = fields[:fieldStart]
			params++
		}
		fields = append(fields, parameters)
	default:
		return bad()
	}
	if logicalEnd < end {
		add("Trailing Whitespace", "raw", logicalEnd, end)
	}
	add("CRLF", "raw", end, len(wire))
	return fields, map[string]any{
		"Command Name": command, "Parameter Count": params,
		"Trailing Whitespace Accepted":  logicalEnd < end,
		"Extension Semantics Validated": false, "Address Resolution Performed": false,
		"Session State Validated": false, "Delivery Outcome Validated": false,
		"TCP Reassembly Performed": false, "Structured Generation Supported": false,
	}, nil
}

type smtpFieldsLine struct{ start, text, end int }

// A caller-selected, complete seven-bit DATA block including its final dot.
// Header values are unfolded as metadata with individual wire ranges; decoded
// text never receives a fabricated contiguous source span. Structured header
// grammars and MIME are deliberately not inferred by this layout reader.
func decodeSMTPDataFields(wire []byte) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) < 3 || len(wire) > smtpFieldsMaxBytes {
		return nil, nil, fmt.Errorf("smtp-data-fields: invalid byte boundary")
	}
	lines := make([]smtpFieldsLine, 0)
	canonical, dots, decodedBytes, terminal := true, 0, 0, -1
	for at := 0; at < len(wire); {
		size := bytes.Index(wire[at:], []byte("\r\n"))
		if size < 0 {
			return nil, nil, fmt.Errorf("smtp-data-fields: incomplete CRLF line")
		}
		end := at + size
		if size == 1 && wire[at] == '.' {
			if end+2 != len(wire) {
				return nil, nil, fmt.Errorf("smtp-data-fields: bytes after terminator")
			}
			terminal = at
			break
		}
		if len(lines) >= smtpFieldsMaxLines {
			return nil, nil, fmt.Errorf("smtp-data-fields: line resource limit")
		}
		text := at
		if size > 0 && wire[at] == '.' {
			text++
			dots++
			if wire[text] != '.' {
				canonical = false
			}
		}
		if end-text+2 > 1000 {
			return nil, nil, fmt.Errorf("smtp-data-fields: text line exceeds base profile")
		}
		for _, c := range wire[text:end] {
			if c > 127 || c == '\r' || c == '\n' {
				return nil, nil, fmt.Errorf("smtp-data-fields: non-seven-bit or bare newline")
			}
		}
		lines = append(lines, smtpFieldsLine{at, text, end})
		decodedBytes += end - text + 2
		at = end + 2
	}
	if terminal < 0 {
		return nil, nil, fmt.Errorf("smtp-data-fields: missing terminator")
	}
	fields := make([]tlsCertificateField, 0)
	headers := tlsCertificateField{Name: "Header Fields", Start: 0, List: true}
	headerInfo := make([]map[string]any, 0)
	i, folds := 0, 0
	lineParts := func(l smtpFieldsLine, name string) []tlsCertificateField {
		f := make([]tlsCertificateField, 0, 3)
		if l.text > l.start {
			f = append(f, tlsCertificateLeaf("Transparency Dot", "raw", l.start, l.text))
		}
		return append(f, tlsCertificateLeaf(name, "string", l.text, l.end), tlsCertificateLeaf("CRLF", "raw", l.end, l.end+2))
	}
	for i < len(lines) && lines[i].text != lines[i].end {
		if len(headers.Children) >= smtpFieldsMaxHeaders {
			return nil, nil, fmt.Errorf("smtp-data-fields: header resource limit")
		}
		l := lines[i]
		colon := bytes.IndexByte(wire[l.text:l.end], ':')
		if colon <= 0 {
			return nil, nil, fmt.Errorf("smtp-data-fields: invalid header name/colon")
		}
		colon += l.text
		for _, c := range wire[l.text:colon] {
			if c < 33 || c > 126 {
				return nil, nil, fmt.Errorf("smtp-data-fields: invalid header name")
			}
		}
		h := tlsCertificateField{Name: "Header", Start: l.start}
		if l.text > l.start {
			h.Children = append(h.Children, tlsCertificateLeaf("Transparency Dot", "raw", l.start, l.text))
		}
		h.Children = append(h.Children, tlsCertificateLeaf("Field Name", "string", l.text, colon), tlsCertificateLeaf("Colon", "raw", colon, colon+1))
		name := string(wire[l.text:colon])
		values := tlsCertificateField{Name: "Value Lines", Start: colon + 1, List: true}
		var unfolded strings.Builder
		ranges := make([][2]int, 0)
		for {
			valueStart := l.text
			if len(values.Children) == 0 {
				valueStart = colon + 1
			}
			for _, c := range wire[valueStart:l.end] {
				if c != '\t' && (c < 32 || c > 126) {
					return nil, nil, fmt.Errorf("smtp-data-fields: invalid header value")
				}
			}
			valueLine := l
			if len(values.Children) == 0 {
				valueLine.start, valueLine.text = valueStart, valueStart
			}
			values.Children = append(values.Children, tlsCertificateField{Name: "Value Line", Start: valueLine.start, End: l.end + 2, Children: lineParts(valueLine, "Value Fragment")})
			unfolded.Write(wire[valueStart:l.end])
			ranges = append(ranges, [2]int{valueStart, l.end})
			i++
			if i >= len(lines) || lines[i].text == lines[i].end || wire[lines[i].text] != ' ' && wire[lines[i].text] != '\t' {
				break
			}
			l = lines[i]
			folds++
		}
		values.End = l.end + 2
		h.End = values.End
		h.Children = append(h.Children, values)
		headers.End = h.End
		headers.Children = append(headers.Children, h)
		headerInfo = append(headerInfo, map[string]any{"Name": name, "Unfolded Value": unfolded.String(), "Value Relative Byte Ranges": ranges, "Relative Byte Range": [2]int{h.Start, h.End}})
	}
	fields = append(fields, headers)
	bodyStart := headers.End
	if i < len(lines) {
		l := lines[i]
		fields = append(fields, tlsCertificateLeaf("Header Body Separator", "raw", l.start, l.end+2))
		bodyStart = l.end + 2
		i++
	}
	body := tlsCertificateField{Name: "Body Lines", Start: bodyStart, End: terminal, List: true}
	bodyBytes := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		body.Children = append(body.Children, tlsCertificateField{Name: "Body Line", Start: l.start, End: l.end + 2, Children: lineParts(l, "Text")})
		bodyBytes += l.end - l.text + 2
	}
	fields = append(fields, body, tlsCertificateLeaf("DATA Terminator", "raw", terminal, len(wire)))
	return fields, map[string]any{
		"Header Count": len(headerInfo), "Headers": headerInfo, "Fold Count": folds,
		"Body Line Count": len(body.Children), "Body Octets": bodyBytes,
		"Message Octets": decodedBytes, "Transparency Dot Count": dots,
		"Dot Transparency Canonical":          canonical,
		"Structured Header Grammar Validated": false, "Message Conformance Validated": false,
		"MIME Decoded": false, "Session State Validated": false,
		"Delivery Outcome Validated": false, "TCP Reassembly Performed": false,
		"Structured Generation Supported": false,
	}, nil
}

func parseSMTPFields(node *base.Node, process func(*base.Node) (func(bool), error), data bool) error {
	decode, limit := decodeSMTPCommandFields, uint64(512*8)
	if data {
		decode, limit = decodeSMTPDataFields, smtpFieldsMaxBytes*8
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > limit {
		return fmt.Errorf("smtp-fields: explicit nonempty byte boundary within profile limit required")
	}
	return parseCertificateFieldTree(node, process, decode, "smtp-fields")
}
