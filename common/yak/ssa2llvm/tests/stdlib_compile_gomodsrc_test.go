package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

func TestStdlibCompileFromGomodsrcTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ssa2llvm native runtime build/link is not supported on windows in this test")
	}

	repoRoot := RepoRoot(t)

	tmpRoot := t.TempDir()
	srcDir := filepath.Join(tmpRoot, "ssa2llvm-runtime-src")
	buildDir := filepath.Join(tmpRoot, "build")
	require.NoError(t, os.MkdirAll(buildDir, 0o755))

	// Generate a pruned source tree from the current module (the same way build_runtime_embed.sh does).
	cmd := exec.Command("go", "run", "./common/utils/gomodsrc/cmd",
		"--pkg", "./common/yak/ssa2llvm/runtime/runtime_go",
		"--dst", srcDir,
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "gomodsrc failed: %v\n%s", err, out)

	// Provide libgc.a to the runtime_go build (it links with -L${SRCDIR}/libs -lgc).
	libgcPath, err := findLibgcArchive()
	require.NoError(t, err)
	gcDstDir := filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go", "libs")
	require.NoError(t, os.MkdirAll(gcDstDir, 0o755))
	require.NoError(t, copyFileBytes(libgcPath, filepath.Join(gcDstDir, "libgc.a")))

	archivePath, gcLibDir, err := embed.BuildRuntimeArchiveFromSourceTree(buildDir, srcDir)
	require.NoError(t, err)

	// Compile a known-good yak program using this runtime archive and run it.
	exampleYak := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "testdata", "example.yak")
	codeBytes, err := os.ReadFile(exampleYak)
	require.NoError(t, err)

	tmpYak := filepath.Join(tmpRoot, "example.yak")
	require.NoError(t, os.WriteFile(tmpYak, codeBytes, 0o644))

	outBin := filepath.Join(tmpRoot, "example.bin")
	_, err = compiler.CompileToExecutable(
		compiler.WithCompileWorkDir(buildDir),
		compiler.WithCompileSourceFile(tmpYak),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEntryFunction("check"),
		compiler.WithCompileOutputFile(outBin),
		compiler.WithCompileRuntimeArchive(archivePath),
		compiler.WithCompileExtraLinkArgs("-L"+gcLibDir),
	)
	require.NoError(t, err)

	run := exec.Command(outBin)
	runOut, runErr := run.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected non-zero exit code, got 0; output=%q", string(runOut))
	}
	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("run failed: %v\noutput=%s", runErr, runOut)
	}
	// example.yak returns fib(8)+factorial(5)+sumRange(1,10) = 21+120+55 = 196
	require.Equalf(t, 196, exitErr.ExitCode(), "unexpected exit code; output=%q", string(runOut))
}

func findLibgcArchive() (string, error) {
	tools := []string{"cc", "gcc", "clang"}
	var lastErr error
	for _, tool := range tools {
		p, err := exec.LookPath(tool)
		if err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(p, "-print-file-name=libgc.a")
		out, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("%s -print-file-name failed: %v\n%s", tool, err, out)
			continue
		}
		path := strings.TrimSpace(string(out))
		if path == "" || path == "libgc.a" {
			lastErr = fmt.Errorf("%s did not resolve libgc.a: %q", tool, path)
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
		lastErr = fmt.Errorf("libgc.a not found at %q", path)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("libgc.a not found")
	}
	return "", lastErr
}

func copyFileBytes(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
