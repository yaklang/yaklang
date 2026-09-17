package stream_parser

// Passive JRMP transport and Java serialization syntax. No class resolution,
// object instantiation, callbacks from the stream, or remote invocation occurs.
// https://docs.oracle.com/en/java/javase/17/docs/specs/rmi/protocol.html
// https://docs.oracle.com/en/java/javase/17/docs/specs/serialization/protocol.html
// The prose RMI grammar still prints version 1; the actual OpenJDK 17 transport
// constant and original fixtures use version 2:
// https://github.com/openjdk/jdk/blob/jdk-17%2B35/src/java.rmi/share/classes/sun/rmi/transport/TransportConstants.java
import (
	"bytes"
	"fmt"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/yserx"
)

type rmiField struct {
	Name, Type string
	Start, End int
	Children   []rmiField
	List       bool
	Value      any
	Decoded    bool
}
type rmiClassField struct {
	name string
	code byte
}
type rmiClass struct {
	name     string
	flags    byte
	fields   []rmiClassField
	super    *rmiClass
	complete bool
}
type rmiHandle struct {
	kind  string
	class *rmiClass
	text  string
}
type rmiReader struct {
	wire               []byte
	pos, depth, tokens int
	handles            []rmiHandle
}

func (r *rmiReader) fail(s string) error { return fmt.Errorf("rmi: byte %d: %s", r.pos, s) }
func (r *rmiReader) take(out *[]rmiField, name, typ string, n int) ([]byte, error) {
	r.tokens++
	if r.tokens > 32768 {
		return nil, r.fail("decoded field limit")
	}
	if n < 0 || n > len(r.wire)-r.pos {
		return nil, r.fail("truncated " + name)
	}
	start := r.pos
	r.pos += n
	*out = append(*out, rmiField{Name: name, Type: typ, Start: start, End: r.pos})
	return r.wire[start:r.pos], nil
}
func (r *rmiReader) num(out *[]rmiField, name string, n int) (uint64, error) {
	typ := fmt.Sprintf("uint%d", n*8)
	b, e := r.take(out, name, typ, n)
	if e != nil {
		return 0, e
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

// DataInput modified UTF-8 encodes NUL as C0 80 and supplementary characters
// as a UTF-16 surrogate pair, never as a four-byte UTF-8 sequence. Unpaired
// surrogates are retained as code units instead of silently replacing bytes.
func rmiModifiedUTF8(b []byte) (string, []uint16, error) {
	var units []uint16
	for pos := 0; pos < len(b); {
		c := b[pos]
		pos++
		var v uint16
		switch {
		case c > 0 && c < 128:
			v = uint16(c)
		case c&0xe0 == 0xc0:
			if pos >= len(b) || b[pos]&0xc0 != 0x80 {
				return "", nil, fmt.Errorf("invalid modified UTF8 continuation")
			}
			v = uint16(c&31)<<6 | uint16(b[pos]&63)
			pos++
			if v < 128 && v != 0 {
				return "", nil, fmt.Errorf("overlong modified UTF8")
			}
			if v == 0 && c != 0xc0 {
				return "", nil, fmt.Errorf("invalid modified UTF8 NUL")
			}
		case c&0xf0 == 0xe0:
			if len(b)-pos < 2 || b[pos]&0xc0 != 0x80 || b[pos+1]&0xc0 != 0x80 {
				return "", nil, fmt.Errorf("invalid modified UTF8 continuation")
			}
			v = uint16(c&15)<<12 | uint16(b[pos]&63)<<6 | uint16(b[pos+1]&63)
			pos += 2
			if v < 2048 {
				return "", nil, fmt.Errorf("overlong modified UTF8")
			}
		default:
			return "", nil, fmt.Errorf("invalid modified UTF8 lead byte")
		}
		units = append(units, v)
	}
	for i := 0; i < len(units); i++ {
		v := units[i]
		if v >= 0xd800 && v <= 0xdbff {
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", units, nil
			}
			i++
		} else if v >= 0xdc00 && v <= 0xdfff {
			return "", units, nil
		}
	}
	return string(utf16.Decode(units)), nil, nil
}
func (r *rmiReader) utf(out *[]rmiField, name string, long bool) (string, error) {
	width := 2
	if long {
		width = 8
	}
	n, e := r.num(out, name+" Length", width)
	if e != nil {
		return "", e
	}
	if n > uint64(len(r.wire)-r.pos) || n > 1<<20 {
		return "", r.fail("modified UTF8 length exceeds record")
	}
	b, e := r.take(out, name, "string", int(n))
	if e != nil {
		return "", e
	}
	s, units, e := rmiModifiedUTF8(b)
	if e != nil {
		return "", r.fail(e.Error())
	}
	f := &(*out)[len(*out)-1]
	if units != nil {
		f.Type = "raw"
		f.Name = name + " UTF16 Units"
		f.Decoded = true
		f.Value = units
	} else if !bytes.Equal(b, []byte(s)) {
		f.Decoded = true
		f.Value = s
	}
	return s, nil
}
func (r *rmiReader) handle(v rmiHandle) error {
	if len(r.handles) >= 4096 {
		return r.fail("serialization handle limit")
	}
	r.handles = append(r.handles, v)
	return nil
}
func (r *rmiReader) annotation(out *[]rmiField, name string) error {
	f := rmiField{Name: name, Start: r.pos, List: true}
	for {
		if r.pos >= len(r.wire) {
			return r.fail("missing TC_ENDBLOCKDATA")
		}
		if r.wire[r.pos] == 0x78 {
			_, e := r.num(&f.Children, "End Block Data", 1)
			if e != nil {
				return e
			}
			break
		}
		if _, e := r.content(&f.Children, "Annotation Element", "content"); e != nil {
			return e
		}
	}
	f.End = r.pos
	*out = append(*out, f)
	return nil
}
func rmiPrimitiveWidth(code byte) int {
	switch code {
	case 'B', 'Z':
		return 1
	case 'C', 'S':
		return 2
	case 'F', 'I':
		return 4
	case 'D', 'J':
		return 8
	}
	return 0
}
func (r *rmiReader) classData(out *[]rmiField, c *rmiClass) error {
	var chain []*rmiClass
	seen := map[*rmiClass]bool{}
	for p := c; p != nil; p = p.super {
		if !p.complete || seen[p] || len(chain) >= 48 {
			return r.fail("invalid or cyclic class hierarchy")
		}
		seen[p] = true
		chain = append(chain, p)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		p := chain[i]
		r.tokens++
		if r.tokens > 32768 {
			return r.fail("decoded field limit")
		}
		f := rmiField{Name: "Class Data " + p.name, Start: r.pos}
		if p.flags&2 != 0 {
			for _, field := range p.fields {
				width := rmiPrimitiveWidth(field.code)
				if width != 0 {
					b, e := r.take(&f.Children, field.name, "raw", width)
					if e != nil {
						return e
					}
					if field.code == 'Z' && b[0] > 1 {
						return r.fail("invalid serialized boolean")
					}
				} else {
					if _, e := r.content(&f.Children, field.name, "object"); e != nil {
						return e
					}
				}
			}
		}
		if p.flags&4 != 0 && p.flags&8 == 0 {
			return r.fail("externalizable protocol-v1 data has no syntax boundary")
		}
		if p.flags&3 == 3 || p.flags&12 == 12 {
			if e := r.annotation(&f.Children, "Object Annotation"); e != nil {
				return e
			}
		}
		f.End = r.pos
		*out = append(*out, f)
	}
	return nil
}
func (r *rmiReader) content(out *[]rmiField, name, context string) (rmiHandle, error) {
	r.depth++
	defer func() { r.depth-- }()
	r.tokens++
	if r.depth > 48 || r.tokens > 32768 {
		return rmiHandle{}, r.fail("serialization resource limit")
	}
	f := rmiField{Name: name, Start: r.pos}
	v, e := r.num(&f.Children, "Content Type", 1)
	if e != nil {
		return rmiHandle{}, e
	}
	tag := byte(v)
	h := rmiHandle{kind: "object"}
	if context == "class" && tag != 0x70 && tag != 0x71 && tag != 0x72 && tag != 0x7d {
		return h, r.fail("expected class descriptor")
	}
	if context == "string" && tag != 0x71 && tag != 0x74 && tag != 0x7c {
		return h, r.fail("expected serialized String")
	}
	if context == "object" && (tag == 0x77 || tag == 0x7a || tag == 0x78 || tag == 0x79) {
		return h, r.fail("expected serialized object")
	}
	switch tag {
	case 0x70:
		h.kind = "null"
	case 0x71:
		handle, e := r.num(&f.Children, "Handle", 4)
		if e != nil {
			return h, e
		}
		if handle < 0x7e0000 || handle-0x7e0000 >= uint64(len(r.handles)) {
			return h, r.fail("unknown serialization handle")
		}
		h = r.handles[handle-0x7e0000]
		if context == "class" && h.kind != "class" || context == "string" && h.kind != "string" {
			return h, r.fail("serialization handle kind mismatch")
		}
	case 0x74, 0x7c:
		s, e := r.utf(&f.Children, "String", tag == 0x7c)
		if e != nil {
			return h, e
		}
		h = rmiHandle{kind: "string", text: s}
		if e = r.handle(h); e != nil {
			return h, e
		}
	case 0x72, 0x7d:
		c := &rmiClass{}
		h = rmiHandle{kind: "class", class: c}
		if tag == 0x72 {
			c.name, e = r.utf(&f.Children, "Class Name", false)
			if e != nil {
				return h, e
			}
			if c.name == "" {
				return h, r.fail("empty class name")
			}
			if _, e = r.num(&f.Children, "Serial Version UID", 8); e != nil {
				return h, e
			}
		}
		if e = r.handle(h); e != nil {
			return h, e
		}
		if tag == 0x7d {
			c.name = "<dynamic proxy>"
			c.flags = 2
			n, e := r.num(&f.Children, "Interface Count", 4)
			if e != nil {
				return h, e
			}
			if n > 1024 {
				return h, r.fail("interface count exceeds profile")
			}
			interfaces := rmiField{Name: "Interfaces", Start: r.pos, List: true}
			for i := uint64(0); i < n; i++ {
				item := rmiField{Name: fmt.Sprintf("Interface %d", i), Start: r.pos}
				s, e := r.utf(&item.Children, "Interface Name", false)
				if e != nil {
					return h, e
				}
				if s == "" {
					return h, r.fail("empty proxy interface")
				}
				item.End = r.pos
				interfaces.Children = append(interfaces.Children, item)
			}
			interfaces.End = r.pos
			f.Children = append(f.Children, interfaces)
		} else {
			flags, e := r.num(&f.Children, "Class Flags", 1)
			if e != nil {
				return h, e
			}
			c.flags = byte(flags)
			if flags&^uint64(31) != 0 || flags&6 == 6 || flags&1 != 0 && flags&2 == 0 || flags&8 != 0 && flags&4 == 0 {
				return h, r.fail("invalid class flags")
			}
			n, e := r.num(&f.Children, "Field Count", 2)
			if e != nil {
				return h, e
			}
			if n > 1024 {
				return h, r.fail("field count exceeds profile")
			}
			if flags&16 != 0 && (flags != 18 || n != 0) {
				return h, r.fail("invalid enum descriptor")
			}
			fields := rmiField{Name: "Class Fields", Start: r.pos, List: true}
			for i := uint64(0); i < n; i++ {
				item := rmiField{Name: fmt.Sprintf("Field %d", i), Start: r.pos}
				code, e := r.num(&item.Children, "Field Type", 1)
				if e != nil {
					return h, e
				}
				if rmiPrimitiveWidth(byte(code)) == 0 && code != 'L' && code != '[' {
					return h, r.fail("unknown class field type")
				}
				s, e := r.utf(&item.Children, "Field Name", false)
				if e != nil {
					return h, e
				}
				if s == "" {
					return h, r.fail("empty field name")
				}
				if code == 'L' || code == '[' {
					if _, e = r.content(&item.Children, "Field Class Name", "string"); e != nil {
						return h, e
					}
				}
				c.fields = append(c.fields, rmiClassField{s, byte(code)})
				item.End = r.pos
				fields.Children = append(fields.Children, item)
			}
			fields.End = r.pos
			f.Children = append(f.Children, fields)
		}
		if e = r.annotation(&f.Children, "Class Annotation"); e != nil {
			return h, e
		}
		super, e := r.content(&f.Children, "Super Class", "class")
		if e != nil {
			return h, e
		}
		c.super = super.class
		// A reference to a descriptor still under construction would form an
		// ancestor cycle; reject before calling the existing object reader.
		if c.super != nil && !c.super.complete {
			return h, r.fail("cyclic or incomplete superclass")
		}
		c.complete = true
	case 0x73, 0x75, 0x76, 0x7e:
		class, e := r.content(&f.Children, "Class Descriptor", "class")
		if e != nil {
			return h, e
		}
		if class.class == nil || !class.class.complete {
			return h, r.fail("object has no complete class descriptor")
		}
		h.class = class.class
		if e = r.handle(h); e != nil {
			return h, e
		}
		switch tag {
		case 0x73:
			if class.class.flags&16 != 0 {
				return h, r.fail("enum requires TC_ENUM")
			}
			if e = r.classData(&f.Children, class.class); e != nil {
				return h, e
			}
		case 0x75:
			n, e := r.num(&f.Children, "Array Length", 4)
			if e != nil {
				return h, e
			}
			if n > 4096 {
				return h, r.fail("array length exceeds profile")
			}
			cn := class.class.name
			if len(cn) < 2 || cn[0] != '[' {
				return h, r.fail("array descriptor required")
			}
			width := rmiPrimitiveWidth(cn[1])
			if width == 0 && cn[1] != 'L' && cn[1] != '[' {
				return h, r.fail("unsupported array element descriptor")
			}
			array := rmiField{Name: "Array Values", Start: r.pos, List: true}
			for i := uint64(0); i < n; i++ {
				if width != 0 {
					b, e := r.take(&array.Children, fmt.Sprintf("Element %d", i), "raw", width)
					if e != nil {
						return h, e
					}
					if cn[1] == 'Z' && b[0] > 1 {
						return h, r.fail("invalid array boolean")
					}
				} else {
					if _, e = r.content(&array.Children, fmt.Sprintf("Element %d", i), "object"); e != nil {
						return h, e
					}
				}
			}
			array.End = r.pos
			f.Children = append(f.Children, array)
		case 0x7e:
			if class.class.flags&16 == 0 {
				return h, r.fail("TC_ENUM requires enum descriptor")
			}
			if _, e = r.content(&f.Children, "Enum Name", "string"); e != nil {
				return h, e
			}
		}
	case 0x77, 0x7a:
		width := 1
		if tag == 0x7a {
			width = 4
		}
		n, e := r.num(&f.Children, "Block Length", width)
		if e != nil {
			return h, e
		}
		if n > 1024 {
			return h, r.fail("block data exceeds 1024 byte profile")
		}
		if _, e = r.take(&f.Children, "Block Data", "raw", int(n)); e != nil {
			return h, e
		}
		h.kind = "block"
	case 0x79:
		if r.depth != 1 {
			return h, r.fail("TC_RESET inside nested object")
		}
		r.handles = nil
		h.kind = "reset"
	case 0x7b:
		return h, r.fail("TC_EXCEPTION stream reset outside supported profile")
	case 0x78:
		return h, r.fail("unexpected TC_ENDBLOCKDATA")
	default:
		return h, r.fail("unknown Java serialization token")
	}
	f.End = r.pos
	*out = append(*out, f)
	return h, nil
}

func rmiExistingSerializationCheck(wire []byte) (err error) {
	// Called only after the scanner has checked all nested lengths, handles,
	// hierarchy cycles and resource bounds. yserx is a passive Go syntax reader.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("rmi: existing serialization reader: %v", p)
		}
	}()
	r := bytes.NewReader(wire)
	_, err = yserx.ParseJavaSerializedFromReader(r)
	if err != nil {
		return fmt.Errorf("rmi: existing serialization reader: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("rmi: serialization reader left bytes")
	}
	return nil
}

func (r *rmiReader) endpoint(out *[]rmiField, name string) error {
	f := rmiField{Name: name, Start: r.pos}
	if _, e := r.utf(&f.Children, "Host", false); e != nil {
		return e
	}
	port, e := r.num(&f.Children, "Port", 4)
	if e != nil {
		return e
	}
	if port > 65535 {
		return r.fail("endpoint port outside 0..65535")
	}
	f.End = r.pos
	*out = append(*out, f)
	return nil
}
func (r *rmiReader) message(out *[]rmiField) error {
	f := rmiField{Name: "Message", Start: r.pos}
	typ, e := r.num(&f.Children, "Type", 1)
	if e != nil {
		return e
	}
	switch typ {
	case 0x52, 0x53:
	case 0x54:
		for _, v := range []struct {
			n string
			w int
		}{{"UID Unique", 4}, {"UID Time", 8}, {"UID Count", 2}} {
			if _, e = r.num(&f.Children, v.n, v.w); e != nil {
				return e
			}
		}
	case 0x50, 0x51:
		serStart := r.pos
		magic, e := r.num(&f.Children, "Ser Magic", 2)
		if e != nil {
			return e
		}
		version, e := r.num(&f.Children, "Ser Version", 2)
		if e != nil {
			return e
		}
		if magic != 0xaced || version != 5 {
			return r.fail("serialization header must be ACED0005")
		}
		// ObjID/Operation/Hash and ReturnCode/UID are primitive values written
		// through ObjectOutputStream and may span multiple block-data records.
		need := 34
		if typ == 0x51 {
			need = 15
		}
		var body []byte
		var blocks []rmiField
		for len(body) < need {
			block := rmiField{Name: "Primitive Block", Start: r.pos}
			tag, e := r.num(&block.Children, "Content Type", 1)
			if e != nil {
				return e
			}
			if tag != 0x77 && tag != 0x7a {
				return r.fail("call/return header requires block data")
			}
			width := 1
			if tag == 0x7a {
				width = 4
			}
			n, e := r.num(&block.Children, "Block Length", width)
			if e != nil {
				return e
			}
			if n == 0 || n > 1024 {
				return r.fail("invalid primitive block length")
			}
			b, e := r.take(&block.Children, "Primitive Data", "raw", int(n))
			if e != nil {
				return e
			}
			body = append(body, b...)
			block.End = r.pos
			blocks = append(blocks, block)
		}
		if typ == 0x51 && (body[0] != 1 && body[0] != 2) {
			return r.fail("return code must be 1 or 2")
		}
		if typ == 0x51 && body[0] == 2 {
			if len(body) != 15 || r.pos >= len(r.wire) || (r.wire[r.pos] != 0x73 && r.wire[r.pos] != 0x71) {
				return r.fail("exceptional return requires exception object")
			}
		}
		// Replace each raw primitive span with the exact field portions it
		// contains. Split fields retain raw portions; only contiguous fields
		// expose a numeric value, never an invented contiguous wire span.
		defs := []struct {
			name  string
			width int
		}{{"Object Number", 8}, {"UID Unique", 4}, {"UID Time", 8}, {"UID Count", 2}, {"Operation", 4}, {"Method Hash", 8}}
		if typ == 0x51 {
			defs = []struct {
				name  string
				width int
			}{{"Return Code", 1}, {"UID Unique", 4}, {"UID Time", 8}, {"UID Count", 2}}
		}
		starts := []int{0}
		for _, d := range defs {
			starts = append(starts, starts[len(starts)-1]+d.width)
		}
		logical := 0
		for bi := range blocks {
			block := &blocks[bi]
			raw := block.Children[len(block.Children)-1]
			block.Children = block.Children[:len(block.Children)-1]
			left := raw.End - raw.Start
			wirePos := raw.Start
			for left > 0 {
				idx := 0
				for idx < len(defs) && logical >= starts[idx+1] {
					idx++
				}
				size := left
				name, kind := "Uninterpreted Primitive Arguments", "raw"
				if idx < len(defs) {
					name = defs[idx].name
					remain := starts[idx+1] - logical
					if size > remain {
						size = remain
					}
					if logical == starts[idx] && size == defs[idx].width {
						kind = fmt.Sprintf("uint%d", size*8)
						if name == "Operation" {
							kind = "int32"
						}
					} else {
						name += " Part"
					}
				}
				block.Children = append(block.Children, rmiField{Name: name, Type: kind, Start: wirePos, End: wirePos + size})
				logical += size
				wirePos += size
				left -= size
			}
			f.Children = append(f.Children, *block)
		}
		// Without the remote method signature primitive arguments stay raw;
		// all following Java object syntax is still validated and represented.
		r.handles = nil
		r.tokens = 0
		objects := 0
		for r.pos < len(r.wire) {
			if _, e = r.content(&f.Children, "Serialized Value", "content"); e != nil {
				return e
			}
			objects++
		}
		if typ == 0x51 && body[0] == 2 && objects == 0 {
			return r.fail("exceptional return requires exception object")
		}
		if e = rmiExistingSerializationCheck(r.wire[serStart:r.pos]); e != nil {
			return e
		}
	default:
		return r.fail("unknown transport message type")
	}
	f.End = r.pos
	*out = append(*out, f)
	return nil
}

func decodeRMIRecord(wire []byte, mode string) ([]rmiField, map[string]any, error) {
	if len(wire) == 0 || len(wire) > 1<<20 {
		return nil, nil, fmt.Errorf("rmi: explicit record must contain 1..1048576 bytes")
	}
	r := &rmiReader{wire: wire}
	var f []rmiField
	info := map[string]any{"Profile": "JRMP bounded transport and serialization syntax", "Mode": mode, "Classes Instantiated": false, "Method Semantics": "Not interpreted", "Connection State": "Caller supplied phase"}
	switch mode {
	case "header":
		magic, e := r.take(&f, "Magic", "raw", 4)
		if e != nil {
			return nil, nil, e
		}
		if !bytes.Equal(magic, []byte("JRMI")) {
			return nil, nil, r.fail("magic must be JRMI")
		}
		version, e := r.num(&f, "Version", 2)
		if e != nil {
			return nil, nil, e
		}
		if version != 2 {
			return nil, nil, r.fail("transport version must be 2")
		}
		protocol, e := r.num(&f, "Protocol", 1)
		if e != nil {
			return nil, nil, e
		}
		if protocol != 0x4b && protocol != 0x4c && protocol != 0x4d {
			return nil, nil, r.fail("unknown protocol")
		}
		if protocol == 0x4c {
			if e = r.message(&f); e != nil {
				return nil, nil, e
			}
		} else if r.pos != len(wire) {
			return nil, nil, r.fail("transport header has trailing bytes; handshake phase required")
		}
	case "server-handshake":
		ack, e := r.num(&f, "Protocol Ack", 1)
		if e != nil {
			return nil, nil, e
		}
		if ack == 0x4e {
			if e = r.endpoint(&f, "Server Observed Endpoint"); e != nil {
				return nil, nil, e
			}
		} else if ack != 0x4f {
			return nil, nil, r.fail("invalid protocol acknowledgment")
		}
	case "client-endpoint":
		if e := r.endpoint(&f, "Client Endpoint"); e != nil {
			return nil, nil, e
		}
	case "message":
		if e := r.message(&f); e != nil {
			return nil, nil, e
		}
	case "multiplex":
		op, e := r.num(&f, "Multiplex Operation", 1)
		if e != nil {
			return nil, nil, e
		}
		if op < 0xe1 || op > 0xe5 {
			return nil, nil, r.fail("unknown multiplex operation")
		}
		if _, e = r.num(&f, "Connection ID", 2); e != nil {
			return nil, nil, e
		}
		if op == 0xe4 || op == 0xe5 {
			n, e := r.num(&f, "Count", 4)
			if e != nil {
				return nil, nil, e
			}
			if n == 0 || n > 0x7fffffff {
				return nil, nil, r.fail("multiplex count must be positive int32")
			}
			if op == 0xe5 {
				if n > uint64(len(wire)-r.pos) {
					return nil, nil, r.fail("multiplex data exceeds record")
				}
				if _, e = r.take(&f, "Virtual Connection Data", "raw", int(n)); e != nil {
					return nil, nil, e
				}
			}
		}
	default:
		return nil, nil, fmt.Errorf("rmi: unknown explicit phase")
	}
	if r.pos != len(wire) {
		return nil, nil, r.fail("trailing record bytes")
	}
	return f, info, nil
}
