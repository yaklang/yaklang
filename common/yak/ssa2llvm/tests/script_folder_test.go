package tests

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestScriptFolder_CompileAndRun compiles every *.yak under tests/script through
// the real shipping ssa2llvm CLI, then runs the produced native binary. This
// exercises the full user-facing flow (compile -> executable -> run) rather
// than the compiler package API directly, and verifies the binary runs
// correctly with no external runtime dependencies.
func TestScriptFolder_CompileAndRun(t *testing.T) {
	repoRoot := RepoRoot(t)
	scriptDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "script")
	scripts := ListYakScripts(t, scriptDir)
	if len(scripts) == 0 {
		t.Fatalf("no yak scripts found under %s", scriptDir)
	}

	for _, script := range scripts {
		script := script
		name := strings.TrimSuffix(filepath.Base(script), filepath.Ext(script))
		t.Run(name, func(t *testing.T) {
			// Compile + run via the real CLI. Exit code 0 and a non-empty
			// (or empty, for pure-side-effect scripts) output are the success
			// criteria; the CLI helper asserts both.
			_ = RunYakScriptFileWithCLI(t, script, nil)
		})
	}
}
