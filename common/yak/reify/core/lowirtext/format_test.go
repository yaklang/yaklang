package lowirtext

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestFormat(t *testing.T) {
	ir := &core.LowIR{
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Functions: []core.Function{{
			ID:      1,
			Address: 0x1000,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x1000,
				Instructions: []core.Instruction{
					{
						Kind:     core.OpConstant,
						Results:  []core.Value{{ID: 1, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}},
						Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{0x10}}},
						Address:  0x1000,
					},
					{
						Kind:     core.OpReadRegister,
						Results:  []core.Value{{ID: 2, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}},
						Operands: []core.Operand{{Kind: core.OperandRegister, Name: "rax"}},
						Effects:  core.EffectReadRegister,
						Address:  0x1001,
					},
					{
						Kind:     core.OpAdd,
						Results:  []core.Value{{ID: 3, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}},
						Operands: []core.Operand{{Kind: core.OperandValue, Value: 1}, {Kind: core.OperandValue, Value: 2}},
						Address:  0x1002,
					},
					{
						Kind:    core.OpJump,
						Targets: []core.BlockID{2},
						Address: 0x1003,
					},
				},
			}},
		}},
	}
	text := Format(ir)
	for _, fragment := range []string{
		"target: linux/amd64 (64-bit, little-endian)",
		"function f1 @ 0x1000",
		"block b1 @ 0x1000",
		"v1:i64 = constant 0x10",
		"v2:i64 = read_register %rax [read_register]",
		"v3:i64 = add v1, v2",
		"jump -> b2",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("LowIR text lacks %q:\n%s", fragment, text)
		}
	}
	if Format(nil) != "" {
		t.Fatal("nil LowIR produced text")
	}
}

func TestFormatEffectsPreservesUnknownBits(t *testing.T) {
	got := formatEffects(core.EffectReadMemory | core.Effect(1<<20))
	if !strings.Contains(got, "read_memory") || !strings.Contains(got, "0x100000") {
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
