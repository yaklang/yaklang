package script

import (
	"strings"
	"testing"

	s2tests "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

// TestSsaGoParse_Output compiles ssa_go_parse.yak via the real ssa2llvm CLI
// and verifies that parsing a small Go snippet with ssa.Parse + SyntaxFlow
// finds the println argument and prints it.
func TestSsaGoParse_Output(t *testing.T) {
	output := s2tests.RunYakScriptFileWithCLI(t, "ssa_go_parse.yak", nil)
	if !strings.Contains(output, "matched-println-arg-count: 1") {
		t.Fatalf("missing matched-println-arg-count in output: %q", output)
	}
	if !strings.Contains(output, `"hello-from-go"`) {
		t.Fatalf("missing hello-from-go value in output: %q", output)
	}
}
