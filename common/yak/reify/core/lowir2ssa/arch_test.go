package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
)

// Test386RegisterView covers the 32-bit names that share a canonical variable
// with their 64-bit spellings.
func Test386RegisterView(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  string
		width uint16
	}{
		{"eax", "rax", 32},
		{"esp", "rsp", 32},
		{"ebp", "rbp", 32},
		{"eflags", "rflags", 64},
	} {
		view := (&converter{facts: core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Width: 32, Flags: "eflags", FlagsWidth: 64}}}).registerView(tc.name, 0)
		if view.canonical != tc.want || view.width != tc.width {
			t.Fatalf("%s -> %+v, want canonical %s width %d", tc.name, view, tc.want, tc.width)
		}
	}
}

// TestARMRegisterView covers the named ARM registers and the flags register.
func TestARMRegisterView(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  string
		width uint16
	}{
		{"r0", "r0", 32},
		{"sp", "sp", 32},
		{"lr", "lr", 32},
		{"pc", "pc", 32},
		{"cpsr", "cpsr", 32},
	} {
		view := (&converter{facts: core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Width: 32, Flags: "cpsr", FlagsWidth: 32}}}).registerView(tc.name, 0)
		if view.canonical != tc.want || view.width != tc.width {
			t.Fatalf("%s -> %+v, want canonical %s width %d", tc.name, view, tc.want, tc.width)
		}
	}
}

// TestARMConditionRecovery checks that flags.* and cond.* fold into a
// comparison instead of an opaque string marker.
func TestARMConditionRecovery(t *testing.T) {
	ir, err := lowirtext.Parse(`target: linux/arm (32-bit, little-endian)
frame: sp=sp fp=r11 ret=lr flags=cpsr saved=r4,r5,r6,r7,r8,r9,r10,r11 growth=-1
registers: naming=plain width=32 flags=cpsr flags_width=32 args=r0,r1,r2,r3 return=r0
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = read_register %r0 [read_register]
  0x1000  v2:i32 = constant 0x1
  0x1000  v3:i32 = intrinsic @flags.cmp, v1, v2 [write_register]
  0x1000  write_register %cpsr, v3 [write_register]
  0x1004  v4:i32 = read_register %cpsr [read_register]
  0x1004  v5:i1 = intrinsic @cond.eq, v4 [read_register]
  0x1004  branch v5 -> b2, b3
block b2 @ 0x1008
  0x1008  jump -> b3
block b3 @ 0x100c
  0x100c  return`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	if strings.Contains(text, "cond.eq") {
		t.Fatalf("condition was not recovered:\n%s", text)
	}
	if !strings.Contains(text, "eq") {
		t.Fatalf("comparison is missing:\n%s", text)
	}
}

// Test386StackArguments checks that 386 binds no register argument, because
// cdecl passes every argument on the stack.
func Test386StackArguments(t *testing.T) {
	ir, err := lowirtext.Parse(`target: linux/386 (32-bit, little-endian)
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = read_register %eax [read_register]
  0x1000  return`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	function, ok := result.Program.Funcs.Get("f")
	if !ok {
		t.Fatal("function f is missing")
	}
	if len(function.Params) != 0 {
		t.Fatalf("386 function has %d register params, want 0", len(function.Params))
	}
}

// TestARMArguments checks that ARM binds r0..r3 as parameters and returns in r0.
func TestARMArguments(t *testing.T) {
	ir, err := lowirtext.Parse(`target: linux/arm (32-bit, little-endian)
frame: sp=sp fp=r11 ret=lr flags=cpsr saved=r4,r5,r6,r7,r8,r9,r10,r11 growth=-1
registers: naming=plain width=32 flags=cpsr flags_width=32 args=r0,r1,r2,r3 return=r0
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = read_register %r0 [read_register]
  0x1000  v2:i32 = read_register %r1 [read_register]
  0x1000  v3:i32 = add v1, v2
  0x1000  write_register %r0, v3 [write_register]
  0x1000  return`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	function, ok := result.Program.Funcs.Get("f")
	if !ok {
		t.Fatal("function f is missing")
	}
	if len(function.Params) != 4 {
		t.Fatalf("ARM function has %d params, want 4", len(function.Params))
	}
	text := Text(result.Program)
	if strings.Contains(text, "undefined") {
		t.Fatalf("parameter was not bound:\n%s", text)
	}
}

// TestX86FlagsName checks that the 386 flags pseudo register stays one word.
func TestX86FlagsName(t *testing.T) {
	view := (&converter{facts: core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Width: 32, Flags: "eflags", FlagsWidth: 64}}}).registerView("eflags", 32)
	if view.canonicalBits != 64 || view.width != 64 {
		t.Fatalf("eflags view = %+v", view)
	}
}

var _ = core.ValueType{}
