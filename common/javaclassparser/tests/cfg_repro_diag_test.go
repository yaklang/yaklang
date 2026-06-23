package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestCFGReproDiag decompiles a single .class file (path from env CFG_CLASS) and prints the
// resulting Java together with a per-method stub report. It is a diagnostic aid for the
// "multiple next" / "if must have two next" control-flow structuring failures and is skipped
// unless CFG_CLASS is set.
func TestCFGReproDiag(t *testing.T) {
	path := os.Getenv("CFG_CLASS")
	if path == "" {
		t.Skip("set CFG_CLASS to a .class file path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read class: %v", err)
	}
	if os.Getenv("CFG_NOVALIDATE") != "" {
		javaclassparser.EnableDecompileSyntaxValidation = false
		defer func() { javaclassparser.EnableDecompileSyntaxValidation = true }()
	}
	out, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile error: %v", err)
	}
	stub := strings.Contains(out, javaclassparser.DecompileStubMarker)
	t.Logf("stub=%v len=%d\n========= DECOMPILED =========\n%s\n==============================", stub, len(out), out)
	if stub {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, javaclassparser.DecompileStubMarker) {
				t.Logf("STUB LINE: %s", strings.TrimSpace(line))
			}
		}
	}
}
