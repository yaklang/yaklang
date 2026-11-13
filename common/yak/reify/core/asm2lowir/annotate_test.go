package asm2lowir

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
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
