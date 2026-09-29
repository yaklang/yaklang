package tests

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps every scratch artifact of this package inside one per-run
// root and removes that root when the run ends.
//
// A single ssa2llvm compile writes roughly 600 MB into $TMPDIR: a private copy
// of the runtime archive (up to ~390 MB for the staticanalyze tier) plus the
// linked binary, and the compiler deliberately keeps those deterministic work
// dirs around so later compiles can reuse them. A mustpass run compiles 115
// scripts, which leaves about 60 GB behind per run when nothing collects it.
//
// Setting $TMPDIR before m.Run() redirects the compiler work dirs, the CLI
// built by buildSSA2LLVMCLI, and any t.TempDir() of this package under the run
// root, so the whole run is bounded and collected. Set SSA2LLVM_TEST_KEEP_TMP=1
// to keep the root for inspection; its path is printed in that case.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ssa2llvm-test-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create ssa2llvm test temp root failed: %v\n", err)
		os.Exit(1)
	}

	keep := os.Getenv("SSA2LLVM_TEST_KEEP_TMP") != ""
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if setErr := os.Setenv(key, root); setErr != nil {
			fmt.Fprintf(os.Stderr, "set %s failed: %v\n", key, setErr)
			os.Exit(1)
		}
	}

	code := m.Run()

	if keep {
		fmt.Fprintf(os.Stderr, "ssa2llvm test temp root kept at %s\n", root)
	} else if rmErr := os.RemoveAll(root); rmErr != nil {
		fmt.Fprintf(os.Stderr, "remove ssa2llvm test temp root %s failed: %v\n", root, rmErr)
	}
	os.Exit(code)
}
