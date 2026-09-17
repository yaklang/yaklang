package yakvm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

// DecodeLimits bound allocation and recursion before accepting wire counts.
// Zero fields select the defaults. Limits apply to one complete payload.
type DecodeLimits struct{ MaxBytes, MaxObjects, MaxDepth int }

type codeDecoder struct {
	buf                        []byte
	err                        error
	remaining, depth, maxDepth int
	source                     string
	tables                     map[int]*SymbolTable
}

func (d *codeDecoder) fail(format string, args ...any) {
	if d.err == nil {
		d.err = fmt.Errorf("invalid Yak bytecode: "+format, args...)
	}
}
func (d *codeDecoder) uint() uint64 {
	if d.err != nil {
		return 0
	}
	u, n := protowire.ConsumeVarint(d.buf)
	if n <= 0 {
		d.fail("varint: %v", protowire.ParseError(n))
		return 0
	}
	d.buf = d.buf[n:]
	return u
}
func (d *codeDecoder) integer() int {
	u := d.uint()
	if u > uint64(^uint(0)>>1) {
		d.fail("integer exceeds host range")
		return 0
	}
	return int(u)
}
func (d *codeDecoder) signed() int {
	i := int64(d.uint())
	if int64(int(i)) != i {
		d.fail("signed integer exceeds host range")
		return 0
	}
	return int(i)
}
func (d *codeDecoder) flag() bool {
	u := d.uint()
	if u > 1 {
		d.fail("invalid boolean flag %d", u)
	}
	return u == 1
}
func (d *codeDecoder) bytes() []byte {
	if d.err != nil {
		return nil
	}
	b, n := protowire.ConsumeBytes(d.buf)
	if n <= 0 {
		d.fail("bytes: %v", protowire.ParseError(n))
		return nil
	}
	d.buf = d.buf[n:]
	return b
}
func (d *codeDecoder) count() int {
	n := d.integer()
	// Every encoded item requires at least one remaining byte. The aggregate
	// object budget prevents nested count fields from multiplying allocation.
	if d.err != nil {
		return 0
	}
	if n > len(d.buf) || n > d.remaining {
		d.fail("count %d exceeds decode budget", n)
		return 0
	}
	d.remaining -= n
	return n
}

func (c *CodesMarshaller) UnmarshalWithLimits(buf []byte, limits DecodeLimits) (*SymbolTable, []*Code, error) {
	c.table = nil
	if limits.MaxBytes == 0 {
		limits.MaxBytes = 64 << 20
	}
	if limits.MaxObjects == 0 {
		limits.MaxObjects = 1 << 20
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = 128
	}
	if limits.MaxBytes < 0 || limits.MaxObjects < 0 || limits.MaxDepth < 0 || len(buf) > limits.MaxBytes {
		return nil, nil, fmt.Errorf("invalid Yak bytecode: decode limit exceeded")
	}
	d := &codeDecoder{buf: buf, remaining: limits.MaxObjects, maxDepth: limits.MaxDepth, source: c.sourceCode}
	root := d.symbolTables()
	codes := d.codes()
	if d.err == nil && len(d.buf) != 0 {
		d.fail("trailing bytes")
	}
	if d.err != nil {
		return nil, nil, d.err
	}
	c.table = d.tables
	return root, codes, nil
}

func (d *codeDecoder) symbolTables() *SymbolTable {
	n := d.count()
	if n == 0 {
		d.fail("missing root symbol table")
		return nil
	}
	d.tables = make(map[int]*SymbolTable, n)
	descriptions := make(map[int]*SymbolTableDesc, n)
	maxIndex := 0
	for i := 0; i < n && d.err == nil; i++ {
		t := &SymbolTableDesc{Index: d.integer(), Verbose: string(d.bytes()), currentSymbolIndex: d.integer(), Parent: d.integer()}
		if t.Index <= 0 || descriptions[t.Index] != nil {
			d.fail("duplicate or invalid table index %d", t.Index)
			break
		}
		t.Children = make([]int, d.count())
		for j := range t.Children {
			t.Children[j] = d.integer()
		}
		count := d.count()
		t.NameToId = make(map[string]int, count)
		for j := 0; j < count && d.err == nil; j++ {
			name, id := string(d.bytes()), d.integer()
			if _, exists := t.NameToId[name]; exists || id <= 0 {
				d.fail("duplicate or invalid symbol %q", name)
				break
			}
			t.NameToId[name] = id
		}
		descriptions[t.Index] = t
		d.tables[t.Index] = &SymbolTable{index: t.Index, Verbose: t.Verbose, currentSymbolIndex: t.currentSymbolIndex, symbolToId: t.NameToId, InitedId: make(map[int]struct{})}
		maxIndex = max(maxIndex, t.Index)
	}
	if d.err != nil {
		return nil
	}
	root := d.tables[1]
	if root == nil || descriptions[1].Parent != 0 {
		d.fail("invalid root symbol table")
		return nil
	}
	for id, desc := range descriptions {
		t := d.tables[id]
		if id != 1 && (desc.Parent == 0 || d.tables[desc.Parent] == nil) {
			d.fail("missing parent for table %d", id)
			return nil
		}
		t.parent = d.tables[desc.Parent]
		for _, child := range desc.Children {
			if descriptions[child] == nil || descriptions[child].Parent != id {
				d.fail("inconsistent child %d of table %d", child, id)
				return nil
			}
			t.children = append(t.children, d.tables[child])
		}
	}
	// Iterative traversal rejects cycles, duplicate edges, and disconnected
	// components without recursion through attacker-controlled table links.
	seen := map[int]bool{1: true}
	queue := []*SymbolTable{root}
	for i := 0; i < len(queue); i++ {
		for _, child := range queue[i].children {
			if seen[child.index] {
				d.fail("symbol table cycle or duplicate child")
				return nil
			}
			seen[child.index] = true
			queue = append(queue, child)
		}
	}
	if len(seen) != n {
		d.fail("disconnected symbol tables")
		return nil
	}
	root.tableCount, root.idToSymbolTable = maxIndex, d.tables
	return root
}

func (d *codeDecoder) codes() []*Code {
	if d.err != nil {
		return nil
	}
	d.depth++
	defer func() { d.depth-- }()
	if d.depth > d.maxDepth {
		d.fail("nested code depth exceeded")
		return nil
	}
	codes := make([]*Code, d.count())
	for i := range codes {
		if d.err != nil {
			return nil
		}
		code := &Code{Opcode: OpcodeFlag(d.integer()), Unary: d.signed()}
		if d.flag() {
			code.Op1 = d.value()
		}
		if d.flag() {
			code.Op2 = d.value()
		}
		if d.flag() {
			code.StartLineNumber, code.StartColumnNumber = d.integer(), d.integer()
			code.EndLineNumber, code.EndColumnNumber = d.integer(), d.integer()
		}
		codes[i] = code
	}
	if d.err == nil {
		d.validateCodes(codes)
	}
	return codes
}

func (d *codeDecoder) value() *Value {
	v := &Value{TypeVerbose: string(d.bytes()), Literal: string(d.bytes())}
	switch tag := d.uint(); tag {
	case 0:
		kind := d.integer()
		if kind == 0 {
			return v
		}
		types := map[int]reflect.Type{
			int(reflect.Int): literalReflectType_Int, int(reflect.Int8): literalReflectType_Int8,
			int(reflect.Int16): literalReflectType_Int16, int(reflect.Int32): literalReflectType_Int32, int(reflect.Int64): literalReflectType_Int64,
			int(reflect.Uint): literalReflectType_Uint, int(reflect.Uint8): literalReflectType_Uint8,
			int(reflect.Uint16): literalReflectType_Uint16, int(reflect.Uint32): literalReflectType_Uint32, int(reflect.Uint64): literalReflectType_Uint64,
			int(reflect.Float32): literalReflectType_Float32, int(reflect.Float64): literalReflectType_Float64,
			int(reflect.String): literalReflectType_String, int(reflect.Bool): literalReflectType_Bool,
			int(reflect.Map): reflect.TypeOf(map[string]interface{}{}), 27: literalReflectType_Bytes,
		}
		typ := types[kind]
		if typ == nil {
			d.fail("unsupported literal kind %d", kind)
			return v
		}
		data := d.bytes()
		if d.err != nil {
			return v
		}
		if typ == literalReflectType_String {
			s, err := strconv.Unquote(string(data))
			if err != nil {
				d.fail("string: %v", err)
			}
			v.Value = s
		} else {
			x := reflect.New(typ)
			if err := json.Unmarshal(data, x.Interface()); err != nil {
				d.fail("literal: %v", err)
			}
			v.Value = x.Elem().Interface()
		}
	case 1:
		f := &Function{sourceCode: d.source, name: string(d.bytes()), id: d.integer(), isVariableParameter: d.flag()}
		f.symbolTable = d.tables[d.integer()]
		if f.symbolTable == nil {
			d.fail("function has invalid symbol table")
			return v
		}
		f.paramSymbols = make([]int, d.count())
		for i := range f.paramSymbols {
			f.paramSymbols[i] = d.integer()
			if f.paramSymbols[i] <= 0 {
				d.fail("invalid parameter symbol")
			}
		}
		if f.isVariableParameter && len(f.paramSymbols) == 0 {
			d.fail("variadic function has no parameter")
		}
		f.codes = d.codes()
		v.Value = f
	case 2:
		v.Value = d.codes()
	default:
		d.fail("unsupported value tag %d", tag)
	}
	return v
}

// validateCodes checks structural operands and control targets. Decoding is
// not a sandbox: native capabilities and runtime execution budgets are separate.
func (d *codeDecoder) validateCodes(codes []*Code) {
	for i, c := range codes {
		if _, ok := OpcodeVerboseName[c.Opcode]; !ok {
			d.fail("unknown opcode %d at %d", c.Opcode, i)
			return
		}
		if c.Opcode.IsJmp() && c.Opcode != OpRangeNext && c.Opcode != OpInNext && (c.Unary < 0 || c.Unary > len(codes)) {
			d.fail("invalid jump at %d", i)
			return
		}
		switch c.Opcode {
		case OpPush, OpPushfuzz, OpPushId, OpType, OpDefer:
			if c.Op1 == nil {
				d.fail("missing operand at %d", i)
				return
			}
		case OpCatchError, OpBreak, OpContinue:
			if c.Op1 == nil || !c.Op1.IsInt() {
				d.fail("missing integer operand at %d", i)
				return
			}
		}
		switch c.Opcode {
		case OpCall, OpAsyncCall, OpVariadicCall, OpList, OpNewSlice, OpNewSliceWithType, OpNewMap, OpNewMapWithType, OpMake, OpRangeNext, OpInNext, OpIterableCall, OpAssert, OpEllipsis:
			if c.Unary < 0 || c.Unary > 1<<20 {
				d.fail("invalid operand count at %d", i)
				return
			}
		}
		switch c.Opcode {
		case OpRangeNext, OpInNext, OpCatchError:
			if c.Op1 == nil || !c.Op1.IsInt() || c.Op1.Int() < 0 || c.Op1.Int() > len(codes) {
				d.fail("invalid control target at %d", i)
				return
			}
		case OpScope:
			if d.tables[c.Unary] == nil {
				d.fail("invalid scope at %d", i)
				return
			}
		case OpDefer:
			if _, ok := c.Op1.Value.([]*Code); !ok {
				d.fail("invalid defer body at %d", i)
				return
			}
		}
	}
}
