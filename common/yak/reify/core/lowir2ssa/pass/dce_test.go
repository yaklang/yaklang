package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestDropDeadFlagsPreservesLiveConditionsAndRemovesOverwritten(t *testing.T) {
	facts := core.ArchFacts{
		Registers: core.RegisterModel{
			Naming:     core.RegisterNamingX86,
			Width:      64,
			Flags:      "rflags",
			FlagsWidth: 64,
		},
	}
	// Block 1 has two arithmetic flag writes: the first is overwritten before any read
	// (so both its write_register %rflags and intrinsic @flags.add must be removed),
	// the second feeds a cond.ge in block 2 (so it must be preserved).
	fn := &core.Function{
		ID: 1,
		Blocks: []core.Block{
			{
				ID: 1,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{1}}}, Results: []core.Value{{ID: 1, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}},
					{Kind: core.OpConstant, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{2}}}, Results: []core.Value{{ID: 2, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}},
					{Kind: core.OpAdd, Operands: []core.Operand{{Kind: core.OperandValue, Value: 1}, {Kind: core.OperandValue, Value: 2}}, Results: []core.Value{{ID: 3, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}},
					{Kind: core.OpIntrinsic, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "flags.add"}, {Kind: core.OperandValue, Value: 1}, {Kind: core.OperandValue, Value: 2}, {Kind: core.OperandValue, Value: 3}}, Results: []core.Value{{ID: 4, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{{Kind: core.OperandRegister, Name: "rflags"}, {Kind: core.OperandValue, Value: 4}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpIntrinsic, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "flags.cmp"}, {Kind: core.OperandValue, Value: 3}, {Kind: core.OperandValue, Value: 2}}, Results: []core.Value{{ID: 5, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{{Kind: core.OperandRegister, Name: "rflags"}, {Kind: core.OperandValue, Value: 5}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpJump, Targets: []core.BlockID{2}},
				},
			},
			{
				ID: 2,
				Instructions: []core.Instruction{
					{Kind: core.OpReadRegister, Operands: []core.Operand{{Kind: core.OperandRegister, Name: "rflags"}}, Results: []core.Value{{ID: 6, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}, Effects: core.EffectReadRegister},
					{Kind: core.OpIntrinsic, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "cond.ge"}, {Kind: core.OperandValue, Value: 6}}, Results: []core.Value{{ID: 7, Type: core.ValueType{Kind: core.TypeInteger, Bits: 1}}}, Effects: core.EffectReadRegister},
					{Kind: core.OpBranch, Operands: []core.Operand{{Kind: core.OperandValue, Value: 7}}, Targets: []core.BlockID{3, 3}},
				},
			},
			{
				ID: 3,
				Instructions: []core.Instruction{
					// Another trailing flag update right before return (dead across exit).
					{Kind: core.OpIntrinsic, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "flags.sub"}, {Kind: core.OperandValue, Value: 3}, {Kind: core.OperandValue, Value: 1}}, Results: []core.Value{{ID: 8, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{{Kind: core.OperandRegister, Name: "rflags"}, {Kind: core.OperandValue, Value: 8}}, Effects: core.EffectWriteRegister},
					{Kind: core.OpReturn, Operands: []core.Operand{{Kind: core.OperandValue, Value: 3}}},
				},
			},
		},
	}

	removed := DropDeadFlags(fn, facts)
	if removed != 4 {
		t.Fatalf("removed = %d, want 4", removed)
	}
	// Live flags.cmp + write_register must remain in block 1.
	hasCmp := false
	for _, inst := range fn.Blocks[0].Instructions {
		if inst.Kind == core.OpIntrinsic && len(inst.Operands) > 0 && inst.Operands[0].Name == "flags.cmp" {
			hasCmp = true
		}
		if inst.Kind == core.OpIntrinsic && len(inst.Operands) > 0 && inst.Operands[0].Name == "flags.add" {
			t.Fatal("dead flags.add survived")
		}
	}
	if !hasCmp {
		t.Fatal("live flags.cmp was dropped")
	}
	if len(fn.Blocks[2].Instructions) != 1 || fn.Blocks[2].Instructions[0].Kind != core.OpReturn {
		t.Fatalf("dead exit flags survived in block 3: %+v", fn.Blocks[2].Instructions)
	}
}
