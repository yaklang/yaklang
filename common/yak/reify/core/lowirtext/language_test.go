package lowirtext

import (
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func TestLanguageNamesAndFunctionFactsRoundTrip(t *testing.T) {
	name := "<alloc::vec::Vec<fn(u8, u8) -> bool, alloc::alloc::Global> as core::fmt::Debug>::fmt"
	facts := core.ArchFacts{
		Frame:     core.FrameModel{StackPointer: "rsp", FramePointer: "rbp", CalleeSaved: []string{"rbp"}, StackGrowth: -1},
		Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Width: 64, Arguments: []string{"rax", "rbx"}, Return: "rax"},
	}
	ir := &core.LowIR{Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Registers: core.RegisterModel{Arguments: []string{"rdi", "rsi"}},
		Functions: []core.Function{{ID: 1, Name: name, Address: 0x1000, Facts: &facts,
			Blocks: []core.Block{{ID: 1, Address: 0x1000, Instructions: []core.Instruction{{Kind: core.OpCall, Address: 0x1000,
				Operands: []core.Operand{{Kind: core.OperandSymbol, Name: name}}, Effects: core.EffectCall}}}}},
			{ID: 2, Name: "plain_c", Address: 0x2000}}}
	text := Format(ir)
	got, err := Parse(text)
	if err != nil {
		t.Fatalf("round trip: %v\n%s", err, text)
	}
	if got.Functions[0].Name != name || got.Functions[0].Blocks[0].Instructions[0].Operands[0].Name != name {
		t.Fatalf("Rust name changed: %+v", got.Functions[0])
	}
	if !reflect.DeepEqual(got.Functions[0].Facts, &facts) {
		t.Fatalf("function ABI lost: %+v", got.Functions[0].Facts)
	}
	if got.FactsFor(got.Functions[1]).Registers.Arguments[0] != "rdi" {
		t.Fatal("Go ABI leaked into the C function")
	}
}

func TestMalformedLanguageText(t *testing.T) {
	for _, text := range []string{
		"function-facts: {}\n",
		"function f @ 0x1\nfunction-facts: invalid\n",
		"function f @ 0x1\nblock b1 @ 0x1\n  0x1  call @\"unterminated\n",
	} {
		if _, err := Parse(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
