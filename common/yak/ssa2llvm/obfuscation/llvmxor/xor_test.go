package llvmxor_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	obftest "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func TestLLVMXorRewriteAddIR(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`

	beforeIR := obftest.CompileLLVMIRString(t, code, "yak")

	afterIR := obftest.CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileLLVMObfuscators("xor"),
	)
	require.Greater(t, strings.Count(afterIR, " xor i64 "), strings.Count(beforeIR, " xor i64 "))
	require.Greater(t, strings.Count(afterIR, " and i64 "), strings.Count(beforeIR, " and i64 "))
	require.Greater(t, strings.Count(afterIR, " shl i64 "), strings.Count(beforeIR, " shl i64 "))
}

func TestLLVMXorRewriteSubIR(t *testing.T) {
	code := `
three = () => { return 50 }
four = () => { return 8 }
check = () => {
	return three() - four()
}
`

	beforeIR := obftest.CompileLLVMIRString(t, code, "yak")

	afterIR := obftest.CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileLLVMObfuscators("xor"),
	)
	require.Greater(t, strings.Count(afterIR, " xor i64 "), strings.Count(beforeIR, " xor i64 "))
	require.Greater(t, strings.Count(afterIR, " and i64 "), strings.Count(beforeIR, " and i64 "))
	require.Greater(t, strings.Count(afterIR, " shl i64 "), strings.Count(beforeIR, " shl i64 "))
}
