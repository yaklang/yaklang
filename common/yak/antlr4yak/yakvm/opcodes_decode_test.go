package yakvm

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func checkCoreScalarLiteral(t *testing.T, input []byte, kind uint8) {
	t.Helper()
	// Independent reference types: validate the new fast path against JSON's
	// existing acceptance, range, null and exact destination-type behavior.
	zeroes := []any{int(0), int8(0), int16(0), int32(0), int64(0), uint(0), uint8(0), uint16(0), uint32(0), uint64(0), false, float32(0), float64(0)}
	typ := reflect.TypeOf(zeroes[int(kind)%len(zeroes)])
	want := reflect.New(typ)
	wantErr := json.Unmarshal(input, want.Interface())
	buf := protowire.AppendBytes(nil, []byte("literal"))
	buf = protowire.AppendBytes(buf, nil)
	buf = protowire.AppendVarint(buf, 0)
	buf = protowire.AppendVarint(buf, uint64(typ.Kind()))
	buf = protowire.AppendBytes(buf, input)
	d := &codeDecoder{buf: buf}
	got := d.value()
	if (wantErr == nil) != (d.err == nil) {
		t.Fatalf("%s %q: JSON error %v, decoder error %v", typ, input, wantErr, d.err)
	}
	if wantErr == nil && !reflect.DeepEqual(want.Elem().Interface(), got.Value) {
		t.Fatalf("%s %q: want %T(%v), got %T(%v)", typ, input, want.Elem().Interface(), want.Elem().Interface(), got.Value, got.Value)
	}
	if wantErr == nil && (typ.Kind() == reflect.Float32 || typ.Kind() == reflect.Float64) && math.Signbit(want.Elem().Float()) != math.Signbit(reflect.ValueOf(got.Value).Float()) {
		t.Fatalf("%s %q: changed floating-point sign", typ, input)
	}
}

func TestCoreDecodeScalarJSONCompatibility(t *testing.T) {
	for _, input := range []string{"0", "-0", "+0", "00", "01", "-01", "1", "-1", "127", "128", "255", "256", "32768", "2147483648", "9223372036854775807", "9223372036854775808", "-9223372036854775808", "-9223372036854775809", "18446744073709551615", "18446744073709551616", "1e0", "1.0", "-0.0", "1e-5000", "1e5000", "3.4028234663852886e+38", "3.4028236e38", "1.7976931348623157e308", "0xff", "0x1p2", "1_0", " 1\n", "null", "true", "false", "\"1\"", "NaN", "Inf", "", "-", "1 2"} {
		for kind := uint8(0); kind < 13; kind++ {
			checkCoreScalarLiteral(t, []byte(input), kind)
		}
	}
}

func FuzzCoreDecodeScalar(f *testing.F) {
	for _, input := range []string{"0", "-1", "01", "+1", "null", " 1 ", "18446744073709551615", "-9223372036854775808", "true", "-0.0", "1e5000", "3.4028236e38", "NaN", "0x1p2"} {
		for kind := uint8(0); kind < 13; kind++ {
			f.Add([]byte(input), kind)
		}
	}
	f.Fuzz(func(t *testing.T, input []byte, kind uint8) {
		if len(input) <= 4096 {
			checkCoreScalarLiteral(t, input, kind)
		}
	})
}

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
		{Opcode: OpPush}, {Opcode: OpPushfuzz}, {Opcode: OpPushId}, {Opcode: OpType},
		{Opcode: OpScope, Unary: 99}, {Opcode: OpDefer, Op1: NewAutoValue(1)},
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
