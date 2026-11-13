package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func testTarget() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian}
}

func integer(bits uint16) core.ValueType {
	return core.ValueType{Kind: core.TypeInteger, Bits: bits}
}

func pointer() core.ValueType {
	return core.ValueType{Kind: core.TypePointer, Bits: 64}
}

func register(name string, bits uint16) core.Operand {
	return core.Operand{Kind: core.OperandRegister, Type: integer(bits), Name: name}
}

func value(id core.ValueID, typ core.ValueType) core.Operand {
	return core.Operand{Kind: core.OperandValue, Type: typ, Value: id}
}

func literal(data ...byte) core.Operand {
	return core.Operand{Kind: core.OperandConstant, Literal: data}
}

func functionWithBlocks(blocks ...core.Block) core.Function {
	return core.Function{ID: 1, Address: 0x1000, Blocks: blocks}
}

func TestRegisterViewSharesCanonicalVariable(t *testing.T) {
	cases := map[string]string{
		"al": "rax", "ah": "rax", "ax": "rax", "eax": "rax", "rax": "rax",
		"r8b": "r8", "r8w": "r8", "r8d": "r8", "r8": "r8",
		"r15l": "r15", "rsp": "rsp",
	}
	for name, want := range cases {
		if got := registerViewFor(name, 0).canonical; got != want {
			t.Fatalf("registerViewFor(%q) canonical = %q, want %q", name, got, want)
		}
	}
	if view := registerViewFor("eax", 32); view.width != 32 || view.shift != 0 || view.canonicalBits != 64 {
		t.Fatalf("eax view is %+v", view)
	}
	if view := registerViewFor("ah", 8); view.width != 8 || view.shift != 8 {
		t.Fatalf("ah view is %+v", view)
	}
}

func TestRegisterViewMaskMatchesWidth(t *testing.T) {
	if mask := registerViewFor("al", 0).mask(); mask != 0xff {
		t.Fatalf("al mask = %#x", mask)
	}
	if mask := registerViewFor("rax", 0).mask(); mask != ^uint64(0) {
		t.Fatalf("rax mask = %#x", mask)
	}
}

func TestLiteralUint64IsLittleEndian(t *testing.T) {
	if got := literalUint64([]byte{0x2a}); got != 0x2a {
		t.Fatalf("literal = %#x", got)
	}
	if got := literalUint64([]byte{0x34, 0x12}); got != 0x1234 {
		t.Fatalf("literal = %#x", got)
	}
	if got := literalUint64(nil); got != 0 {
		t.Fatalf("empty literal = %#x", got)
	}
}

func TestValueKeyFollowsCopiesAndAddresses(t *testing.T) {
	defs := map[core.ValueID]core.Instruction{
		1: {Kind: core.OpConstant, Operands: []core.Operand{literal(0x10)}},
		2: {Kind: core.OpCopy, Operands: []core.Operand{value(1, integer(64))}},
		3: {Kind: core.OpAdd, Operands: []core.Operand{value(2, integer(64)), value(1, integer(64))}},
	}
	if got := valueKey(defs, 2); got != "const:10" {
		t.Fatalf("copy key = %q", got)
	}
	if got := valueKey(defs, 3); got != "add(const:10,const:10)" {
		t.Fatalf("add key = %q", got)
	}
	if got := valueKey(defs, 99); got != "v99" {
		t.Fatalf("unknown key = %q", got)
	}
}
