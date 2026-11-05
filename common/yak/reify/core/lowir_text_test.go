package core

import (
	"strings"
	"testing"
)

func TestFormatLowIR(t *testing.T) {
	ir := &LowIR{
		Target: Target{OS: OSLinux, Architecture: ArchAMD64, Bits: 64, ByteOrder: LittleEndian},
		Functions: []Function{{
			ID:      1,
			Address: 0x1000,
			Blocks: []Block{{
				ID:      1,
				Address: 0x1000,
				Instructions: []Instruction{
					{
						Kind:     OpConstant,
						Results:  []Value{{ID: 1, Type: ValueType{Kind: TypeInteger, Bits: 64}}},
						Operands: []Operand{{Kind: OperandConstant, Literal: []byte{0x10}}},
						Address:  0x1000,
					},
					{
						Kind:     OpReadRegister,
						Results:  []Value{{ID: 2, Type: ValueType{Kind: TypeInteger, Bits: 64}}},
						Operands: []Operand{{Kind: OperandRegister, Name: "rax"}},
						Effects:  EffectReadRegister,
						Address:  0x1001,
					},
					{
						Kind:     OpAdd,
						Results:  []Value{{ID: 3, Type: ValueType{Kind: TypeInteger, Bits: 64}}},
						Operands: []Operand{{Kind: OperandValue, Value: 1}, {Kind: OperandValue, Value: 2}},
						Address:  0x1002,
					},
					{
						Kind:    OpJump,
						Targets: []BlockID{2},
						Address: 0x1003,
					},
				},
			}},
		}},
	}
	text := FormatLowIR(ir)
	for _, fragment := range []string{
		"target: linux/amd64 (64-bit, little-endian)",
		"function f1 @ 0x1000",
		"block b1 @ 0x1000",
		"v1:i64 = constant 0x10",
		"v2:i64 = read-register %rax [read-register]",
		"v3:i64 = add v1, v2",
		"jump -> b2",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("LowIR text lacks %q:\n%s", fragment, text)
		}
	}
	if FormatLowIR(nil) != "" {
		t.Fatal("nil LowIR produced text")
	}
}

func TestFormatEffectsPreservesUnknownBits(t *testing.T) {
	got := formatEffects(EffectReadMemory | Effect(1<<20))
	if !strings.Contains(got, "read-memory") || !strings.Contains(got, "0x100000") {
		t.Fatalf("effects = %q", got)
	}
}

func TestFormatLiteralWide(t *testing.T) {
	literal := make([]byte, 16)
	literal[15] = 0x80
	if got := formatLiteral(literal); got != "0x00000000000000000000000000000080" {
		t.Fatalf("wide literal = %q", got)
	}
}
