package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func canaryIR() *core.LowIR {
	return &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x7000,
			Blocks: []core.Block{
				{
					ID:      1,
					Address: 0x7000,
					Instructions: []core.Instruction{
						{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x28)}, Address: 0x7000},
						{Kind: core.OpCopy, Results: []core.Value{{ID: 2, Type: pointer()}}, Operands: []core.Operand{value(1, integer(64))}, Address: 0x7000},
						{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.segment.fs"}, value(2, pointer())}, Effects: core.EffectReadRegister | core.EffectUnknown, Address: 0x7000},
						{Kind: core.OpLoad, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{value(3, integer(64))}, Effects: core.EffectReadMemory, Address: 0x7000},
						{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsi", 64), value(4, integer(64))}, Effects: core.EffectWriteRegister, Address: 0x7000},
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 6, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Effects: core.EffectReadRegister, Address: 0x7009},
						{Kind: core.OpCopy, Results: []core.Value{{ID: 7, Type: pointer()}}, Operands: []core.Operand{value(6, integer(64))}, Address: 0x7009},
						{Kind: core.OpConstant, Results: []core.Value{{ID: 8, Type: integer(64)}}, Operands: []core.Operand{literal(0xf8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)}, Address: 0x7009},
						{Kind: core.OpAdd, Results: []core.Value{{ID: 9, Type: pointer()}}, Operands: []core.Operand{value(7, pointer()), value(8, integer(64))}, Address: 0x7009},
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 10, Type: integer(64)}}, Operands: []core.Operand{register("rsi", 64)}, Effects: core.EffectReadRegister, Address: 0x7009},
						{Kind: core.OpStore, Operands: []core.Operand{value(9, pointer()), value(10, integer(64))}, Effects: core.EffectWriteMemory, Address: 0x7009},
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 12, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Effects: core.EffectReadRegister, Address: 0x7010},
						{Kind: core.OpCopy, Results: []core.Value{{ID: 13, Type: pointer()}}, Operands: []core.Operand{value(12, integer(64))}, Address: 0x7010},
						{Kind: core.OpConstant, Results: []core.Value{{ID: 14, Type: integer(64)}}, Operands: []core.Operand{literal(0xf8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)}, Address: 0x7010},
						{Kind: core.OpAdd, Results: []core.Value{{ID: 15, Type: pointer()}}, Operands: []core.Operand{value(13, pointer()), value(14, integer(64))}, Address: 0x7010},
						{Kind: core.OpLoad, Results: []core.Value{{ID: 16, Type: integer(64)}}, Operands: []core.Operand{value(15, pointer())}, Effects: core.EffectReadMemory, Address: 0x7010},
						{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(16, integer(64))}, Effects: core.EffectWriteRegister, Address: 0x7010},
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 18, Type: integer(64)}}, Operands: []core.Operand{register("rax", 64)}, Effects: core.EffectReadRegister, Address: 0x7014},
						{Kind: core.OpConstant, Results: []core.Value{{ID: 19, Type: integer(64)}}, Operands: []core.Operand{literal(0x28)}, Address: 0x7014},
						{Kind: core.OpCopy, Results: []core.Value{{ID: 20, Type: pointer()}}, Operands: []core.Operand{value(19, integer(64))}, Address: 0x7014},
						{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 21, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.segment.fs"}, value(20, pointer())}, Effects: core.EffectReadRegister | core.EffectUnknown, Address: 0x7014},
						{Kind: core.OpLoad, Results: []core.Value{{ID: 22, Type: integer(64)}}, Operands: []core.Operand{value(21, integer(64))}, Effects: core.EffectReadMemory, Address: 0x7014},
						{Kind: core.OpSub, Results: []core.Value{{ID: 23, Type: integer(64)}}, Operands: []core.Operand{value(18, integer(64)), value(22, integer(64))}, Address: 0x7014},
						{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(23, integer(64))}, Effects: core.EffectWriteRegister, Address: 0x7014},
						{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 25, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.flags.sub"}, value(18, integer(64)), value(22, integer(64)), value(23, integer(64))}, Effects: core.EffectWriteRegister, Address: 0x7014},
						{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rflags", 64), value(25, integer(64))}, Effects: core.EffectWriteRegister, Address: 0x7014},
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 27, Type: integer(64)}}, Operands: []core.Operand{register("rflags", 64)}, Effects: core.EffectReadRegister, Address: 0x701a},
						{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 28, Type: integer(1)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.cond.e"}, value(27, integer(64))}, Effects: core.EffectReadRegister, Address: 0x701a},
						{Kind: core.OpBranch, Operands: []core.Operand{value(28, integer(1))}, Targets: []core.BlockID{3, 2}, Address: 0x701a},
					},
				},
				{
					ID:      2,
					Address: 0x701c,
					Instructions: []core.Instruction{
						{Kind: core.OpConstant, Results: []core.Value{{ID: 30, Type: pointer()}}, Operands: []core.Operand{literal(0xf8, 0x9e, 0, 0)}, Address: 0x701c},
						{Kind: core.OpLoad, Results: []core.Value{{ID: 31, Type: integer(64)}}, Operands: []core.Operand{value(30, pointer())}, Effects: core.EffectReadMemory, Address: 0x701c},
						{Kind: core.OpCopy, Results: []core.Value{{ID: 32, Type: pointer()}}, Operands: []core.Operand{value(31, integer(64))}, Address: 0x701c},
						{Kind: core.OpCall, Operands: []core.Operand{value(32, pointer())}, Effects: core.EffectCall | core.EffectMayTrap | core.EffectUnknown, Address: 0x701c},
						{Kind: core.OpJump, Targets: []core.BlockID{3}, Address: 0x7022},
					},
				},
				{ID: 3, Address: 0x7022, Instructions: []core.Instruction{{Kind: core.OpReturn, Effects: core.EffectMayTrap | core.EffectUnknown, Address: 0x7022}}},
			},
		}},
	}
}

func TestStackCanaryRemovedByDefault(t *testing.T) {
	function := canaryIR().Functions[0]
	removed := RemoveStackCanary(&function)
	if removed == 0 {
		t.Fatal("stack canary pass removed nothing")
	}
	if len(function.Blocks) != 2 {
		t.Fatalf("failure block survived: %d blocks", len(function.Blocks))
	}
	result, err := Convert(context.Background(), canaryIR())
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, forbidden := range []string{"x86.segment.fs", "x86.cond.e", "reify.call"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("canary code survived (%q):\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "jump") {
		t.Fatalf("check branch was not redirected:\n%s", text)
	}
}

func TestStackCanaryKeptWhenRequested(t *testing.T) {
	result, err := Convert(context.Background(), canaryIR(), WithKeepStackCanary())
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	if !strings.Contains(text, "x86.segment.fs") {
		t.Fatalf("stack canary was removed despite the option:\n%s", text)
	}
	if !strings.Contains(text, "x86.cond.e") {
		t.Fatalf("canary check was removed despite the option:\n%s", text)
	}
}

func TestStackCanaryLeavesUnrelatedFunctionsUntouched(t *testing.T) {
	function := testIR().Functions[0]
	before := len(function.Blocks)
	if removed := RemoveStackCanary(&function); removed != 0 {
		t.Fatalf("unrelated function changed: %d", removed)
	}
	if len(function.Blocks) != before {
		t.Fatal("unrelated function lost blocks")
	}
}
