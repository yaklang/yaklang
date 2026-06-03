package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteLLVMIRArtifactWritesOutputAndFinalCopy(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "cache", "probe.ll")
	finalFile := filepath.Join(tmpDir, "final", "probe.ll")
	cfg := &CompileConfig{
		OutputFile:        outputFile,
		FinalOutputFile:   finalFile,
		FinalOutputAuto:   false,
		EmitLLVM:          true,
		EntryFunctionName: "main",
	}

	out, err := writeLLVMIRArtifact(cfg, "; probe ir\n")
	require.NoError(t, err)
	require.Equal(t, outputFile, out)

	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	require.Equal(t, "; probe ir\n", string(data))

	finalData, err := os.ReadFile(finalFile)
	require.NoError(t, err)
	require.Equal(t, "; probe ir\n", string(finalData))
}

func TestLLVMOutputPathFallsBackToSourceOrWorkDir(t *testing.T) {
	tmpDir := t.TempDir()

	require.Equal(t,
		filepath.Join(tmpDir, "probe.ll"),
		llvmOutputPath(&CompileConfig{SourceFile: filepath.Join(tmpDir, "probe.yak")}),
	)
	require.Equal(t,
		filepath.Join(tmpDir, "ssa2llvm.ll"),
		llvmOutputPath(&CompileConfig{WorkDir: tmpDir}),
	)
}
