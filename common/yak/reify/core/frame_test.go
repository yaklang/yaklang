package core

import (
	"strings"
	"testing"
)

// TestAliasOfX86 checks the partial views of one register, including the high byte
// views a write has to merge at bit 8.
func TestAliasOfX86(t *testing.T) {
	model := RegisterModel{Naming: RegisterNamingX86, Width: 64}
	cases := []struct {
		name  string
		want  RegisterAlias
		found bool
	}{
		{"rax", RegisterAlias{Name: "rax", Width: 64}, true},
		{"eax", RegisterAlias{Name: "rax", Width: 32}, true},
		{"ax", RegisterAlias{Name: "rax", Width: 16}, true},
		{"al", RegisterAlias{Name: "rax", Width: 8}, true},
		{"ah", RegisterAlias{Name: "rax", Width: 8, Shift: 8}, true},
		{"rsp", RegisterAlias{Name: "rsp", Width: 64}, true},
		{"esp", RegisterAlias{Name: "rsp", Width: 32}, true},
		{"spl", RegisterAlias{Name: "rsp", Width: 8}, true},
		{"spb", RegisterAlias{Name: "rsp", Width: 8}, true},
		{"bpb", RegisterAlias{Name: "rbp", Width: 8}, true},
		{"sib", RegisterAlias{Name: "rsi", Width: 8}, true},
		{"dib", RegisterAlias{Name: "rdi", Width: 8}, true},
		{"r8", RegisterAlias{Name: "r8", Width: 64}, true},
		{"r8b", RegisterAlias{Name: "r8", Width: 8}, true},
		{"r8w", RegisterAlias{Name: "r8", Width: 16}, true},
		{"r8d", RegisterAlias{Name: "r8", Width: 32}, true},
		{"r15l", RegisterAlias{Name: "r15", Width: 32}, true},
		{"rip", RegisterAlias{Name: "rip", Width: 64}, true},
		{"eflags", RegisterAlias{Name: "rflags", Width: 64}, true},
		{"xmm0", RegisterAlias{}, false},
	}
	for _, tc := range cases {
		got, ok := model.AliasOf(tc.name)
		if ok != tc.found {
			t.Fatalf("AliasOf(%q) found = %v", tc.name, ok)
		}
		if ok && got != tc.want {
			t.Fatalf("AliasOf(%q) = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestAliasOfAArch64 checks that a W view shares its variable with the X register
// and that the stack, zero, frame and link registers keep their names.
func TestAliasOfAArch64(t *testing.T) {
	model := RegisterModel{Naming: RegisterNamingAArch64, Width: 64}
	cases := []struct {
		name string
		want RegisterAlias
	}{
		{"x0", RegisterAlias{Name: "x0", Width: 64}},
		{"w0", RegisterAlias{Name: "x0", Width: 32}},
		{"x29", RegisterAlias{Name: "x29", Width: 64}},
		{"fp", RegisterAlias{Name: "x29", Width: 64}},
		{"lr", RegisterAlias{Name: "x30", Width: 64}},
		{"sp", RegisterAlias{Name: "sp", Width: 64}},
		{"wsp", RegisterAlias{Name: "sp", Width: 64}},
		{"xzr", RegisterAlias{Name: "xzr", Width: 64}},
		{"wzr", RegisterAlias{Name: "xzr", Width: 64}},
	}
	for _, tc := range cases {
		got, ok := model.AliasOf(tc.name)
		if !ok {
			t.Fatalf("AliasOf(%q) was rejected", tc.name)
		}
		if got != tc.want {
			t.Fatalf("AliasOf(%q) = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestAliasOfPlainKeepsTheName checks that a target without partial views reports
// one name as its own alias and leaves the width to the caller.
func TestAliasOfPlainKeepsTheName(t *testing.T) {
	for _, naming := range []string{RegisterNamingPlain, ""} {
		model := RegisterModel{Naming: naming, Width: 64}
		for _, name := range []string{"r3", "x10", "v0", "R2"} {
			got, ok := model.AliasOf(name)
			if !ok {
				t.Fatalf("%q rejected for naming %q", name, naming)
			}
			// The name is lowercased, because every operand spelling a parser
			// writes is already lower case.
			want := strings.ToLower(name)
			if got.Name != want || got.Width != 0 || got.Shift != 0 {
				t.Fatalf("%q under %q gave %+v", name, naming, got)
			}
		}
	}
}
