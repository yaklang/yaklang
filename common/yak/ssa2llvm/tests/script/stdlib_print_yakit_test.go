package script

import (
	"strings"
	"testing"

	s2tests "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

// TestStdlibPrint_Stdout compiles print_stdlib.yak via the real ssa2llvm CLI
// and verifies the produced binary's stdout matches the expected output.
func TestStdlibPrint_Stdout(t *testing.T) {
	output := s2tests.RunYakScriptFileWithCLI(t, "print_stdlib.yak", nil)
	if got := output; got != "hello world 123\nx=1\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

// TestStdlibYakit_Stderr compiles yakit_stdlib.yak via the real ssa2llvm CLI
// and verifies the yakit.Info/Warn/Debug/Error stderr logging lines.
func TestStdlibYakit_Stderr(t *testing.T) {
	output := s2tests.RunYakScriptFileWithCLI(t, "yakit_stdlib.yak", nil)
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
