package lowir2ssa

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

//go:embed testcases
var testcases embed.FS

// lowIRCase is one conversion fixture. LowIR, SSAIR and source are stored as the
// lines of the text they hold, so a diff shows one changed line instead of the
// whole document.
type lowIRCase struct {
	Name   string   `json:"name"`
	LowIR  []string `json:"lowir"`
	SSAIR  []string `json:"ssair"`
	Source []string `json:"source"`
}

func loadLowIRCases(t *testing.T) []lowIRCase {
	t.Helper()
	entries, err := testcases.ReadDir("testcases")
	if err != nil {
		t.Fatal(err)
	}
	var cases []lowIRCase
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := testcases.ReadFile("testcases/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var single lowIRCase
		if err := json.Unmarshal(data, &single); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		if single.Name == "" {
			single.Name = strings.TrimSuffix(entry.Name(), ".json")
		}
		cases = append(cases, single)
	}
	return cases
}

func TestConvertRejectsFunctionWithoutBlocks(t *testing.T) {
	ir := &core.LowIR{Functions: []core.Function{{Name: "empty", Address: 0x1000}}}
	_, err := Convert(context.Background(), ir)
	if err == nil || err.Error() != "lowir2ssa: function empty at 0x1000 has no basic blocks" {
		t.Fatalf("expected a missing basic blocks error, got %v", err)
	}
}

func TestConvertSkipsCopyWithoutResult(t *testing.T) {
	ir := &core.LowIR{Functions: []core.Function{{
		Name: "copy_result",
		Blocks: []core.Block{{ID: 1, Instructions: []core.Instruction{
			{Kind: core.OpCopy, Operands: []core.Operand{{Kind: core.OperandConstant, Literal: []byte{1}}}},
			{Kind: core.OpReturn},
		}}},
	}}}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	if got := Text(result.Program); !strings.HasSuffix(strings.TrimSpace(got), "\n\tret") {
		t.Fatalf("return missing from SSAIR: %s", got)
	}
}

func TestLowIRToSSAIRFixtures(t *testing.T) {
	cases := loadLowIRCases(t)
	if len(cases) == 0 {
		t.Fatal("no testcases")
	}
	count := 0
	for _, tc := range cases {
		count++
		t.Run(tc.Name, func(t *testing.T) {
			ir, err := lowirtext.Parse(strings.Join(tc.LowIR, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := Convert(context.Background(), ir)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := normalizeIndent(Text(result.Program)), normalizeIndent(strings.Join(tc.SSAIR, "\n")); got != want {
				t.Fatalf("SSAIR mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
			source, err := ssa2source.Render(context.Background(), result.Program)
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.Join(tc.Source, "\n"); source != want {
				t.Fatalf("source mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, source)
			}
		})
	}
	if count == 0 {
		t.Fatal("no testcases")
	}
}

// TestConvertIsDeterministic guards the order values are created in. Phis of a
// join are keyed by register or memory slot, and creating them from map iteration
// order would make their identifiers differ between runs of the same input.
func TestConvertIsDeterministic(t *testing.T) {
	const rounds = 25
	for _, tc := range loadLowIRCases(t) {
		ir, err := lowirtext.Parse(strings.Join(tc.LowIR, "\n"))
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		first := ""
		for round := 0; round < rounds; round++ {
			result, err := Convert(context.Background(), ir)
			if err != nil {
				t.Fatalf("%s: %v", tc.Name, err)
			}
			text := Text(result.Program)
			if round == 0 {
				first = text
				continue
			}
			if text != first {
				t.Fatalf("%s: conversion differs on round %d:\n--- first ---\n%s\n--- round %d ---\n%s", tc.Name, round, first, round, text)
			}
		}
	}
}

func TestValuesCarryBitWidthTypes(t *testing.T) {
	converter := &converter{types: make(map[core.ValueType]ssa.Type)}
	for bits, expected := range map[uint16]string{8: "i8", 16: "i16", 32: "i32", 64: "i64"} {
		if got := converter.typeFor(core.ValueType{Kind: core.TypeInteger, Bits: bits}).String(); got != expected {
			t.Fatalf("i%d type = %s", bits, expected)
		}
	}
	if got := converter.typeFor(core.ValueType{Kind: core.TypePointer, Bits: 64}).String(); got != "ptr64" {
		t.Fatalf("pointer type = %s", got)
	}
	if got := converter.typeFor(core.ValueType{Kind: core.TypeFloat, Bits: 64}).String(); got != "f64" {
		t.Fatalf("float type = %s", got)
	}
}

func TestConvertRejectsEmptyAndCancelled(t *testing.T) {
	if _, err := Convert(context.Background(), nil); err == nil {
		t.Fatal("nil LowIR accepted")
	}
	if _, err := Convert(context.Background(), &core.LowIR{}); err == nil {
		t.Fatal("empty LowIR accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ir, err := lowirtext.Parse(`target: linux/amd64 (64-bit, little-endian)
function cancelled @ 0x0
block b1 @ 0x0
  0x0  v1:i64 = constant 0x1
  0x1  return v1`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(ctx, ir); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestRegisterViewNames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bits      uint16
		canonical string
		width     uint16
		shift     uint16
	}{
		{"al", 8, "rax", 8, 0},
		{"ah", 8, "rax", 8, 8},
		{"eax", 32, "rax", 32, 0},
		{"rax", 64, "rax", 64, 0},
		{"r8b", 8, "r8", 8, 0},
		{"r8w", 16, "r8", 16, 0},
		{"r8d", 32, "r8", 32, 0},
		{"r8", 64, "r8", 64, 0},
		{"xmm3", 128, "xmm3", 128, 0},
		{"rflags", 64, "rflags", 64, 0},
	} {
		view := registerViewFor(tc.name, tc.bits, core.RegisterModel{Naming: core.RegisterNamingX86, Width: 64, Flags: "rflags", FlagsWidth: 64})
		if view.canonical != tc.canonical || view.width != tc.width || view.shift != tc.shift {
			t.Fatalf("%s: %+v", tc.name, view)
		}
	}
}

// normalizeIndent drops the leading tab of each line. The fixture stores the text
// without the indent so it reads as plain lines; the comparison is about content.
func normalizeIndent(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func TestConvertResetsConverterStateBetweenFunctions(t *testing.T) {
	irText := `target: linux/amd64 (64-bit, little-endian)
frame: sp=rsp fp=rbp flags=rflags saved=rbp,rbx,r12,r13,r14,r15 growth=-1
registers: naming=x86 width=64 flags=rflags flags_width=64 args=rdi,rsi,rdx,rcx,r8,r9 return=rax
function fn_one @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i1 = constant 0x1
  0x1001  branch v1 -> b2, b3
block b2 @ 0x1010
  0x1010  v2:i64 = constant 0xa
  0x1011  write_register %rax, v2
  0x1012  jump -> b4
block b3 @ 0x1020
  0x1020  v3:i64 = constant 0x14
  0x1021  write_register %rax, v3
  0x1022  jump -> b4
block b4 @ 0x1030
  0x1030  v4:i64 = read_register %rax
  0x1031  return v4
function fn_two @ 0x2000
block b5 @ 0x2000
  0x2000  v5:i1 = constant 0x1
  0x2001  branch v5 -> b6, b7
block b6 @ 0x2010
  0x2010  v6:i64 = constant 0x1e
  0x2011  write_register %rax, v6
  0x2012  jump -> b8
block b7 @ 0x2020
  0x2020  v7:i64 = constant 0x28
  0x2021  write_register %rax, v7
  0x2022  jump -> b8
block b8 @ 0x2030
  0x2030  v8:i64 = read_register %rax
  0x2031  return v8`
	ir, err := lowirtext.Parse(irText)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), ir, WithConsumeIR())
	if err != nil {
		t.Fatal(err)
	}
	for i, fn := range ir.Functions {
		if fn.Blocks != nil || fn.Facts != nil {
			t.Fatalf("WithConsumeIR did not release function %d", i)
		}
	}
	for _, fn := range res.Program.Funcs.Values() {
		if fn == nil || fn.IsExtern() {
			continue
		}
		for _, bid := range fn.Blocks {
			blk, ok := fn.GetBasicBlockByID(bid)
			if !ok || blk == nil {
				continue
			}
			for _, pid := range blk.Phis {
				inst, ok := fn.GetInstructionById(pid)
				if !ok {
					continue
				}
				phi, ok := ssa.ToPhi(inst)
				if !ok {
					continue
				}
				if len(phi.Edge) != len(blk.Preds) {
					t.Fatalf("fn %s phi %d edges=%d != preds=%d", fn.GetName(), pid, len(phi.Edge), len(blk.Preds))
				}
			}
		}
	}
}
