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
