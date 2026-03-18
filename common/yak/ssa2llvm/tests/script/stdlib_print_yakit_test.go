package script

import (
	"strings"
	"testing"

	s2tests "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func TestStdlibPrint_Stdout(t *testing.T) {
	output := s2tests.RunYakScriptFile(t, "print_stdlib.yak", nil)
	if got := output; got != "hello world 123\nx=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestStdlibYakit_Stderr(t *testing.T) {
	output := s2tests.RunYakScriptFile(t, "yakit_stdlib.yak", nil)
	want := strings.Join([]string{
		"[yakit][info] i=1",
		"[yakit][warn] w=x",
		"[yakit][debug] d=2",
		"[yakit][error] e=3",
		"",
	}, "\n")
	if got := output; got != want {
		t.Fatalf("unexpected output: %q", got)
	}
}
