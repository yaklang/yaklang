package lowir2ssa

import (
	"context"
	"embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

//go:embed testcases
var testcases embed.FS

func TestLowIRToSSAIRFixtures(t *testing.T) {
	entries, err := testcases.ReadDir("testcases")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		t.Run(entry.Name(), func(t *testing.T) {
			lowIRText, err := testcases.ReadFile("testcases/" + entry.Name() + "/lowir.txt")
			if err != nil {
				t.Fatal(err)
			}
			ir, err := lowirtext.Parse(string(lowIRText))
			if err != nil {
				t.Fatal(err)
			}
			result, err := Convert(context.Background(), ir)
			if err != nil {
				t.Fatal(err)
			}
			wantSSAIR, err := testcases.ReadFile("testcases/" + entry.Name() + "/ssair.txt")
			if err != nil {
				t.Fatal(err)
			}
			if got := Text(result.Program); got != string(wantSSAIR) {
				t.Fatalf("SSAIR mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantSSAIR, got)
			}
			source, err := ssa2source.Render(context.Background(), result.Program)
			if err != nil {
				t.Fatal(err)
			}
			wantSource, err := testcases.ReadFile("testcases/" + entry.Name() + "/source.txt")
			if err != nil {
				t.Fatal(err)
			}
			if source != string(wantSource) {
				t.Fatalf("source mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantSource, source)
			}
		})
	}
	if count == 0 {
		t.Fatal("no testcases")
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
		view := registerViewFor(tc.name, tc.bits)
		if view.canonical != tc.canonical || view.width != tc.width || view.shift != tc.shift {
			t.Fatalf("%s: %+v", tc.name, view)
		}
	}
}
