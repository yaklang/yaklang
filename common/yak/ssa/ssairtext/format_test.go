package ssairtext

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseAssignmentLineAndRenumberWordBoundaries(t *testing.T) {
	if typ, name, right, ok := parseAssignmentLine("\t<int32> t42 = add t10, (7)"); !ok || typ != "int32" || name != "t42" || right != "add t10, (7)" {
		t.Fatalf("unexpected typed assignment parse: ok=%v typ=%q name=%q right=%q", ok, typ, name, right)
	}
	if typ, name, right, ok := parseAssignmentLine("\tt12   =   call foo(xt5, t12, int32)"); !ok || typ != "" || name != "t12" || right != "call foo(xt5, t12, int32)" {
		t.Fatalf("unexpected untyped assignment parse: ok=%v typ=%q name=%q right=%q", ok, typ, name, right)
	}
	if _, _, _, ok := parseAssignmentLine("\t<> t1 = 0"); ok {
		t.Fatal("empty type brackets should not match")
	}
	if _, _, _, ok := parseAssignmentLine("t1 = 0"); ok {
		t.Fatal("non-tab-indented line should not match")
	}

	input := "fn (99) a:\n\t<int32> t42 = add t10, t42 // xt5 int32 t42x\n\tcall foo((99), t10)\n"
	want := "fn (1) a:\n\t<int32> t2 = add t3, t2 // xt5 int32 t42x\n\tcall foo((1), t3)\n"
	if got := renumberIDs(input); got != want {
		t.Fatalf("renumberIDs mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatHyphenatedBlocksAndFormatTo(t *testing.T) {
	prog, err := Parse("sample\ntype: () -> void\nentry_0:\n\tjump -> if_then_1\nif_then_1:\n\tret")
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := prog.Funcs.Get("sample")
	if !ok || fn == nil || len(fn.Blocks) < 2 {
		t.Fatalf("unexpected parsed function: %+v", fn)
	}
	if bb, ok := fn.GetBasicBlockByID(fn.Blocks[1]); ok && bb != nil {
		bb.SetName("if-then-1")
	}

	got := Format(fn)
	if !strings.Contains(got, "if_then_1") || strings.Contains(got, "if-then-1") {
		t.Fatalf("hyphenated block name not normalized:\n%s", got)
	}
	var streamed bytes.Buffer
	if err := FormatTo(&streamed, fn); err != nil {
		t.Fatal(err)
	}
	if streamed.String() != got {
		t.Fatalf("FormatTo mismatch:\ngot:\n%s\nwant:\n%s", streamed.String(), got)
	}
}
