package tests

import (
	"fmt"
	"io"
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
	return runProcessInDir(t, "", bin, env, args...)
}

func runProcessInDir(t *testing.T, dir, bin string, env map[string]string, args ...string) processResult {
	t.Helper()

	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
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
	return runSSA2LLVMCLIInDir(t, "", args...)
}

func runSSA2LLVMCLIInDir(t *testing.T, dir string, args ...string) processResult {
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
		if dir != "" {
			prepareRuntimeArchiveForDir(t, dir)
		}
		args = append(args, "-a")
	}

	cliPath := buildSSA2LLVMCLI(t)
	yakitHome := filepath.Join(t.TempDir(), ".db")
	return runProcessInDir(t, dir, cliPath, map[string]string{
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

func prepareRuntimeArchiveForDir(t *testing.T, dir string) {
	t.Helper()

	repoRoot := RepoRoot(t)
	srcRuntimeDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "runtime")
	dstRuntimeDir := filepath.Join(dir, "common", "yak", "ssa2llvm", "runtime")
	if _, err := os.Stat(dstRuntimeDir); err == nil {
		return
	}

	requireDir := filepath.Dir(dstRuntimeDir)
	if err := os.MkdirAll(requireDir, 0o755); err != nil {
		t.Fatalf("prepare runtime archive dir failed: %v", err)
	}

	if err := os.Symlink(srcRuntimeDir, dstRuntimeDir); err == nil {
		return
	}

	if err := os.MkdirAll(dstRuntimeDir, 0o755); err != nil {
		t.Fatalf("prepare mirrored runtime dir failed: %v", err)
	}

	srcArchive := filepath.Join(srcRuntimeDir, "libyak.a")
	dstArchive := filepath.Join(dstRuntimeDir, "libyak.a")
	src, err := os.Open(srcArchive)
	if err != nil {
		t.Fatalf("open runtime archive failed: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstArchive)
	if err != nil {
		t.Fatalf("create mirrored runtime archive failed: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("copy mirrored runtime archive failed: %v", err)
	}

	srcRuntimeGo := filepath.Join(srcRuntimeDir, "runtime_go")
	dstRuntimeGo := filepath.Join(dstRuntimeDir, "runtime_go")
	if err := os.Symlink(srcRuntimeGo, dstRuntimeGo); err != nil && !os.IsExist(err) {
		t.Fatalf("mirror runtime_go dir failed: %v", err)
	}

	srcLinkFlags := filepath.Join(srcRuntimeDir, "libyak.linkflags")
	if _, err := os.Stat(srcLinkFlags); err == nil {
		dstLinkFlags := filepath.Join(dstRuntimeDir, "libyak.linkflags")
		data, readErr := os.ReadFile(srcLinkFlags)
		if readErr != nil {
			t.Fatalf("read runtime link flags failed: %v", readErr)
		}
		if writeErr := os.WriteFile(dstLinkFlags, data, 0o644); writeErr != nil {
			t.Fatalf("write runtime link flags failed: %v", writeErr)
		}
	}
}
