package core

import (
	"context"
	"testing"
)

func TestArtifactStringAt(t *testing.T) {
	artifact := testArtifact(t)
	raw := make([]byte, 100)
	copy(raw[0x10:], []byte("hello, reify\x00trailing"))
	artifact.Reader = newReader(t, raw)
	if text, ok := artifact.StringAt(0x1000, 64); !ok || text != "hello, reify" {
		t.Fatalf("StringAt = %q %v", text, ok)
	}
	if _, ok := artifact.StringAt(0x2000, 64); ok {
		t.Fatal("zero-fill section returned a string")
	}
}

func TestArtifactAnnotatesSymbols(t *testing.T) {
	artifact := testArtifact(t)
	artifact.Image.Symbols = []Symbol{
		{Name: "puts", Address: 0x1000, Size: 0x10, Kind: SymbolFunction, Imported: true},
		{Name: "__libc_start_main", Address: 0x2000, Size: 8, Kind: SymbolFunction, Imported: true},
	}
	target := artifact.Target
	ir := &LowIR{
		Target: target,
		Functions: []Function{{
			ID:      1,
			Address: 0x1000,
			Blocks: []Block{{
				ID:      1,
				Address: 0x1000,
				Instructions: []Instruction{
					{Kind: OpCall, Operands: []Operand{{Kind: OperandConstant, Literal: []byte{0x00, 0x10}}}, Address: 0x1000},
					{Kind: OpConstant, Results: []Value{{ID: 1, Type: ValueType{Kind: TypePointer, Bits: 64}}}, Operands: []Operand{{Kind: OperandConstant, Literal: []byte{0x00, 0x20}}}, Address: 0x1001},
					{Kind: OpLoad, Results: []Value{{ID: 2, Type: ValueType{Kind: TypeInteger, Bits: 64}}}, Operands: []Operand{{Kind: OperandValue, Value: 1}}, Address: 0x1002},
					{Kind: OpCall, Operands: []Operand{{Kind: OperandValue, Value: 2}}, Address: 0x1003},
					{Kind: OpReturn, Address: 0x1004},
				},
			}},
		}},
	}
	artifact.Parser = &stubParser{available: true, result: &ParseResult{IR: ir}}
	result, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	function := result.IR.Functions[0]
	if function.Name != "puts" {
		t.Fatalf("function name = %q", function.Name)
	}
	if got := function.Blocks[0].Instructions[0].Operands[0]; got.Kind != OperandSymbol || got.Name != "puts" {
		t.Fatalf("direct call target = %+v", got)
	}
	if got := function.Blocks[0].Instructions[3].Operands[0]; got.Kind != OperandSymbol || got.Name != "__libc_start_main" {
		t.Fatalf("GOT call target = %+v", got)
	}
}

func TestImageSymbolAt(t *testing.T) {
	image := &Image{Symbols: []Symbol{
		{Name: "start", Address: 0x1000, Size: 0x20, Kind: SymbolFunction},
		{Name: "main", Address: 0x1010, Size: 0, Kind: SymbolFunction},
	}}
	if symbol, ok := image.SymbolAt(0x1010); !ok || symbol.Name != "main" {
		t.Fatalf("exact symbol = %+v %v", symbol, ok)
	}
	if symbol, ok := image.SymbolAt(0x1004); !ok || symbol.Name != "start" {
		t.Fatalf("range symbol = %+v %v", symbol, ok)
	}
	if _, ok := image.SymbolAt(0x1030); ok {
		t.Fatal("unknown address matched")
	}
}
