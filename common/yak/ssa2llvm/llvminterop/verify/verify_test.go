package verify

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckOutputFile_Valid(t *testing.T) {
	tmp := t.TempDir() + "/test.ll"
	require.NoError(t, os.WriteFile(tmp, []byte("define i64 @foo() { ret i64 0 }"), 0644))

	result := CheckOutputFile(tmp)
	require.True(t, result.Valid)
	require.Empty(t, result.Errors)
}

func TestCheckOutputFile_NotFound(t *testing.T) {
	result := CheckOutputFile("/tmp/nonexistent_verify_test.ll")
	require.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
}

func TestCheckOutputFile_Empty(t *testing.T) {
	tmp := t.TempDir() + "/empty.ll"
	require.NoError(t, os.WriteFile(tmp, []byte{}, 0644))

	result := CheckOutputFile(tmp)
	require.False(t, result.Valid)
}

func TestCheckIRValidity_Valid(t *testing.T) {
	ir := `define i64 @check() {
entry:
  ret i64 42
}`
	result := CheckIRValidity(ir)
	require.True(t, result.Valid)
}

func TestCheckIRValidity_Empty(t *testing.T) {
	result := CheckIRValidity("")
	require.False(t, result.Valid)
}

func TestCheckIRValidity_NoFunctions(t *testing.T) {
	result := CheckIRValidity("target triple = \"x86_64-pc-linux-gnu\"")
	require.False(t, result.Valid)
}

func TestDiagnoseFailure(t *testing.T) {
	diags := DiagnoseFailure("LLVM ERROR: something broke\n", 1)
	require.NotEmpty(t, diags)

	found := false
	for _, d := range diags {
		if d == "LLVM ERROR: something broke" {
			found = true
		}
	}
	require.True(t, found)
}

func TestDiagnoseFailure_PluginLoad(t *testing.T) {
	diags := DiagnoseFailure("Cannot register pass\n", 1)
	require.NotEmpty(t, diags)
}

func TestDiagnoseFailure_Assertion(t *testing.T) {
	diags := DiagnoseFailure("Assertion `x != nullptr' failed\n", 134)
	found := false
	for _, d := range diags {
		if d == "LLVM internal assertion failure (likely plugin incompatibility)" {
			found = true
		}
	}
	require.True(t, found)
}

func TestDiagnoseFailure_Success(t *testing.T) {
	diags := DiagnoseFailure("", 0)
	require.Empty(t, diags)
}
