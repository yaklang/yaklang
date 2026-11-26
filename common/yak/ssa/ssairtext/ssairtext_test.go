package ssairtext

import (
	"embed"
	"strings"
	"testing"
)

//go:embed testdata/base.ssair
var testdata embed.FS

// TestRoundTrip is the only contract of this package: text to program to text.
// It does not depend on any renderer, so a failure points at the format itself.
func TestRoundTrip(t *testing.T) {
	data, err := testdata.ReadFile("testdata/base.ssair")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimRight(string(data), "\n")
	program, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatProgram(program); got != text+"\n" {
		t.Fatalf("round trip changed the text:\n--- want ---\n%s\n--- got ---\n%s", text, got)
	}
}

func TestParseRejectsUnsupportedForms(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("empty input was accepted")
	}
	if _, err := Parse("sub\n\tret\nb9:\n\tjump -> b404\n"); err == nil {
		t.Fatal("jump to an unknown block was accepted")
	}
	if _, err := Parse("sub\n\tunknown instruction\n"); err == nil {
		t.Fatal("unknown instruction was accepted")
	}
}

func TestFormatProgramSkipsExterns(t *testing.T) {
	program, err := Parse("extern puts\ncaller\ntype: () -> void\nentry_0:\n\tret")
	if err != nil {
		t.Fatal(err)
	}
	text := FormatProgram(program)
	if !strings.HasPrefix(text, "extern puts\n") {
		t.Fatalf("extern declaration is missing:\n%s", text)
	}
	if strings.Count(text, "extern puts") != 1 {
		t.Fatalf("extern declaration is duplicated:\n%s", text)
	}
}

// TestMemoryTextRoundTrip checks the text form of a memory read and a memory
// write. Both carry real operands: a read names the address it loads from, and a
// write names the address and the stored value. Neither is a call, so the text
// never wraps the mnemonic in parentheses.
func TestMemoryTextRoundTrip(t *testing.T) {
	const text = `sub_0x1000 <i64> (1) arg0
type: (i64) -> i32
entry_0:
	<ptr64> t2 = type_cast[ptr64] <i64> arg0
	<i32> t3   = load <ptr64> t2
	<i32> t5   = <ptr64> t2 store <i32> 2
	ret <i32> t3`
	program, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(program)
	for _, fragment := range []string{"= load <ptr64> t2", "= <ptr64> t2 store <i32> 2"} {
		if !strings.Contains(formatted, fragment) {
			t.Fatalf("memory operation lost its operands, want %q:\n%s", fragment, formatted)
		}
	}
	for _, forbidden := range []string{"load(", "load:", "store(", "store:"} {
		if strings.Contains(formatted, forbidden) {
			t.Fatalf("memory operation printed as a call or a marker %q:\n%s", forbidden, formatted)
		}
	}
	reparsed, err := Parse(formatted)
	if err != nil {
		t.Fatalf("formatted text does not parse back: %v\n%s", err, formatted)
	}
	if again := FormatProgram(reparsed); again != formatted {
		t.Fatalf("round trip changed the text:\n--- first ---\n%s\n--- second ---\n%s", formatted, again)
	}
}

// TestStoreWordInStringConstant checks that only a real store parses as one. A
// string constant can contain the separator, and it must stay a constant.
func TestStoreWordInStringConstant(t *testing.T) {
	program, err := Parse("f\ntype: () -> void\nentry_0:\n\t<string> t1 = <string> \"a store b\"\n\tret")
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatProgram(program)
	if !strings.Contains(formatted, "<string> a store b") {
		t.Fatalf("string constant was not preserved:\n%s", formatted)
	}
	// A store line always names a typed operand after the mnemonic.
	if strings.Contains(formatted, " store <") {
		t.Fatalf("string constant was parsed as a store:\n%s", formatted)
	}
}
