package stream_parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const pop3FieldsMaxItems = 4096

func pop3FieldsLimit(profile string) int {
	switch profile {
	case "command":
		return 255
	case "status", "stat", "list-one", "uidl-one":
		return 512
	case "challenge", "client-continuation":
		return 65536 // Local bound, not a SASL line-size claim.
	case "list", "uidl", "capa", "message":
		return 1 << 20
	}
	return 0
}

func pop3FieldsDecimal(s string, positive bool) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("pop3-fields: empty decimal")
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("pop3-fields: invalid decimal")
		}
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || positive && v == 0 {
		return 0, fmt.Errorf("pop3-fields: decimal out of supported range")
	}
	return v, nil
}

func pop3FieldsBase64(w []byte, initial bool) (int, error) {
	if initial && bytes.Equal(w, []byte("=")) {
		return 0, nil
	}
	for _, c := range w {
		if c == '\r' || c == '\n' {
			return 0, fmt.Errorf("pop3-fields: newline in base64")
		}
	}
	b, err := base64.StdEncoding.Strict().DecodeString(string(w))
	if err != nil {
		return 0, fmt.Errorf("pop3-fields: invalid canonical base64")
	}
	return len(b), nil // Do not publish decoded continuation values or use them.
}

type pop3FieldsReader struct {
	wire   []byte
	fields []tlsCertificateField
	info   map[string]any
}

func (r *pop3FieldsReader) leaf(name, typ string, a, b int) {
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, a, b))
}

func (r *pop3FieldsReader) line(at, limit int) (int, error) {
	i := bytes.Index(r.wire[at:], []byte("\r\n"))
	if i < 0 || i+2 > limit {
		return 0, fmt.Errorf("pop3-fields: incomplete or oversized line")
	}
	for _, c := range r.wire[at : at+i] {
		if c < 32 || c > 126 {
			return 0, fmt.Errorf("pop3-fields: non-ASCII line")
		}
	}
	return at + i, nil
}

func (r *pop3FieldsReader) command(end int) error {
	line := string(r.wire[:end])
	split := strings.IndexByte(line, ' ')
	if split < 0 {
		split = end
	}
	command := strings.ToUpper(line[:split])
	r.leaf("Command", "string", 0, split)
	at := split
	if at < end {
		r.leaf("Argument Separator", "raw", at, at+1)
		at++
	}
	arg := line[at:]
	r.info["Command Name"] = command
	r.info["Legacy AUTH Probe"] = false
	r.info["Command Argument Layout Validated"] = true
	args := strings.Split(arg, " ")
	number := func(name, s string, at int, positive bool) error {
		v, err := pop3FieldsDecimal(s, positive)
		if err != nil {
			return err
		}
		r.leaf(name, "string", at, at+len(s))
		r.info[name] = v
		return nil
	}
	switch command {
	case "STAT", "NOOP", "RSET", "QUIT", "CAPA", "STLS":
		if split != end {
			return fmt.Errorf("pop3-fields: unexpected arguments")
		}
	case "USER", "PASS":
		if split == end || arg == "" || command == "USER" && strings.Contains(arg, " ") {
			return fmt.Errorf("pop3-fields: missing/invalid argument")
		}
		name := "User Name"
		if command == "PASS" {
			name = "Password"
		}
		r.leaf(name, "raw", at, end)
	case "RETR", "DELE", "LIST", "UIDL":
		if split == end && (command == "LIST" || command == "UIDL") {
			break
		}
		if err := number("Message Number", arg, at, true); err != nil {
			return err
		}
	case "TOP":
		if len(args) != 2 {
			return fmt.Errorf("pop3-fields: TOP needs two decimals")
		}
		if err := number("Message Number", args[0], at, true); err != nil {
			return err
		}
		at += len(args[0])
		r.leaf("Argument Separator", "raw", at, at+1)
		at++
		if err := number("Body Line Count", args[1], at, false); err != nil {
			return err
		}
	case "APOP":
		if len(args) != 2 || len(args[0]) == 0 || len(args[1]) != 32 {
			return fmt.Errorf("pop3-fields: invalid APOP layout")
		}
		for _, c := range []byte(args[1]) {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return fmt.Errorf("pop3-fields: invalid digest text")
			}
		}
		r.leaf("User Name", "raw", at, at+len(args[0]))
		at += len(args[0])
		r.leaf("Argument Separator", "raw", at, at+1)
		at++
		r.leaf("Digest", "raw", at, end)
	case "AUTH":
		if split == end {
			// Preserve the original legacy mechanism-list probe, which the
			// observed modern server rejects, without relabeling it valid.
			r.info["Legacy AUTH Probe"] = true
			r.info["Command Argument Layout Validated"] = false
			break
		}
		if len(args) > 2 || len(args[0]) < 1 || len(args[0]) > 20 {
			return fmt.Errorf("pop3-fields: invalid mechanism layout")
		}
		for _, c := range []byte(args[0]) {
			if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return fmt.Errorf("pop3-fields: invalid mechanism name")
			}
		}
		r.leaf("Mechanism", "string", at, at+len(args[0]))
		at += len(args[0])
		if len(args) == 2 {
			if args[1] == "" {
				return fmt.Errorf("pop3-fields: absent initial value")
			}
			r.leaf("Argument Separator", "raw", at, at+1)
			at++
			size, err := pop3FieldsBase64(r.wire[at:end], true)
			if err != nil {
				return err
			}
			r.leaf("Initial Response", "raw", at, end)
			r.info["Decoded Response Octets"] = size
		}
	default:
		return fmt.Errorf("pop3-fields: unsupported command")
	}
	r.leaf("CRLF", "raw", end, end+2)
	return nil
}

func (r *pop3FieldsReader) listing(at, end int, kind string) (map[string]any, error) {
	start := at
	for at < end && r.wire[at] != ' ' {
		at++
	}
	v, err := pop3FieldsDecimal(string(r.wire[start:at]), kind != "stat")
	if err != nil || at == end {
		return nil, fmt.Errorf("pop3-fields: invalid listing number/separator")
	}
	name := "Message Number"
	if kind == "stat" {
		name = "Message Count"
	}
	r.leaf(name, "string", start, at)
	r.leaf("Listing Separator", "raw", at, at+1)
	at++
	values := map[string]any{name: v}
	start = at
	for at < end && r.wire[at] != ' ' {
		at++
	}
	if kind == "uidl" || kind == "uidl-one" {
		if at != end || end-start < 1 || end-start > 70 {
			return nil, fmt.Errorf("pop3-fields: invalid unique-id")
		}
		r.leaf("Unique ID", "string", start, end)
		values["Unique ID"] = string(r.wire[start:end])
	} else {
		v, err = pop3FieldsDecimal(string(r.wire[start:at]), false)
		if err != nil {
			return nil, err
		}
		r.leaf("Message Octets", "string", start, at)
		values["Message Octets"] = v
		if at < end {
			r.leaf("Listing Additional Text", "string", at, end)
		}
	}
	return values, nil
}

func (r *pop3FieldsReader) capability(at, end int) (map[string]any, error) {
	start := at
	for at < end && r.wire[at] != ' ' {
		if r.wire[at] == '.' {
			return nil, fmt.Errorf("pop3-fields: dot in capability tag")
		}
		at++
	}
	if at == start {
		return nil, fmt.Errorf("pop3-fields: empty capability")
	}
	r.leaf("Capability Name", "string", start, at)
	info := map[string]any{"Name": string(r.wire[start:at])}
	params := tlsCertificateField{Name: "Capability Parameters", Start: at, End: end, List: true}
	var values []string
	for at < end {
		start = at
		at++
		valueAt := at
		for at < end && r.wire[at] != ' ' {
			at++
		}
		if at == valueAt {
			return nil, fmt.Errorf("pop3-fields: empty capability parameter")
		}
		params.Children = append(params.Children, tlsCertificateField{Name: "Parameter", Start: start, End: at, Children: []tlsCertificateField{tlsCertificateLeaf("Separator", "raw", start, valueAt), tlsCertificateLeaf("Value", "string", valueAt, at)}})
		values = append(values, string(r.wire[valueAt:at]))
	}
	r.fields = append(r.fields, params)
	info["Parameters"] = values
	return info, nil
}

func decodePOP3Fields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	limit := pop3FieldsLimit(profile)
	if limit == 0 || len(wire) < 2 || len(wire) > limit {
		return nil, nil, fmt.Errorf("pop3-fields: invalid explicit profile/boundary")
	}
	r := &pop3FieldsReader{wire: wire, info: map[string]any{
		"Layout Context": profile, "Session State Validated": false, "Mailbox State Validated": false,
		"TCP Reassembly Performed": false, "Structured Generation Supported": false,
		"Mechanism Payload Decoded": false, "MIME Decoded": false,
	}}
	lineLimit := 512
	if profile == "command" || profile == "challenge" || profile == "client-continuation" {
		lineLimit = limit
	}
	end, err := r.line(0, lineLimit)
	if err != nil {
		return nil, nil, err
	}
	if profile == "command" || profile == "challenge" || profile == "client-continuation" {
		if end+2 != len(wire) {
			return nil, nil, fmt.Errorf("pop3-fields: extra bytes after line")
		}
		if profile == "command" {
			err = r.command(end)
		} else {
			at := 0
			if profile == "challenge" {
				if end < 2 || wire[0] != '+' || wire[1] != ' ' {
					return nil, nil, fmt.Errorf("pop3-fields: missing continuation prefix")
				}
				r.leaf("Continuation Marker", "raw", 0, 1)
				r.leaf("Separator", "raw", 1, 2)
				at = 2
			}
			cancel := profile == "client-continuation" && bytes.Equal(wire[:end], []byte("*"))
			if cancel {
				r.leaf("Cancel Marker", "raw", 0, end)
			} else {
				var size int
				size, err = pop3FieldsBase64(wire[at:end], false)
				r.leaf("Encoded Value", "raw", at, end)
				r.info["Decoded Response Octets"] = size
			}
			r.info["Cancel Observed"] = cancel
			r.leaf("CRLF", "raw", end, end+2)
		}
		if err != nil {
			return nil, nil, err
		}
		return r.fields, r.info, nil
	}
	positive := bytes.HasPrefix(wire, []byte("+OK"))
	statusEnd := 3
	if !positive {
		statusEnd = 4
		if !bytes.HasPrefix(wire, []byte("-ERR")) {
			return nil, nil, fmt.Errorf("pop3-fields: invalid status")
		}
	}
	if statusEnd > end || statusEnd < end && wire[statusEnd] != ' ' {
		return nil, nil, fmt.Errorf("pop3-fields: invalid status separator")
	}
	r.leaf("Status", "string", 0, statusEnd)
	r.info["Positive Status Observed"] = positive
	textStart := statusEnd
	if textStart < end {
		r.leaf("Status Separator", "raw", textStart, textStart+1)
		textStart++
	}
	if positive && (profile == "stat" || profile == "list-one" || profile == "uidl-one") {
		row, err := r.listing(textStart, end, profile)
		if err != nil {
			return nil, nil, err
		}
		r.info["Listing"] = row
	} else {
		r.leaf("Status Text", "string", textStart, end)
	}
	r.leaf("CRLF", "raw", end, end+2)
	if !positive || profile == "status" || profile == "stat" || profile == "list-one" || profile == "uidl-one" {
		if end+2 != len(wire) {
			return nil, nil, fmt.Errorf("pop3-fields: bytes after single-line reply")
		}
		return r.fields, r.info, nil
	}
	if profile == "message" {
		fields, info, err := decodeSMTPDataFields(wire[end+2:])
		if err != nil {
			return nil, nil, err
		}
		// The shared dot/header layout has no transport-specific operations.
		// Shift every leaf and metadata fragment back into the POP3 response.
		var shift func([]tlsCertificateField)
		shift = func(fs []tlsCertificateField) {
			for i := range fs {
				fs[i].Start += end + 2
				fs[i].End += end + 2
				if fs[i].Name == "DATA Terminator" {
					fs[i].Name = "Response Terminator"
				}
				shift(fs[i].Children)
			}
		}
		shift(fields)
		for _, h := range info["Headers"].([]map[string]any) {
			span := h["Relative Byte Range"].([2]int)
			span[0] += end + 2
			span[1] += end + 2
			h["Relative Byte Range"] = span
			for i := range h["Value Relative Byte Ranges"].([][2]int) {
				h["Value Relative Byte Ranges"].([][2]int)[i][0] += end + 2
				h["Value Relative Byte Ranges"].([][2]int)[i][1] += end + 2
			}
		}
		r.fields = append(r.fields, fields...)
		r.info["Message Fields"] = info
		mimeInfo, mimeErr := decodeMIMEDotMessage(wire, end+2)
		if mimeErr != nil {
			r.info["MIME Decoding Error"] = mimeErr.Error()
		} else {
			r.info["MIME Fields"] = mimeInfo
			r.info["MIME Decoded"] = mimeInfo["All Transfers Decoded"]
		}
		return r.fields, r.info, nil
	}
	list := tlsCertificateField{Name: "Response Items", Start: end + 2, List: true}
	rows := make([]map[string]any, 0)
	at := end + 2
	for {
		end, err = r.line(at, 512)
		if err != nil {
			return nil, nil, err
		}
		if end-at == 1 && wire[at] == '.' {
			if end+2 != len(wire) {
				return nil, nil, fmt.Errorf("pop3-fields: bytes after list terminator")
			}
			list.End = at
			r.fields = append(r.fields, list)
			r.leaf("Response Terminator", "raw", at, end+2)
			break
		}
		if len(rows) >= pop3FieldsMaxItems {
			return nil, nil, fmt.Errorf("pop3-fields: list resource limit")
		}
		itemStart, fieldStart := at, len(r.fields)
		var row map[string]any
		if profile == "capa" {
			row, err = r.capability(at, end)
		} else {
			row, err = r.listing(at, end, profile)
		}
		if err != nil {
			return nil, nil, err
		}
		r.leaf("CRLF", "raw", end, end+2)
		row["Relative Byte Range"] = [2]int{itemStart, end + 2}
		rows = append(rows, row)
		list.Children = append(list.Children, tlsCertificateField{Name: "Item", Start: itemStart, End: end + 2, Children: append([]tlsCertificateField(nil), r.fields[fieldStart:]...)})
		r.fields = r.fields[:fieldStart]
		at = end + 2
	}
	r.info["Items"] = rows
	r.info["Item Count"] = len(rows)
	return r.fields, r.info, nil
}

func parsePOP3Fields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	limit := pop3FieldsLimit(profile)
	if limit == 0 || !bounded || bits == 0 || bits%8 != 0 || bits > uint64(limit)*8 {
		return fmt.Errorf("pop3-fields: explicit nonempty byte boundary within profile limit required")
	}
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodePOP3Fields(w, profile) }, "pop3-fields")
}
