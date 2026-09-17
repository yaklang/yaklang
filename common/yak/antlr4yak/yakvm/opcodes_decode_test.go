package yakvm

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func coreWireTable(index, parent int, children ...int) []byte {
	b := protowire.AppendVarint(nil, uint64(index))
	b = protowire.AppendBytes(b, []byte("table"))
	b = protowire.AppendVarint(b, 0)
	b = protowire.AppendVarint(b, uint64(parent))
	b = protowire.AppendVarint(b, uint64(len(children)))
	for _, c := range children {
		b = protowire.AppendVarint(b, uint64(c))
	}
	return protowire.AppendVarint(b, 0)
}

func TestCoreDecodeTruncationAndReuse(t *testing.T) {
	m := NewCodesMarshaller()
	payload, err := m.Marshal(NewSymbolTable(), []*Code{{Opcode: OpPush, Op1: NewAutoValue("hello")}, {Opcode: OpPop}})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(payload); n++ {
		tbl, codes, err := m.Unmarshal(payload[:n])
		if err == nil || tbl != nil || codes != nil || len(m.table) != 0 {
			t.Fatalf("truncation %d accepted or retained state", n)
		}
	}
	if _, _, err := m.Unmarshal(append(append([]byte(nil), payload...), 1)); err == nil {
		t.Fatal("trailing garbage accepted")
	}
	if _, codes, err := m.Unmarshal(payload); err != nil || len(codes) != 2 {
		t.Fatalf("reuse: %v", err)
	}
}

func TestCoreDecodeRejectsMalformedStructure(t *testing.T) {
	duplicate := append(protowire.AppendVarint(nil, 2), coreWireTable(1, 0)...)
	duplicate = append(duplicate, coreWireTable(1, 0)...)
	cycle := append(protowire.AppendVarint(nil, 3), coreWireTable(1, 0)...)
	cycle = append(cycle, coreWireTable(2, 3, 3)...)
	cycle = append(cycle, coreWireTable(3, 2, 2)...)
	for _, input := range [][]byte{nil, {0, 0}, {0x80}, {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1}, protowire.AppendVarint(nil, 1<<63), append(duplicate, 0), append(cycle, 0)} {
		if _, _, err := NewCodesMarshaller().Unmarshal(input); err == nil {
			t.Fatalf("accepted malformed input %x", input)
		}
	}
	for _, code := range []*Code{
		{Opcode: OpcodeFlag(999)}, {Opcode: OpJMP, Unary: 2}, {Opcode: OpCall, Unary: -1},
		{Opcode: OpPush}, {Opcode: OpScope, Unary: 99}, {Opcode: OpDefer, Op1: NewAutoValue(1)},
	} {
		m := NewCodesMarshaller()
		b, err := m.Marshal(NewSymbolTable(), []*Code{code})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := m.Unmarshal(b); err == nil {
			t.Fatalf("accepted invalid code %v", code.Opcode)
		}
	}
}

func TestCoreDecodeBudgets(t *testing.T) {
	m := NewCodesMarshaller()
	codes := []*Code{{Opcode: OpPush, Op1: NewAutoValue(1)}}
	for i := 0; i < 8; i++ {
		codes = []*Code{{Opcode: OpDefer, Op1: NewValue("opcodes", codes, "")}}
	}
	b, err := m.Marshal(NewSymbolTable(), codes)
	if err != nil {
		t.Fatal(err)
	}
	for _, limits := range []DecodeLimits{{MaxBytes: 1}, {MaxObjects: 2}, {MaxDepth: 4}} {
		if _, _, err := m.UnmarshalWithLimits(b, limits); err == nil {
			t.Fatalf("ignored limits: %+v", limits)
		}
	}
	if _, _, err := m.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
}

func FuzzCoreDecode(f *testing.F) {
	b, err := NewCodesMarshaller().Marshal(NewSymbolTable(), []*Code{{Opcode: OpPush, Op1: NewAutoValue(1)}, {Opcode: OpPop}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Add([]byte{0xff})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 65536 {
			return
		}
		_, _, _ = NewCodesMarshaller().UnmarshalWithLimits(input, DecodeLimits{MaxBytes: 65536, MaxObjects: 4096, MaxDepth: 32})
	})
}
