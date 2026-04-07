package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

// CheckBinaryReturn compiles the code, runs it, and verifies the return value.
// This is an exported wrapper for cross-package obfuscation tests.
func CheckBinaryReturn(t *testing.T, code string, entry string, language string, expected int64) {
	t.Helper()
	checkBinaryEx(t, code, entry, language, expected)
}

// CheckBinaryReturnWithOpts compiles the code with the given obfuscators,
// runs it, and verifies the return value.
func CheckBinaryReturnWithOpts(t *testing.T, code string, entry string, language string, expected int64, obfuscators ...string) {
	t.Helper()
	checkBinaryExWithOptions(t, code, entry, language, expected, withCompileObfuscators(obfuscators...))
}

// CompileToIRWithOpts compiles code to LLVM IR with additional options.
func CompileToIRWithOpts(t *testing.T, code string, language string, opts ...compiler.CompileOption) string {
	t.Helper()
	return CompileLLVMIRString(t, code, language, opts...)
}
