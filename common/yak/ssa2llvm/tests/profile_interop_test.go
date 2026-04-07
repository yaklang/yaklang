package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/pack"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

func TestCompileProfileLiteEmitLLVM(t *testing.T) {
	code := `
leaf = () => { return 20 }
check = () => { return leaf() + 22 }
`
	ir := CompileLLVMIRString(t, code, "yak",
		compiler.WithCompileEntryFunction("check"),
		compiler.WithCompileProfile("resilience-lite"),
	)
	require.Contains(t, ir, "%obf_vs_sp")
	require.Contains(t, ir, "%obf_cs_sp")
}

func TestCompileProfileHybridEmitLLVMWithoutProtectedSelection(t *testing.T) {
	_, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(`check = () => { return 42 }`),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(filepath.Join(t.TempDir(), "out.ll")),
		compiler.WithCompileProfile("resilience-hybrid"),
	)
	require.NoError(t, err)
}

func TestCompileWithLLVMToolInteropEmitLLVM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell adapter test is unix-only")
	}

	tmpDir := t.TempDir()
	toolPath := filepath.Join(tmpDir, "tool.sh")
	require.NoError(t, os.WriteFile(toolPath, []byte(`#!/bin/sh
out=""
in=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      in="$1"
      shift
      ;;
  esac
done
cat "$in" > "$out"
printf '\n; llvm interop touched\n' >> "$out"
`), 0o755))

	outFile := filepath.Join(tmpDir, "out.ll")
	_, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(`check = () => { return 42 }`),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(outFile),
		compiler.WithCompileLLVMPlugin(toolPath),
		compiler.WithCompileLLVMPluginKind("tool"),
	)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	require.Contains(t, string(data), "llvm interop touched")
}

func TestCompileWithLLVMPackManifestEmitLLVM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell adapter test is unix-only")
	}

	tmpDir := t.TempDir()
	toolPath := filepath.Join(tmpDir, "tool.sh")
	require.NoError(t, os.WriteFile(toolPath, []byte(`#!/bin/sh
out=""
in=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      in="$1"
      shift
      ;;
  esac
done
cat "$in" > "$out"
printf '\n; pack touched\n' >> "$out"
`), 0o755))

	manifest := pack.Manifest{
		Name:           "test-pack",
		LLVMVersionMin: 1,
		Plugins: []plugin.Descriptor{
			{
				Name: "append-comment",
				Kind: plugin.KindTool,
				Path: toolPath,
			},
		},
	}
	manifestPath := filepath.Join(tmpDir, "pack.json")
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, data, 0o644))

	outFile := filepath.Join(tmpDir, "out.ll")
	_, err = compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(`check = () => { return 7 }`),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(outFile),
		compiler.WithCompileLLVMPack(manifestPath),
	)
	require.NoError(t, err)

	finalData, err := os.ReadFile(outFile)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(finalData), "pack touched"))
}

func TestCompileWithBuiltinLLVMPackByName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opt-based builtin pack test is unix-only in this environment")
	}

	outFile := filepath.Join(t.TempDir(), "out.ll")
	_, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(`check = () => { return 7 }`),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(outFile),
		compiler.WithCompileLLVMPack("instcombine-simplifycfg"),
	)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	require.Contains(t, string(data), "define")
}
