package tests

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestScriptFolder_CompileAndRun(t *testing.T) {
	repoRoot := RepoRoot(t)
	scriptDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "script")
	scripts := ListYakScripts(t, scriptDir)
	if len(scripts) == 0 {
		t.Fatalf("no yak scripts found under %s", scriptDir)
	}

	EnsureRuntimeArchive(t, repoRoot)

	tmpDir := t.TempDir()
	for _, script := range scripts {
		script := script
		name := strings.TrimSuffix(filepath.Base(script), filepath.Ext(script))
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(tmpDir, name)

			if _, err := compiler.CompileToExecutable(
				compiler.WithCompileSourceFile(script),
				compiler.WithCompileLanguage("yak"),
				compiler.WithCompileOutputFile(out),
			); err != nil {
				t.Fatalf("compile failed: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, out)
			output, runErr := cmd.CombinedOutput()
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("execution timed out: %s", output)
			}
			if runErr != nil {
				t.Fatalf("execution failed: %v\n%s", runErr, output)
			}
		})
	}
}
