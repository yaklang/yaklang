package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const postgresqlFieldsMaxBytes = 1 << 20
const postgresqlFieldsMaxItems = 4096
const postgresqlFieldsMaxLeaves = 65536

// Protocol 3.0, explicit direction/phase, complete bounded messages only.
// No connection-global cache, query evaluation or implicit 'p' interpretation.
type postgresqlFieldsReader struct {
	wire            []byte
	at, end, leaves int
	err             error
	fields          []tlsCertificateField
}

func (r *postgresqlFieldsReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("postgresql-fields: %s at byte %d", s, r.at)
	}
}
func (r *postgresqlFieldsReader) take(name, typ string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.end-r.at {
		r.fail("incomplete " + name)
		return nil
	}
	if r.leaves == postgresqlFieldsMaxLeaves {
		r.fail("field resource limit")
		return nil
	}
	r.leaves++
	s := r.at
	r.at += n
	r.fields = append(r.fields, tlsCertificateLeaf(name, typ, s, r.at))
	return r.wire[s:r.at]
}
func (r *postgresqlFieldsReader) uint(name string, n int) uint64 {
	var v uint64
	for _, b := range r.take(name, fmt.Sprintf("uint%d", 8*n), n) {
		v = v<<8 | uint64(b)
	}
	return v
}
func (r *postgresqlFieldsReader) sint(name string, n int) int64 {
	var v uint64
	for _, b := range r.take(name, fmt.Sprintf("int%d", 8*n), n) {
		v = v<<8 | uint64(b)
	}
	if n == 2 {
		return int64(int16(v))
	}
	return int64(int32(v))
}
func (r *postgresqlFieldsReader) nul(name, typ string) []byte {
	if r.err != nil {
		return nil
	}
	n := bytes.IndexByte(r.wire[r.at:r.end], 0)
	if n < 0 {
		r.fail("unterminated " + name)
		return nil
	}
	v := r.take(name, typ, n)
	r.take("NUL", "raw", 1)
	return v
}
func (r *postgresqlFieldsReader) group(name string, start, index int, list bool) {
	children := append([]tlsCertificateField(nil), r.fields[index:]...)
	r.fields = append(r.fields[:index], tlsCertificateField{Name: name, Start: start, End: r.at, Children: children, List: list})
}
func (r *postgresqlFieldsReader) count(name string, n int) int {
	c := r.uint(name, n)
	if c > postgresqlFieldsMaxItems {
		r.fail(name + " resource limit")
		return 0
	}
	return int(c)
}
func (r *postgresqlFieldsReader) format(name string, n int) uint64 {
	v := r.uint(name, n)
	if v > 1 {
		r.fail("unsupported " + name)
	}
	return v
}
func (r *postgresqlFieldsReader) formats(name string) []uint64 {
	c := r.count(name+" Count", 2)
	s, i := r.at, len(r.fields)
	v := make([]uint64, 0)
	for j := 0; r.err == nil && j < c; j++ {
		v = append(v, r.format("Format Code", 2))
	}
	r.group(name, s, i, true)
	return v
}
func (r *postgresqlFieldsReader) value(name string) map[string]any {
	n := r.sint(name+" Length", 4)
	start := r.at
	info := map[string]any{"NULL": n == -1, "Octets": n}
	if n < -1 {
		r.fail("negative value length other than NULL")
	}
	if n >= 0 {
		info["Value"] = bytes.Clone(r.take(name, "raw", int(n)))
	}
	info["Relative Byte Range"] = [2]int{start, r.at}
	return info
}
func (r *postgresqlFieldsReader) values(name string) []map[string]any {
	c := r.count(name+" Count", 2)
	s, i := r.at, len(r.fields)
	v := make([]map[string]any, 0)
	for j := 0; r.err == nil && j < c; j++ {
		a, f := r.at, len(r.fields)
		v = append(v, r.value("Value"))
		r.group("Item", a, f, false)
	}
	r.group(name, s, i, true)
	return v
}
func (r *postgresqlFieldsReader) oids(name string) []uint64 {
	c := r.count(name+" Count", 2)
	s, i := r.at, len(r.fields)
	v := make([]uint64, 0)
	for j := 0; r.err == nil && j < c; j++ {
		v = append(v, r.uint("Type OID", 4))
	}
	r.group(name, s, i, true)
	return v
}
func (r *postgresqlFieldsReader) parameters(info map[string]any) {
	s, i := r.at, len(r.fields)
	items := make([]map[string]any, 0)
	hasUser := false
	for r.err == nil {
		if r.at == r.end {
			r.fail("missing parameter terminator")
			break
		}
		if r.wire[r.at] == 0 {
			break
		}
		if len(items) == postgresqlFieldsMaxItems {
			r.fail("parameters resource limit")
			break
		}
		a, f := r.at, len(r.fields)
		ks := r.at
		k := r.nul("Parameter Name", "raw")
		vs := r.at
		v := r.nul("Parameter Value", "raw")
		items = append(items, map[string]any{"Name": bytes.Clone(k), "Value": bytes.Clone(v), "Name Relative Byte Range": [2]int{ks, ks + len(k)}, "Value Relative Byte Range": [2]int{vs, vs + len(v)}})
		if bytes.Equal(k, []byte("user")) {
			hasUser = true
		}
		r.group("Parameter", a, f, false)
	}
	r.group("Parameters", s, i, true)
	r.take("Parameters Terminator", "raw", 1)
	if !hasUser {
		r.fail("startup user parameter required")
	}
	info["Parameters"] = items
}
func (r *postgresqlFieldsReader) untyped(info map[string]any, profile string) {
	if profile == "ssl-response" || profile == "gss-response" {
		v := r.uint("Transport Response", 1)
		allowed := byte('S')
		if profile == "gss-response" {
			allowed = 'G'
		}
		if v != uint64(allowed) && v != 'N' {
			r.fail("invalid transport response")
		}
		info["Response Code"] = string([]byte{byte(v)})
		info["Positive Response Observed"] = v == uint64(allowed)
		return
	}
	n := r.uint("Length", 4)
	if n != uint64(len(r.wire)) {
		r.fail("untyped length mismatch")
	}
	code := r.uint("Protocol Code", 4)
	info["Protocol Code"] = code
	switch profile {
	case "startup":
		if code != 196608 {
			r.fail("protocol 3.0 required")
		}
		r.parameters(info)
	case "ssl-request":
		if code != 80877103 {
			r.fail("not SSLRequest")
		}
	case "gss-request":
		if code != 80877104 {
			r.fail("not GSSENCRequest")
		}
	case "cancel":
		if code != 80877102 {
			r.fail("not CancelRequest")
		}
		info["Process ID"] = r.uint("Process ID", 4)
		r.take("Cancellation Key", "raw", 4)
	}
}
func (r *postgresqlFieldsReader) frontend(info map[string]any, typ byte) {
	switch typ {
	case 'P':
		info["Message Name"] = "Parse"
		r.nul("Statement Name", "raw")
		r.nul("Query Bytes", "raw")
		info["Parameter Type OIDs"] = r.oids("Parameter Types")
	case 'B':
		info["Message Name"] = "Bind"
		r.nul("Portal Name", "raw")
		r.nul("Statement Name", "raw")
		formats := r.formats("Parameter Formats")
		values := r.values("Parameters")
		if len(formats) != 0 && len(formats) != 1 && len(formats) != len(values) {
			r.fail("parameter format count mismatch")
		}
		for j, v := range values {
			format := uint64(0)
			if len(formats) == 1 {
				format = formats[0]
			} else if len(formats) == len(values) {
				format = formats[j]
			}
			v["Format Code"] = format
		}
		info["Parameter Formats"] = formats
		info["Parameters"] = values
		info["Result Formats"] = r.formats("Result Formats")
	case 'D', 'C':
		info["Message Name"] = "Describe"
		if typ == 'C' {
			info["Message Name"] = "Close"
		}
		target := r.uint("Target Type", 1)
		if target != 'S' && target != 'P' {
			r.fail("invalid statement/portal target")
		}
		r.nul("Target Name", "raw")
	case 'E':
		info["Message Name"] = "Execute"
		r.nul("Portal Name", "raw")
		n := r.sint("Maximum Rows", 4)
		if n < 0 {
			r.fail("negative maximum rows")
		}
		info["Maximum Rows"] = n
	case 'S':
		info["Message Name"] = "Sync"
	case 'H':
		info["Message Name"] = "Flush"
	case 'X':
		info["Message Name"] = "Terminate"
	case 'Q':
		info["Message Name"] = "Query"
		r.nul("Query Bytes", "raw")
	case 'd':
		info["Message Name"] = "CopyData"
		r.take("Copy Stream Bytes", "raw", r.end-r.at)
	case 'c':
		info["Message Name"] = "CopyDone"
	case 'f':
		info["Message Name"] = "CopyFail"
		r.nul("Message Bytes", "raw")
	default:
		r.fail("unsupported frontend type or missing phase context")
	}
}
func (r *postgresqlFieldsReader) columnDescriptions(info map[string]any) {
	c := r.count("Column Count", 2)
	s, i := r.at, len(r.fields)
	cols := make([]map[string]any, 0)
	for j := 0; r.err == nil && j < c; j++ {
		a, f := r.at, len(r.fields)
		name := r.nul("Column Name", "raw")
		col := map[string]any{"Name": bytes.Clone(name), "Name Relative Byte Range": [2]int{a, a + len(name)}}
		col["Table OID"] = r.uint("Table OID", 4)
		col["Attribute Number"] = r.sint("Attribute Number", 2)
		col["Type OID"] = r.uint("Type OID", 4)
		col["Type Size"] = r.sint("Type Size", 2)
		col["Type Modifier"] = r.sint("Type Modifier", 4)
		col["Format Code"] = r.format("Format Code", 2)
		cols = append(cols, col)
		r.group("Column", a, f, false)
	}
	r.group("Columns", s, i, true)
	info["Columns"] = cols
}
func (r *postgresqlFieldsReader) auth(info map[string]any, scram bool) {
	code := r.uint("Method Code", 4)
	info["Method Code"] = code
	switch code {
	case 0, 2, 3, 7, 9: // Fixed protocol indicators, not independently verified outcomes.
	case 5:
		r.take("MD5 Salt", "raw", 4)
	case 8:
		r.take("GSS Continue Data", "raw", r.end-r.at)
	case 10:
		s, i := r.at, len(r.fields)
		names := make([]string, 0)
		for r.err == nil {
			if r.at == r.end {
				r.fail("missing mechanisms terminator")
				break
			}
			if r.wire[r.at] == 0 {
				break
			}
			if len(names) == postgresqlFieldsMaxItems {
				r.fail("mechanisms resource limit")
				break
			}
			name := r.nul("Mechanism Name", "string")
			if !postgresqlFieldsMechanism(name) {
				r.fail("invalid SASL mechanism")
			}
			names = append(names, string(name))
		}
		r.group("Mechanisms", s, i, true)
		r.take("Mechanisms Terminator", "raw", 1)
		if len(names) == 0 {
			r.fail("empty mechanism offer")
		}
		info["Mechanisms"] = names
	case 11, 12:
		if scram {
			phase := "server-first"
			if code == 12 {
				phase = "server-final"
			}
			r.scram(info, phase)
		} else {
			r.take("SASL Data", "raw", r.end-r.at)
		}
	default:
		r.fail("unsupported method layout")
	}
}
func (r *postgresqlFieldsReader) backend(info map[string]any, typ byte, scram bool) {
	switch typ {
	case 'R':
		info["Message Name"] = "Authentication"
		r.auth(info, scram)
	case 'S':
		info["Message Name"] = "ParameterStatus"
		info["Parameter Name"] = bytes.Clone(r.nul("Parameter Name", "raw"))
		info["Parameter Value"] = bytes.Clone(r.nul("Parameter Value", "raw"))
	case 'K':
		info["Message Name"] = "BackendKeyData"
		info["Process ID"] = r.uint("Process ID", 4)
		r.take("Cancellation Key", "raw", 4)
	case 'Z':
		info["Message Name"] = "ReadyForQuery"
		v := r.uint("Transaction Status", 1)
		if v != 'I' && v != 'T' && v != 'E' {
			r.fail("invalid transaction status")
		}
		info["Transaction Status"] = string([]byte{byte(v)})
	case '1':
		info["Message Name"] = "ParseComplete"
	case '2':
		info["Message Name"] = "BindComplete"
	case '3':
		info["Message Name"] = "CloseComplete"
	case 'n':
		info["Message Name"] = "NoData"
	case 'I':
		info["Message Name"] = "EmptyQueryResponse"
	case 's':
		info["Message Name"] = "PortalSuspended"
	case 'C':
		info["Message Name"] = "CommandComplete"
		info["Command Tag"] = bytes.Clone(r.nul("Command Tag", "raw"))
	case 'T':
		info["Message Name"] = "RowDescription"
		r.columnDescriptions(info)
	case 'D':
		info["Message Name"] = "DataRow"
		info["Values"] = r.values("Columns")
		info["Row Metadata Applied"] = false
	case 't':
		info["Message Name"] = "ParameterDescription"
		info["Parameter Type OIDs"] = r.oids("Parameter Types")
	case 'E', 'N':
		info["Message Name"] = "ErrorResponse"
		if typ == 'N' {
			info["Message Name"] = "NoticeResponse"
		}
		s, i := r.at, len(r.fields)
		items := make([]map[string]any, 0)
		for r.err == nil {
			if r.at == r.end {
				r.fail("missing error/notice terminator")
				break
			}
			if r.wire[r.at] == 0 {
				break
			}
			if len(items) == postgresqlFieldsMaxItems {
				r.fail("error fields resource limit")
				break
			}
			a, f := r.at, len(r.fields)
			code := r.uint("Field Code", 1)
			v := r.nul("Field Value", "raw")
			items = append(items, map[string]any{"Code": code, "Value": bytes.Clone(v)})
			r.group("Field", a, f, false)
		}
		r.group("Message Fields", s, i, true)
		r.take("Fields Terminator", "raw", 1)
		if len(items) == 0 {
			r.fail("empty error/notice")
		}
		info["Message Fields"] = items
	case 'A':
		info["Message Name"] = "NotificationResponse"
		info["Process ID"] = r.uint("Process ID", 4)
		r.nul("Channel", "raw")
		r.nul("Notification Bytes", "raw")
	case 'G', 'H', 'W':
		info["Message Name"] = map[byte]string{'G': "CopyInResponse", 'H': "CopyOutResponse", 'W': "CopyBothResponse"}[typ]
		f := r.format("Overall Format", 1)
		codes := r.formats("Column Formats")
		if f == 0 {
			for _, c := range codes {
				if c != 0 {
					r.fail("text copy with binary column")
				}
			}
		}
		info["Column Formats"] = codes
	case 'd':
		info["Message Name"] = "CopyData"
		r.take("Copy Stream Bytes", "raw", r.end-r.at)
	case 'c':
		info["Message Name"] = "CopyDone"
	case 'V':
		info["Message Name"] = "FunctionCallResponse"
		info["Value"] = r.value("Value")
	default:
		r.fail("unsupported backend type")
	}
}
func postgresqlFieldsMechanism(s []byte) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func (r *postgresqlFieldsReader) typed(profile string) map[string]any {
	start, index := r.at, len(r.fields)
	r.end = len(r.wire)
	typ := byte(r.uint("Message Type", 1))
	n := r.uint("Length", 4)
	if n < 4 || n > uint64(r.end-start-1) {
		r.fail("invalid typed message length")
	}
	if r.err == nil {
		r.end = start + 1 + int(n)
	}
	info := map[string]any{"Type Code": string([]byte{typ}), "Session State Validated": false, "Query Executed": false}
	switch profile {
	case "frontend", "frontend-block":
		r.frontend(info, typ)
	case "backend", "backend-block", "backend-scram", "backend-scram-block":
		r.backend(info, typ, strings.Contains(profile, "scram"))
	case "password", "sasl-initial", "sasl-response", "sasl-scram-response":
		if typ != 'p' {
			r.fail("not phase-specific p message")
		}
		switch profile {
		case "password":
			info["Message Name"] = "PasswordMessage"
			r.nul("Password Response", "raw")
		case "sasl-initial":
			info["Message Name"] = "SASLInitialResponse"
			mechanism := r.nul("Mechanism Name", "string")
			if !postgresqlFieldsMechanism(mechanism) {
				r.fail("invalid SASL mechanism")
			}
			info["Mechanism"] = string(mechanism)
			n := r.sint("Initial Response Length", 4)
			info["Initial Response Present"] = n != -1
			if n < -1 || n >= 0 && n != int64(r.end-r.at) {
				r.fail("invalid SASL initial length")
			}
			if n >= 0 {
				if string(mechanism) == "SCRAM-SHA-256" || string(mechanism) == "SCRAM-SHA-256-PLUS" {
					r.scram(info, "client-first")
				} else {
					r.take("SASL Initial Data", "raw", int(n))
				}
			}
		case "sasl-response":
			info["Message Name"] = "SASLResponse"
			r.take("SASL Data", "raw", r.end-r.at)
		case "sasl-scram-response":
			info["Message Name"] = "SASLResponse"
			r.scram(info, "client-final")
		}
	}
	if r.at != r.end {
		r.fail("unconsumed message bytes")
	}
	info["Relative Byte Range"] = [2]int{start, r.at}
	r.group("Message", start, index, false)
	return info
}

// Local RowDescription evidence may annotate later rows in this same bounded
// backend block. A command/ready boundary clears it. No global OID registry or
// previous invocation supplies types, encoding, portal or statement identity.
func postgresqlFieldsAnnotateRow(row, description map[string]any) {
	cols := description["Columns"].([]map[string]any)
	values := row["Values"].([]map[string]any)
	if len(cols) != len(values) {
		row["Row Metadata Mismatch"] = true
		return
	}
	row["Row Metadata Applied"] = true
	row["Column Metadata Relative Byte Range"] = description["Relative Byte Range"]
	for i, v := range values {
		c := cols[i]
		v["Type OID"] = c["Type OID"]
		v["Format Code"] = c["Format Code"]
		v["Column Name"] = bytes.Clone(c["Name"].([]byte))
		if v["NULL"] == true {
			continue
		}
		b := v["Value"].([]byte)
		// int4recv uses pq_getmsgint(...,4), independent of text encoding.
		if c["Type OID"] == uint64(23) && c["Format Code"] == uint64(1) {
			if len(b) == 4 {
				v["Decoded int4"] = int64(int32(binary.BigEndian.Uint32(b)))
			} else {
				v["Value Layout Error"] = "binary int4 requires four bytes"
			}
		}
	}
}
func postgresqlFieldsProfileValid(profile string) bool {
	switch profile {
	case "startup", "ssl-request", "gss-request", "cancel", "ssl-response", "gss-response", "frontend", "backend", "frontend-block", "backend-block", "backend-scram", "backend-scram-block", "password", "sasl-initial", "sasl-response", "sasl-scram-response":
		return true
	}
	return false
}
func decodePostgreSQLFields(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if !postgresqlFieldsProfileValid(profile) {
		return nil, nil, fmt.Errorf("postgresql-fields: unknown explicit profile")
	}
	if len(wire) == 0 || len(wire) > postgresqlFieldsMaxBytes {
		return nil, nil, fmt.Errorf("postgresql-fields: 1..1048576 byte boundary required")
	}
	r := &postgresqlFieldsReader{wire: wire, end: len(wire)}
	info := map[string]any{"Layout Context": profile, "Session State Validated": false, "Payload Decrypted": false, "Query Executed": false}
	switch profile {
	case "startup", "ssl-request", "gss-request", "cancel", "ssl-response", "gss-response":
		r.untyped(info, profile)
	default:
		if strings.HasSuffix(profile, "-block") {
			messages := make([]map[string]any, 0)
			var description map[string]any
			for r.err == nil && r.at < len(wire) {
				if len(messages) == postgresqlFieldsMaxItems {
					r.fail("messages resource limit")
					break
				}
				m := r.typed(profile)
				if m["Message Name"] == "RowDescription" {
					description = m
				} else if m["Message Name"] == "DataRow" && description != nil && r.err == nil {
					postgresqlFieldsAnnotateRow(m, description)
				}
				switch m["Message Name"] {
				case "CommandComplete", "ReadyForQuery", "Authentication", "ErrorResponse", "NoData":
					description = nil
				}
				messages = append(messages, m)
			}
			r.group("Messages", 0, 0, true)
			info["Messages"] = messages
			info["Message Count"] = len(messages)
		} else {
			for k, v := range r.typed(profile) {
				info[k] = v
			}
		}
	}
	if r.at != len(wire) {
		r.fail("bytes outside selected message")
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	return r.fields, info, nil
}
func parsePostgreSQLFields(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	if !postgresqlFieldsProfileValid(profile) {
		return fmt.Errorf("postgresql-fields: unknown explicit profile")
	}
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodePostgreSQLFields(w, profile)
	}, "postgresql-fields")
}
