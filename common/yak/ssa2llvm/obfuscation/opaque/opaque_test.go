package opaque_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	obftest "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func TestOpaquePredicateIR(t *testing.T) {
	// Use code with control flow that generates multiple basic blocks.
	code := `
check = () => {
	a = 40 + 2
	b = 50 - 8
	if a > b {
		return a
	}
	return b
}
`
	ir := obftest.CompileLLVMIRString(
		t, code, "yak",
		compiler.WithCompileObfuscators("opaque"),
	)
	require.NotEmpty(t, ir)

	// Functions with control flow should have opaque predicate artifacts.
	require.True(t,
		strings.Contains(ir, "opaque_cond") || strings.Contains(ir, "opaque_bogus") || strings.Contains(ir, "opaque_sq"),
		"IR with branches should contain opaque predicate markers",
	)
}

func TestOpaquePredicateIRSingleBlock(t *testing.T) {
	// Single-block functions have no branches to replace.
	code := `
check = () => {
	return 42
}
`
	ir := obftest.CompileLLVMIRString(
		t, code, "yak",
		compiler.WithCompileObfuscators("opaque"),
	)
	require.NotEmpty(t, ir)
	// Pass should succeed without error even if nothing to transform.
}
