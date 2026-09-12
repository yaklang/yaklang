package stream_parser

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const mysqlFieldsMaxBytes = 1 << 20
const mysqlFieldsMaxItems = 4096

// These are explicit, uncompressed classic-protocol layouts, not a session
// detector. Callers supply direction, phase and capability context through the
// selected entry. In particular row bytes cannot identify commands or replies.
type mysqlFieldsReader struct {
	wire    []byte
	at, end int
	err     error
	fields  []tlsCertificateField
}

func (r *mysqlFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("mysql-fields: %s at byte %d", s, r.at)
	}
}
func (r *mysqlFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	start := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, start, r.at))
	return r.wire[start:r.at]
}
func (r *mysqlFieldsReader) number(name string, n int) uint64 {
	typ := fmt.Sprintf("uint%d", n*8)
	if n == 3 {
		typ = "uint32" // Stored scalar has an exact 24-bit wire span.
	}
	b := r.take(name, typ, n)
	var v uint64
	for i, c := range b {
		v |= uint64(c) << (8 * i)
	}
	return v
}
func (r *mysqlFieldsReader) zero(name string, n int) {
	for _, b := range r.take(name, "raw", n) {
		if b != 0 {
			r.fail("nonzero " + name)
			break
		}
	}
}
func (r *mysqlFieldsReader) nul(name, typ string) []byte {
	if r.err != nil {
		return nil
	}
	n := bytes.IndexByte(r.wire[r.at:r.end], 0)
	if n < 0 {
		r.fail("unterminated " + name)
		return nil
	}
	b := r.take(name, typ, n)
	r.zero("NUL", 1)
	return b
}
func (r *mysqlFieldsReader) length(name string, nullable bool) (uint64, bool) {
	b := r.number(name+" Prefix", 1)
	switch b {
	case 0xfb:
		if !nullable {
			r.fail("NULL length outside row")
		}
		return 0, true
	case 0xfc:
		return r.number(name, 2), false
	case 0xfd:
		return r.number(name, 3), false
	case 0xfe:
		return r.number(name, 8), false
	case 0xff:
		r.fail("invalid length marker")
	}
	return b, false
}
func (r *mysqlFieldsReader) sized(name, typ string, nullable bool) ([]byte, bool, [2]int) {
	n, null := r.length(name+" Length", nullable)
	span := [2]int{r.at, r.at}
	if r.err != nil {
		return nil, null, span
	}
	if n > uint64(r.end-r.at) {
		r.fail("length exceeds " + name + " boundary")
		return nil, null, span
	}
	if null {
		return nil, true, span
	}
	b := r.take(name, typ, int(n))
	span[1] = r.at
	return b, false, span
}
func (r *mysqlFieldsReader) group(name string, at, index int, list bool) {
	children := append([]tlsCertificateField(nil), r.fields[index:]...)
	r.fields = append(r.fields[:index], tlsCertificateField{Name: name, Start: at, End: r.at, Children: children, List: list})
}
func (r *mysqlFieldsReader) packet() (int, int, uint64) {
	start, index := r.at, len(r.fields)
	r.end = len(r.wire)
	n := r.number("Payload Length", 3)
	seq := r.number("Sequence ID", 1)
	if r.err == nil && (n == 0 || n > uint64(r.end-r.at)) {
		r.fail("incomplete/empty classic packet")
	}
	if r.err == nil {
		r.end = r.at + int(n)
	}
	return start, index, seq
}
func (r *mysqlFieldsReader) finishPacket(start, index int, name string) {
	if r.at != r.end {
		r.fail("unconsumed packet payload")
	}
	r.group(name, start, index, false)
}
func (r *mysqlFieldsReader) greeting(info map[string]any) {
	if r.number("Protocol Version", 1) != 10 {
		r.fail("not HandshakeV10")
	}
	info["Server Version"] = string(r.nul("Server Version", "string"))
	info["Connection ID"] = r.number("Connection ID", 4)
	r.take("Plugin Data Part 1", "raw", 8)
	r.zero("Filler", 1)
	low := r.number("Capabilities Low", 2)
	// The historical short greeting is a different layout, not guessed here.
	info["Character Set"] = r.number("Character Set", 1)
	info["Status Flags"] = r.number("Status Flags", 2)
	high := r.number("Capabilities High", 2)
	caps := low | high<<16
	info["Capabilities"] = caps
	n := r.number("Plugin Data Length", 1)
	if caps&(1<<19) == 0 && n != 0 {
		r.fail("plugin length without plugin capability")
	}
	r.zero("Reserved", 6)
	if caps&1 == 0 {
		info["MariaDB Extended Capabilities"] = r.number("MariaDB Extended Capabilities", 4)
	} else {
		r.zero("Reserved Tail", 4)
	}
	if caps&(1<<15) != 0 {
		part := 13
		if n > 21 {
			part = int(n) - 8
		}
		r.take("Plugin Data Part 2", "raw", part-1)
		r.zero("Plugin Data Terminator", 1)
	}
	if caps&(1<<19) != 0 {
		info["Plugin Name"] = string(r.nul("Plugin Name", "string"))
	}
}
func (r *mysqlFieldsReader) response(info map[string]any, maria, ssl bool) {
	caps := r.number("Client Capabilities", 4)
	info["Capabilities"] = caps
	if caps&(1<<9) == 0 {
		r.fail("CLIENT_PROTOCOL_41 required")
	}
	info["Maximum Packet Size"] = r.number("Maximum Packet Size", 4)
	info["Character Set"] = r.number("Character Set", 1)
	if maria {
		r.zero("Reserved", 19)
		info["MariaDB Extended Capabilities"] = r.number("MariaDB Extended Capabilities", 4)
	} else {
		r.zero("Reserved", 23)
	}
	if ssl {
		if caps&(1<<11) == 0 {
			r.fail("CLIENT_SSL required")
		}
		info["TLS Requested"] = true
		return
	}
	// Names and plugin responses retain their original octets; no credential
	// decoding, identity or authentication-outcome assertion is made.
	r.nul("User Name", "raw")
	if caps&(1<<21) != 0 {
		r.sized("Plugin Response", "raw", false)
	} else if caps&(1<<15) != 0 {
		n := r.number("Plugin Response Length", 1)
		r.take("Plugin Response", "raw", int(n))
	} else {
		r.nul("Plugin Response", "raw")
	}
	if caps&(1<<3) != 0 {
		r.nul("Database Name", "raw")
	}
	if caps&(1<<19) != 0 {
		info["Plugin Name"] = string(r.nul("Plugin Name", "string"))
	}
	if caps&(1<<20) != 0 {
		n, _ := r.length("Connection Attributes Length", false)
		if r.err != nil || n > uint64(r.end-r.at) {
			r.fail("invalid attributes boundary")
			return
		}
		end := r.end
		r.end = r.at + int(n)
		start, index := r.at, len(r.fields)
		attrs := make([]map[string]any, 0)
		for r.err == nil && r.at < r.end {
			if len(attrs) == mysqlFieldsMaxItems {
				r.fail("attributes resource limit")
				break
			}
			a, f := r.at, len(r.fields)
			key, _, ks := r.sized("Attribute Key", "raw", false)
			val, _, vs := r.sized("Attribute Value", "raw", false)
			attrs = append(attrs, map[string]any{"Key": bytes.Clone(key), "Value": bytes.Clone(val), "Key Relative Byte Range": ks, "Value Relative Byte Range": vs})
			r.group("Attribute", a, f, false)
		}
		r.group("Connection Attributes", start, index, true)
		r.end = end
		info["Connection Attributes"] = attrs
	}
	if caps&(1<<26) != 0 {
		level := r.number("Zstd Compression Level", 1)
		if level < 1 || level > 22 {
			r.fail("invalid zstd level")
		}
	}
}
func (r *mysqlFieldsReader) command(info map[string]any) {
	cmd := r.number("Command", 1)
	info["Command Code"] = cmd
	switch cmd {
	case 1:
		info["Command Name"] = "COM_QUIT"
	case 14:
		info["Command Name"] = "COM_PING"
	case 2:
		info["Command Name"] = "COM_INIT_DB"
		r.take("Database Name", "raw", r.end-r.at)
	case 3:
		info["Command Name"] = "COM_QUERY"
		r.take("Query Bytes", "raw", r.end-r.at)
	default:
		r.fail("unsupported command grammar")
	}
}
func (r *mysqlFieldsReader) ok(info map[string]any, tracked bool) {
	if r.number("OK Marker", 1) != 0 {
		r.fail("not classic OK packet")
	}
	info["Affected Rows"], _ = r.length("Affected Rows", false)
	info["Last Insert ID"], _ = r.length("Last Insert ID", false)
	status := r.number("Status Flags", 2)
	info["Status Flags"] = status
	info["Warnings"] = r.number("Warnings", 2)
	if tracked {
		// Empty info may be omitted, as in the original MariaDB greeting reply.
		if r.at < r.end {
			r.sized("Info", "raw", false)
		}
		if status&(1<<14) != 0 {
			n, _ := r.length("Session State Length", false)
			if r.err != nil || n > uint64(r.end-r.at) {
				r.fail("invalid session state boundary")
				return
			}
			end := r.end
			r.end = r.at + int(n)
			a, f, count := r.at, len(r.fields), 0
			for r.err == nil && r.at < r.end {
				if count == mysqlFieldsMaxItems {
					r.fail("session state resource limit")
					break
				}
				count++
				s, i := r.at, len(r.fields)
				r.number("State Type", 1)
				r.sized("State Data", "raw", false)
				r.group("State", s, i, false)
			}
			r.group("Session State", a, f, true)
			r.end = end
		}
	} else {
		r.take("Info", "raw", r.end-r.at)
	}
}
func (r *mysqlFieldsReader) eof(info map[string]any) {
	if r.end-r.at != 5 || r.number("EOF Marker", 1) != 0xfe {
		r.fail("not protocol41 EOF")
		return
	}
	info["Warnings"] = r.number("Warnings", 2)
	info["Status Flags"] = r.number("Status Flags", 2)
}
func (r *mysqlFieldsReader) error41(info map[string]any) {
	if r.number("Error Marker", 1) != 0xff {
		r.fail("not ERR packet")
	}
	info["Error Code"] = r.number("Error Code", 2)
	if r.number("SQL State Marker", 1) != '#' {
		r.fail("missing SQL state marker")
	}
	r.take("SQL State", "string", 5)
	r.take("Error Message", "raw", r.end-r.at)
}
func (r *mysqlFieldsReader) column(maria bool) map[string]any {
	info := map[string]any{}
	for _, name := range []string{"Catalog", "Schema", "Table Alias", "Table Name", "Column Alias", "Column Name"} {
		v, _, span := r.sized(name, "raw", false)
		info[name] = bytes.Clone(v)
		info[name+" Relative Byte Range"] = span
	}
	if maria {
		n, _ := r.length("Extended Metadata Length", false)
		if r.err != nil || n > uint64(r.end-r.at) {
			r.fail("invalid column metadata boundary")
			return info
		}
		end := r.end
		r.end = r.at + int(n)
		a, f, count := r.at, len(r.fields), 0
		for r.err == nil && r.at < r.end {
			if count == mysqlFieldsMaxItems {
				r.fail("column metadata resource limit")
				break
			}
			count++
			s, i := r.at, len(r.fields)
			r.number("Metadata Type", 1)
			r.sized("Metadata Value", "raw", false)
			r.group("Metadata Item", s, i, false)
		}
		r.group("Extended Metadata", a, f, true)
		r.end = end
	}
	n, _ := r.length("Fixed Fields Length", false)
	if n != 12 {
		r.fail("unsupported column fixed layout")
	}
	info["Character Set"] = r.number("Character Set", 2)
	info["Maximum Column Length"] = r.number("Maximum Column Length", 4)
	info["Column Type"] = r.number("Column Type", 1)
	info["Column Flags"] = r.number("Column Flags", 2)
	info["Decimals"] = r.number("Decimals", 1)
	r.zero("Column Reserved", 2)
	return info
}
func (r *mysqlFieldsReader) resultset(info map[string]any, maria bool) {
	start, index, seq := r.packet()
	count, _ := r.length("Column Count", false)
	if count == 0 || count > mysqlFieldsMaxItems {
		r.fail("invalid column count/resource limit")
	}
	info["Column Count"] = count
	if maria {
		flag := r.number("Metadata Follows", 1)
		if flag != 1 {
			r.fail("fresh metadata required; no cached columns")
		}
		info["Metadata Follows"] = flag
	}
	r.finishPacket(start, index, "Column Count Packet")
	next := func() (int, int) {
		s, i, got := r.packet()
		seq = (seq + 1) & 255
		if got != seq {
			r.fail("resultset sequence discontinuity")
		}
		return s, i
	}
	a, f := r.at, len(r.fields)
	cols := make([]map[string]any, 0)
	for c := uint64(0); r.err == nil && c < count; c++ {
		s, i := next()
		cols = append(cols, r.column(maria))
		r.finishPacket(s, i, "Column Definition Packet")
	}
	r.group("Columns", a, f, true)
	info["Columns"] = cols
	if r.err != nil {
		return
	}
	s, i := next()
	columnEnd := map[string]any{}
	r.eof(columnEnd)
	r.finishPacket(s, i, "Column EOF Packet")
	info["Column EOF"] = columnEnd
	a, f = r.at, len(r.fields)
	rows := make([][]map[string]any, 0)
	values := 0
	for r.err == nil {
		s, i = next()
		if r.err != nil {
			break
		}
		if r.end-r.at == 5 && r.wire[r.at] == 0xfe {
			// Group rows before appending their terminator's packet fields.
			header := append([]tlsCertificateField(nil), r.fields[i:]...)
			r.fields = r.fields[:i]
			saved := r.at
			r.at = s
			r.group("Rows", a, f, true)
			r.at = saved
			i = len(r.fields)
			r.fields = append(r.fields, header...)
			last := map[string]any{}
			r.eof(last)
			r.finishPacket(s, i, "Result EOF Packet")
			info["Result EOF"] = last
			break
		}
		if len(rows) == mysqlFieldsMaxItems || values+int(count) > mysqlFieldsMaxItems {
			r.fail("rows/values resource limit")
			break
		}
		row := make([]map[string]any, 0)
		for c := uint64(0); r.err == nil && c < count; c++ {
			v, null, span := r.sized("Column Value", "raw", true)
			row = append(row, map[string]any{"NULL": null, "Value": bytes.Clone(v), "Relative Byte Range": span})
			values++
		}
		rows = append(rows, row)
		r.finishPacket(s, i, "Text Row Packet")
	}
	info["Rows"] = rows
	info["Row Count"] = len(rows)
}

func mysqlFieldsProfileValid(p string) bool {
	switch p {
	case "greeting", "response41", "mariadb-response41", "ssl-request", "mariadb-ssl-request", "command", "ok41", "ok41-session-track", "error41", "eof41", "text-resultset41", "mariadb-text-resultset":
		return true
	}
	return false
}
func decodeMySQLFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if !mysqlFieldsProfileValid(profile) {
		return nil, nil, fmt.Errorf("mysql-fields: unknown explicit profile")
	}
	if len(wire) == 0 || len(wire) > mysqlFieldsMaxBytes {
		return nil, nil, fmt.Errorf("mysql-fields: 1..1048576 byte boundary required")
	}
	r := &mysqlFieldsReader{wire: wire, end: len(wire)}
	info := map[string]any{"Layout Context": profile, "Session State Validated": false, "Payload Decrypted": false, "Query Executed": false}
	if profile == "text-resultset41" || profile == "mariadb-text-resultset" {
		r.resultset(info, profile == "mariadb-text-resultset")
	} else {
		s, i, seq := r.packet()
		info["Sequence ID"] = seq
		switch profile {
		case "greeting":
			r.greeting(info)
		case "response41", "mariadb-response41":
			r.response(info, profile == "mariadb-response41", false)
		case "ssl-request", "mariadb-ssl-request":
			r.response(info, profile == "mariadb-ssl-request", true)
		case "command":
			r.command(info)
		case "ok41", "ok41-session-track":
			r.ok(info, profile == "ok41-session-track")
		case "error41":
			r.error41(info)
		case "eof41":
			r.eof(info)
		}
		r.finishPacket(s, i, "Classic Packet")
	}
	if r.at != len(wire) {
		r.fail("bytes outside selected message")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	return r.fields, info, nil
}
func parseMySQLFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	if !mysqlFieldsProfileValid(profile) {
		return fmt.Errorf("mysql-fields: unknown explicit profile")
	}
	return parseExactByteFieldTreeWithEndian(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeMySQLFields(w, profile) }, "mysql-fields", "little")
}
