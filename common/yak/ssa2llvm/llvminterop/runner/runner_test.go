package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

func TestParseVersion(t *testing.T) {
	output := `LLVM (http://llvm.org/):
  LLVM version 17.0.6
  Optimizations enabled`

	info := &VersionInfo{}
	err := parseVersion(output, info)
	require.NoError(t, err)
	require.Equal(t, 17, info.Major)
	require.Equal(t, 0, info.Minor)
	require.Equal(t, 6, info.Patch)
}

func TestParseVersion_OlderFormat(t *testing.T) {
	output := `LLVM version 14.0.0`

	info := &VersionInfo{}
	err := parseVersion(output, info)
	require.NoError(t, err)
	require.Equal(t, 14, info.Major)
}

func TestParseVersion_Invalid(t *testing.T) {
	info := &VersionInfo{}
	err := parseVersion("no version here", info)
	require.Error(t, err)
}

func TestDetectCapabilities(t *testing.T) {
	tests := []struct {
		major              int
		expectNewPM        bool
		expectLegacyPM     bool
		expectLoadPassPlug bool
	}{
		{12, false, true, false},
		{13, true, true, true},
		{15, true, true, true},
		{17, true, false, true},
		{18, true, false, true},
	}

	for _, tt := range tests {
		v := &VersionInfo{Major: tt.major}
		caps := DetectCapabilities(v)
		require.Equal(t, tt.expectNewPM, caps.HasNewPM, "major=%d NewPM", tt.major)
		require.Equal(t, tt.expectLegacyPM, caps.HasLegacyPM, "major=%d LegacyPM", tt.major)
		require.Equal(t, tt.expectLoadPassPlug, caps.SupportsLoadPassPlugin, "major=%d LoadPassPlugin", tt.major)
	}
}

func TestDetectCapabilities_Nil(t *testing.T) {
	caps := DetectCapabilities(nil)
	require.False(t, caps.HasNewPM)
	require.False(t, caps.HasLegacyPM)
}

func TestRunNilConfig(t *testing.T) {
	_, err := Run(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil config")
}

func TestRunNilPlugin(t *testing.T) {
	_, err := Run(&Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil plugin")
}

func TestRunMissingInput(t *testing.T) {
	desc := plugin.Descriptor{Name: "test", Kind: plugin.KindNewPM, Path: "/nonexistent.so"}
	_, err := Run(&Config{
		Plugin:     &desc,
		InputFile:  "/tmp/nonexistent_input.ll",
		OutputFile: "/tmp/out.ll",
	})
	require.Error(t, err)
}

// TestResolveLLVMToolBinaryRejectsExplicitMissingPath keeps the failure message
// precise when the caller names a path: probing the versioned install
// directories would silently pick a different toolchain than the one asked for.
func TestResolveLLVMToolBinaryRejectsExplicitMissingPath(t *testing.T) {
	_, err := resolveLLVMToolBinary("/nonexistent/llvm/bin/opt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "/nonexistent/llvm/bin/opt")
}

// TestResolveLLVMToolBinaryFallsBackToVersionedInstall guards the CI failure where
// llvm-18-dev installs opt under /usr/lib/llvm-18/bin, a directory that is not
// on PATH on a plain runner, so a PATH-only lookup reported a missing toolchain.
// The probe name is unique, so PATH can never satisfy it and the fallback
// directory has to be consulted.
func TestResolveLLVMToolBinaryFallsBackToVersionedInstall(t *testing.T) {
	dir := t.TempDir()
	const probe = "opt-yak-ci-probe"
	binPath := filepath.Join(dir, probe)
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	original := versionedLLVMInstallDirs
	versionedLLVMInstallDirs = []string{dir}
	t.Cleanup(func() { versionedLLVMInstallDirs = original })

	resolved, err := resolveLLVMToolBinary(probe)
	require.NoError(t, err)
	require.Equal(t, binPath, resolved)
}

// TestResolveLLVMToolBinarySkipsNonExecutableCandidate keeps a stale, unreadable
// entry in a versioned directory from being selected.
func TestResolveLLVMToolBinarySkipsNonExecutableCandidate(t *testing.T) {
	dir := t.TempDir()
	const probe = "opt-yak-ci-probe-noexec"
	binPath := filepath.Join(dir, probe)
	require.NoError(t, os.WriteFile(binPath, []byte("not executable"), 0o644))

	original := versionedLLVMInstallDirs
	versionedLLVMInstallDirs = []string{dir}
	t.Cleanup(func() { versionedLLVMInstallDirs = original })

	_, err := resolveLLVMToolBinary(probe)
	require.Error(t, err)
}

// TestRunOptResolvesThroughVersionedDirs guards the second half of the CI
// failure where `opt` was found by version detection but the actual run still
// executed a bare "opt" name, so the job died with
// `exec: "opt": executable file not found in $PATH`. A fake opt records that it
// ran, proving runOpt goes through resolveLLVMToolBinary.
func TestRunOptResolvesThroughVersionedDirs(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ran.log")
	script := "#!/bin/sh\necho \"$@\" > " + logPath + "\nexit 0\n"
	binPath := filepath.Join(dir, "opt-yak-run-probe")
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	original := versionedLLVMInstallDirs
	versionedLLVMInstallDirs = []string{dir}
	t.Cleanup(func() { versionedLLVMInstallDirs = original })

	inputPath := filepath.Join(dir, "in.ll")
	require.NoError(t, os.WriteFile(inputPath, []byte("; module\n"), 0o644))
	outputPath := filepath.Join(dir, "out.ll")

	desc := plugin.Descriptor{Name: "test", Kind: plugin.KindNewPM, Path: "/nonexistent.so", Passes: []string{"instcombine"}}
	_, err := Run(&Config{
		OptBinary:  "opt-yak-run-probe",
		Plugin:     &desc,
		InputFile:  inputPath,
		OutputFile: outputPath,
	})
	require.NoError(t, err)

	ran, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr, "opt was never executed")
	require.Contains(t, string(ran), "instcombine")
}

// TestRunToolResolvesThroughVersionedDirs guards the builtin tool pack path.
// instcombine-simplifycfg is declared as a KindTool plugin with Path "opt", so
// it goes through runTool/executeCommand instead of runOpt and used to fail with
// the very same "not found in $PATH" error after the pass-plugin path was fixed.
func TestRunToolResolvesThroughVersionedDirs(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tool-ran.log")
	script := "#!/bin/sh\necho \"$@\" > " + logPath + "\nexit 0\n"
	const probe = "opt-yak-tool-probe"
	binPath := filepath.Join(dir, probe)
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	original := versionedLLVMInstallDirs
	versionedLLVMInstallDirs = []string{dir}
	t.Cleanup(func() { versionedLLVMInstallDirs = original })

	inputPath := filepath.Join(dir, "in.ll")
	require.NoError(t, os.WriteFile(inputPath, []byte("; module\n"), 0o644))
	outputPath := filepath.Join(dir, "out.ll")

	desc := plugin.Descriptor{Name: "tool", Kind: plugin.KindTool, Path: probe, Args: []string{"-S", "-passes=instcombine"}}
	_, err := Run(&Config{
		Plugin:     &desc,
		InputFile:  inputPath,
		OutputFile: outputPath,
	})
	require.NoError(t, err)

	ran, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr, "tool binary was never executed")
	require.Contains(t, string(ran), "instcombine")
}
