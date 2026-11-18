package asm2lowir

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestLanguageFactsArePerFunction(t *testing.T) {
	a := &core.Artifact{
		Language: &core.LanguageReport{Language: core.LanguageGo, ABI: &core.ABI{
			Registers:    core.RegisterModel{Arguments: []string{"rax", "rbx"}, Return: "rax"},
			NoCalleeSave: true}},
		Image: &core.Image{Symbols: []symbol.Symbol{
			{Name: "main.Add", Address: 0x1000, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab},
			{Name: "puts", Address: 0x2000, Kind: symbol.SymbolFunction},
			{Name: "runtime.morestack.abi0", Address: 0x3000, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab, Convention: symbol.ConventionPlatform},
			{Name: "crossCall2", Address: 0x3100, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab, Convention: symbol.ConventionPlatform},
		}},
	}
	ir := &core.LowIR{
		Frame:     core.FrameModel{FramePointer: "rbp", CalleeSaved: []string{"rbp", "rbx", "r12"}},
		Registers: core.RegisterModel{Arguments: []string{"rdi", "rsi"}},
	}
	facts := languageFacts(a, core.FunctionBoundary{Address: 0x1000}, ir)
	if facts == nil || facts.Registers.Arguments[0] != "rax" || len(facts.Frame.CalleeSaved) != 1 {
		t.Fatalf("Go function facts = %+v", facts)
	}
	for _, address := range []uint64{0x2000, 0x3000, 0x3100, 0x4000} {
		if got := languageFacts(a, core.FunctionBoundary{Address: address}, ir); got != nil {
			t.Fatalf("Go ABI applied to unverified function 0x%x", address)
		}
	}
	if ir.Registers.Arguments[0] != "rdi" || len(ir.Frame.CalleeSaved) != 3 {
		t.Fatal("architecture facts were mutated")
	}
}
