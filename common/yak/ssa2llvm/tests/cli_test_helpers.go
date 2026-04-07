package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	ssa2llvmCLIOnce sync.Once
	ssa2llvmCLIPath string
	ssa2llvmCLIErr  error
)

type processResult struct {
	ExitCode int
	Output   string
}

// buildSSA2LLVMCLI builds the real shipping CLI once for the current test run.
// Acceptance tests should exercise this binary instead of calling compiler helpers directly.
func buildSSA2LLVMCLI(t *testing.T) string {
	t.Helper()

	repoRoot := RepoRoot(t)
	ssa2llvmCLIOnce.Do(func() {
		buildDir, err := os.MkdirTemp("", "ssa2llvm-cli-*")
		if err != nil {
			ssa2llvmCLIErr = fmt.Errorf("create cli build dir failed: %w", err)
			return
		}

		name := "ssa2llvm"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		ssa2llvmCLIPath = filepath.Join(buildDir, name)

		cmd := exec.Command("go", "build", "-o", ssa2llvmCLIPath, "./common/yak/ssa2llvm/cmd")
		cmd.Dir = repoRoot
		cmd.Env = append([]string{}, os.Environ()...)
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			ssa2llvmCLIErr = fmt.Errorf("build ssa2llvm cli failed: %v\n%s", err, output)
			return
		}
	})

	if ssa2llvmCLIErr != nil {
		t.Fatalf("%v", ssa2llvmCLIErr)
	}
	return ssa2llvmCLIPath
}

func runProcess(t *testing.T, bin string, env map[string]string, args ...string) processResult {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return processResult{ExitCode: 0, Output: string(output)}
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return processResult{ExitCode: exitErr.ExitCode(), Output: string(output)}
	}
	t.Fatalf("process %q failed: %v\nOutput: %s", bin, err, output)
	return processResult{}
}

func runSSA2LLVMCLI(t *testing.T, args ...string) processResult {
	t.Helper()

	if len(args) == 0 {
		t.Fatal("runSSA2LLVMCLI requires args")
	}
	// Real CLI compile/run tests should match ordinary user flows, which expect a usable runtime archive.
	// Force rebuild so acceptance tests never pick up stale cached artifacts after
	// compiler/runtime refactors.
	switch args[0] {
	case "compile", "run":
		ensureRuntimeArchiveOnce(t)
		args = append(args, "-a")
	}

	cliPath := buildSSA2LLVMCLI(t)
	yakitHome := filepath.Join(t.TempDir(), ".db")
	return runProcess(t, cliPath, map[string]string{
		"YAKIT_HOME": yakitHome,
	}, args...)
}

func writeYakSourceFile(t *testing.T, code string) string {
	t.Helper()

	src := filepath.Join(t.TempDir(), "input.yak")
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}
	return src
}
