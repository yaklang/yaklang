package asm2lowir

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestAnnotateSymbolsNamesFunctionsAndCalls(t *testing.T) {
	artifact := &core.Artifact{
		Image: &core.Image{Symbols: []symbol.Symbol{
			{Name: "puts", Address: 0x1000, Size: 0x10, Kind: symbol.SymbolFunction, Imported: true},
			{Name: "__libc_start_main", Address: 0x2000, Size: 8, Kind: symbol.SymbolFunction, Imported: true},
		}},
	}
	ir := &core.LowIR{Functions: []core.Function{{
		ID:      1,
		Address: 0x1000,
		Blocks: []core.Block{{
			ID:      1,
			Address: 0x1000,
			Instructions: []core.Instruction{
				{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{0x00, 0x10}}}},
				{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: core.ValueType{Kind: core.TypePointer, Bits: 64}}}, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{0x00, 0x20}}}},
				{Kind: core.OpLoad, Results: []core.Value{{ID: 2, Type: core.ValueType{Kind: core.TypeInteger, Bits: 64}}}, Operands: []core.Operand{{Kind: core.OperandValue, Value: 1}}},
				{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandValue, Value: 2}}},
				{Kind: core.OpReturn},
			},
		}},
	}}}

	annotateSymbols(artifact, ir)

	function := ir.Functions[0]
	if function.Name != "puts" {
		t.Fatalf("function name = %q", function.Name)
	}
	if got := function.Blocks[0].Instructions[0].Operands[0]; got.Kind != core.OperandSymbol || got.Name != "puts" {
		t.Fatalf("direct call target = %+v", got)
	}
	if got := function.Blocks[0].Instructions[3].Operands[0]; got.Kind != core.OperandSymbol || got.Name != "__libc_start_main" {
		t.Fatalf("GOT call target = %+v", got)
	}
}

// The two callers reuse v1..v3. Only the first one's load reads a known GOT
// slot. Neither losing that symbol nor lending it to the other call is valid.
func TestCallTargetValueIDsAreFunctionLocal(t *testing.T) {
	artifact := &core.Artifact{Image: &core.Image{Symbols: []symbol.Symbol{
		{Name: "puts", Address: 0x3000, Kind: symbol.SymbolFunction, Imported: true},
	}}}
	const got = `function from_got @ 0x1000
block b1 @ 0x1000
  0x1000  v1:ptr64 = constant 0x3000
  0x1000  v2:i64 = load v1
  0x1000  v3:ptr64 = copy v2
  0x1000  call v3
  0x1006  return
`
	const dynamic = `function from_register @ 0x2000
block b1 @ 0x2000
  0x2000  v1:ptr64 = read_register %rax
  0x2000  v2:i64 = load v1
  0x2000  v3:ptr64 = copy v2
  0x2000  call v3
  0x2002  return
`
	for _, text := range []string{got + dynamic, dynamic + got} {
		ir, err := lowirtext.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		annotateSymbols(artifact, ir)
		for _, f := range ir.Functions {
			target := f.Blocks[0].Instructions[3].Operands[0]
			if f.Name == "from_got" && (target.Kind != core.OperandSymbol || target.Name != "puts") {
				t.Errorf("%s: want puts, got %+v", f.Name, target)
			}
			if f.Name == "from_register" && (target.Kind != core.OperandValue || target.Value != 3) {
				t.Errorf("%s: want indirect call through v3, got %+v", f.Name, target)
			}
		}
	}
}

// TestMappingSymbolIsNeverACallTarget covers the RISC-V and ARM mapping symbols.
// A mapping symbol marks where one encoding or data run starts and may carry an
// ISA string, so using it as a call name produces an identifier that is not a
// legal C name.
func TestMappingSymbolIsNeverACallTarget(t *testing.T) {
	artifact := &core.Artifact{
		Image: &core.Image{Symbols: []symbol.Symbol{{
			Name:    "$xrv64i2p1_m2p0",
			Address: 0x1000,
			Kind:    symbol.SymbolUnknown,
		}}},
	}
	ir := &core.LowIR{Functions: []core.Function{{
		ID:      1,
		Address: 0x2000,
		Blocks: []core.Block{{
			ID:      1,
			Address: 0x2000,
			Instructions: []core.Instruction{
				{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{0x00, 0x10}}}},
				{Kind: core.OpReturn},
			},
		}},
	}}}

	annotateSymbols(artifact, ir)

	if got := ir.Functions[0].Blocks[0].Instructions[0].Operands[0]; got.Kind == core.OperandSymbol {
		t.Fatalf("mapping symbol used as a call target: %+v", got)
	}
}
