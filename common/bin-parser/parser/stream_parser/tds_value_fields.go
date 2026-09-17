package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type tdsFieldsType struct {
	kind           byte
	maximum, fixed int
	plp, unicode   bool
	info           map[string]any
}

func (r *tdsFieldsReader) typeInfo() tdsFieldsType {
	s := r.at
	t := tdsFieldsType{kind: byte(r.uint("Data Type", 1))}
	m := map[string]any{"Type": t.kind}
	switch t.kind {
	case 0x1f: // NULLTYPE has neither a metadata length nor value bytes.
	case 0x30, 0x32:
		t.fixed = 1
	case 0x34:
		t.fixed = 2
	case 0x38:
		t.fixed = 4
	case 0x7f:
		t.fixed = 8
	case 0x24, 0x26, 0x68, 0x6f:
		t.maximum = int(r.uint("Maximum Length", 1))
		valid := t.kind == 0x24 && t.maximum == 16 || t.kind == 0x26 && (t.maximum == 1 || t.maximum == 2 || t.maximum == 4 || t.maximum == 8) || t.kind == 0x68 && t.maximum == 1 || t.kind == 0x6f && (t.maximum == 4 || t.maximum == 8)
		if !valid {
			r.fail("invalid nullable type maximum")
		}
	case 0xa5, 0xad, 0xa7, 0xaf, 0xe7, 0xef:
		t.maximum = int(r.uint("Maximum Length", 2))
		t.unicode = t.kind == 0xe7 || t.kind == 0xef
		t.plp = t.maximum == 65535
		if t.plp && (!r.version72 || (t.kind != 0xa5 && t.kind != 0xa7 && t.kind != 0xe7)) {
			r.fail("unsupported max-length type/version")
		}
		if !t.plp && (t.maximum > 8000 || t.unicode && t.maximum%2 != 0) {
			r.fail("invalid character/binary maximum")
		}
		if t.kind != 0xa5 && t.kind != 0xad {
			cs := r.at
			bits := r.uint("Collation Info", 4)
			sortID := r.uint("Collation Sort ID", 1)
			c := r.span(cs, r.at)
			c["LCID"], c["Flags"], c["Version"], c["Sort ID"] = bits&0xfffff, (bits>>20)&255, bits>>28, sortID
			c["Code Page Applied"] = false
			m["Collation"] = c
		}
	default:
		r.fail(fmt.Sprintf("unsupported data type 0x%02x", t.kind))
	}
	m["Maximum Length"], m["Fixed Length"], m["PLP"] = t.maximum, t.fixed, t.plp
	m["Logical Byte Range"], m["Wire Byte Ranges"] = [2]int{s, r.at}, r.ranges(s, r.at)
	t.info = m
	return t
}

func (r *tdsFieldsReader) value(t tdsFieldsType) map[string]any {
	if r.values == tdsFieldsMaxItems {
		r.fail("value resource limit")
		return nil
	}
	r.values++
	s := r.at
	m := map[string]any{"Type": t.kind, "Null": false}
	var b []byte
	var dataSpans [][2]int
	if t.plp {
		total := r.uint("PLP Total Length", 8)
		m["PLP Total Length"] = total
		if total == ^uint64(0) {
			m["Null"] = true
		} else {
			if total != ^uint64(1) && total > tdsFieldsMaxBytes {
				r.fail("PLP total exceeds resource limit")
			}
			chunks := 0
			for r.err == nil {
				n := r.uint("PLP Chunk Length", 4)
				if n == 0 {
					break
				}
				if chunks == tdsFieldsMaxItems || n > uint64(r.end-r.at) {
					r.fail("PLP chunk resource limit or boundary")
					break
				}
				chunks++
				at := r.at
				b = append(b, r.take("PLP Chunk Data", "raw", int(n))...)
				dataSpans = append(dataSpans, r.ranges(at, r.at)...)
			}
			if total != ^uint64(1) && uint64(len(b)) != total {
				r.fail("PLP total differs from chunks")
			}
			m["PLP Chunk Count"] = chunks
		}
	} else {
		n := t.fixed
		switch t.kind {
		case 0x1f:
			m["Null"] = true
		case 0x24, 0x26, 0x68, 0x6f:
			n = int(r.uint("Value Length", 1))
			if n == 0 {
				m["Null"] = true
			} else if n != t.maximum {
				r.fail("nullable value length differs from type maximum")
			}
		case 0xa5, 0xad, 0xa7, 0xaf, 0xe7, 0xef:
			n = int(r.uint("Value Length", 2))
			if n == 65535 {
				m["Null"] = true
				n = 0
			} else if n > t.maximum {
				r.fail("value exceeds type maximum")
			}
		}
		at := r.at
		typ := "raw"
		integer := t.kind == 0x26 || t.kind == 0x30 || t.kind == 0x34 || t.kind == 0x38 || t.kind == 0x7f
		if integer && n > 0 {
			if n == 1 {
				typ = "uint8"
			} else {
				typ = fmt.Sprintf("int%d", n*8)
			}
		}
		if t.kind == 0x68 || t.kind == 0x32 {
			if n > 0 {
				typ = "uint8"
			}
		}
		b = r.take("Value", typ, n)
		dataSpans = r.ranges(at, r.at)
	}
	m["Logical Byte Range"], m["Wire Byte Ranges"] = [2]int{s, r.at}, r.ranges(s, r.at)
	m["Data Wire Byte Ranges"], m["Bytes"] = dataSpans, bytes.Clone(b)
	if r.err != nil || m["Null"] == true {
		return m
	}
	if t.unicode {
		if len(b)%2 != 0 {
			r.fail("odd Unicode value length")
		}
		tdsFieldsUnicode(m, b)
	}
	switch t.kind {
	case 0x26, 0x30, 0x34, 0x38, 0x7f:
		var v uint64
		for i := len(b) - 1; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
		if len(b) == 1 {
			m["Unsigned Integer"] = v
		} else if len(b) != 0 {
			shift := uint(64 - len(b)*8)
			m["Signed Integer"] = int64(v<<shift) >> shift
		}
	case 0x24:
		if len(b) == 16 {
			m["GUID"] = fmt.Sprintf("%08x-%04x-%04x-%x-%x", binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint16(b[4:]), binary.LittleEndian.Uint16(b[6:]), b[8:10], b[10:])
		}
	case 0x68, 0x32:
		if len(b) != 1 || b[0] > 1 {
			r.fail("bit value must be 0 or 1")
		} else {
			m["Boolean"] = b[0] == 1
		}
	case 0x6f:
		m["Epoch Date"] = "1900-01-01"
		m["Time Zone Known"] = false
		if len(b) == 4 {
			m["Days"] = int64(binary.LittleEndian.Uint16(b))
			minutes := binary.LittleEndian.Uint16(b[2:])
			m["Minutes"] = minutes
			if minutes >= 1440 {
				r.fail("smalldatetime minutes outside day")
			}
		} else if len(b) == 8 {
			m["Days"] = int64(int32(binary.LittleEndian.Uint32(b)))
			ticks := binary.LittleEndian.Uint32(b[4:])
			m["Ticks At 300 Hz"] = ticks
			if ticks >= 25920000 {
				r.fail("datetime ticks outside day")
			}
		}
	}
	return m
}

func (r *tdsFieldsReader) rpc() []map[string]any {
	var calls []map[string]any
	for r.err == nil && r.at < r.end {
		if len(calls) == tdsFieldsMaxItems {
			r.fail("RPC resource limit")
			break
		}
		start := r.at
		m := map[string]any{}
		n := r.uint("Procedure Name Length Or ID Switch", 2)
		if n == 65535 {
			m["Procedure ID"] = r.uint("Procedure ID", 2)
		} else {
			m["Procedure Name"] = r.unicode("Procedure Name", int(n)*2)
		}
		options := r.uint("RPC Options", 2)
		m["Options"] = options
		// A metadata reuse request never grants permission to consult state
		// outside this bounded message. Each parameter still supplies TYPE_INFO.
		var params []map[string]any
		for r.err == nil && r.at < r.end {
			b := r.wire[r.at]
			if !r.version72 && b == 0x80 || r.version72 && (b == 0xff || b == 0xfe) {
				break
			}
			at := r.at
			p := map[string]any{"Name": r.name("Parameter Name", 1)}
			p["Status"] = r.uint("Parameter Status", 1)
			t := r.typeInfo()
			p["Type Info"] = t.info
			p["Value"] = r.value(t)
			p["Logical Byte Range"], p["Wire Byte Ranges"] = [2]int{at, r.at}, r.ranges(at, r.at)
			params = append(params, p)
		}
		m["Parameters"] = params
		m["Logical Byte Range"], m["Wire Byte Ranges"] = [2]int{start, r.at}, r.ranges(start, r.at)
		calls = append(calls, m)
		if r.err == nil && r.at < r.end {
			m["Batch Separator"] = r.uint("RPC Batch Separator", 1)
			if r.at == r.end {
				r.fail("separator without next RPC")
			}
		}
	}
	if len(calls) == 0 {
		r.fail("missing RPC request")
	}
	return calls
}

func (r *tdsFieldsReader) response() []map[string]any {
	var tokens []map[string]any
	var columns []tdsFieldsType // local to this exact message; never cached
	userWidth, rowWidth := 2, 4
	if r.version72 {
		userWidth, rowWidth = 4, 8
	}
	for r.err == nil && r.at < r.end {
		if len(tokens) == tdsFieldsMaxItems {
			r.fail("response token resource limit")
			break
		}
		s := r.at
		kind := r.uint("Token Type", 1)
		m := map[string]any{"Token Type": kind}
		switch kind {
		case 0x81:
			m["Name"] = "COLMETADATA"
			n := r.uint("Column Count", 2)
			columns = nil
			m["Metadata Omitted"] = n == 65535
			if n == 65535 {
				break
			}
			if n > tdsFieldsMaxItems {
				r.fail("column resource limit")
				break
			}
			var defs []map[string]any
			for i := uint64(0); r.err == nil && i < n; i++ {
				at := r.at
				c := map[string]any{"User Type": r.uint("User Type", userWidth), "Flags": r.uint("Column Flags", 2)}
				t := r.typeInfo()
				c["Type Info"] = t.info
				c["Name"] = r.name("Column Name", 1)
				c["Logical Byte Range"], c["Wire Byte Ranges"] = [2]int{at, r.at}, r.ranges(at, r.at)
				defs = append(defs, c)
				columns = append(columns, t)
			}
			m["Columns"] = defs
		case 0xd1:
			m["Name"] = "ROW"
			if len(columns) == 0 {
				r.fail("ROW requires fresh same-message COLMETADATA")
				break
			}
			var values []map[string]any
			for _, t := range columns {
				if r.err != nil {
					break
				}
				values = append(values, r.value(t))
			}
			m["Values"] = values
		case 0xfd, 0xfe, 0xff:
			m["Name"] = map[uint64]string{0xfd: "DONE", 0xfe: "DONEPROC", 0xff: "DONEINPROC"}[kind]
			status := r.uint("Done Status", 2)
			m["Status"], m["Current Command"] = status, r.uint("Current Command", 2)
			count := r.uint("Done Row Count", rowWidth)
			m["Row Count"] = count
			// The older layout uses LONG, not the 7.2+ ULONGLONG.
			if rowWidth == 4 {
				m["Row Count"] = int64(int32(count))
				if r.err == nil {
					r.fields[len(r.fields)-1].Type = "int32"
				}
			}
			m["Row Count Valid"] = status&0x10 != 0
			columns = nil
		case 0x79:
			m["Name"] = "RETURNSTATUS"
			v := r.uint("Return Status", 4)
			if r.err == nil {
				r.fields[len(r.fields)-1].Type = "int32"
			}
			m["Value"] = int64(int32(v))
		case 0xac:
			m["Name"] = "RETURNVALUE"
			m["Ordinal"] = r.uint("Parameter Ordinal", 2)
			m["Parameter Name"] = r.name("Parameter Name", 1)
			m["Status"] = r.uint("Parameter Status", 1)
			m["User Type"], m["Flags"] = r.uint("User Type", userWidth), r.uint("Column Flags", 2)
			t := r.typeInfo()
			m["Type Info"] = t.info
			m["Value"] = r.value(t)
		default:
			r.fail(fmt.Sprintf("unsupported response token 0x%02x", kind))
		}
		m["Logical Byte Range"], m["Wire Byte Ranges"] = [2]int{s, r.at}, r.ranges(s, r.at)
		tokens = append(tokens, m)
	}
	if len(tokens) == 0 {
		r.fail("missing response token")
	}
	return tokens
}
