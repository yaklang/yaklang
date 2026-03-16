package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestCompileCached_InRepo_CacheHit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ssa2llvm native runtime build/link is not supported on windows in this test")
	}

	repoRoot := RepoRoot(t)
	EnsureRuntimeArchive(t, repoRoot)

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	srcFile := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "testdata", "example.yak")
	res1, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceFile(srcFile),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEntryFunction("check"),
		compiler.WithCompileCacheEnabled(true),
		compiler.WithCompileForceRebuild(true),
	)
	require.NoError(t, err)
	require.False(t, res1.CacheHit)
	require.FileExists(t, res1.Artifact)

	cmd := exec.Command(res1.Artifact)
	out, runErr := cmd.CombinedOutput()
	require.Error(t, runErr, "expected non-zero exit code, got 0; output=%q", string(out))
	exitErr, ok := runErr.(*exec.ExitError)
	require.True(t, ok, "expected exec.ExitError, got %T; output=%q", runErr, string(out))
	require.Equal(t, 196, exitErr.ExitCode(), "unexpected exit code; output=%q", string(out))

	res2, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceFile(srcFile),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEntryFunction("check"),
		compiler.WithCompileCacheEnabled(true),
		compiler.WithCompileForceRebuild(false),
	)
	require.NoError(t, err)
	require.True(t, res2.CacheHit)
	require.Equal(t, res1.WorkDir, res2.WorkDir)
	require.Equal(t, res1.Artifact, res2.Artifact)
}
