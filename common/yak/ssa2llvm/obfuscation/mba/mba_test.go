package mba_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	obftest "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func TestMBABasicAdd(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`
	ir := obftest.CompileLLVMIRString(
		t, code, "yak",
		compiler.WithCompileObfuscators("mba"),
	)
	require.NotEmpty(t, ir)

	// MBA rewrites add into and+or, so the IR should contain MBA markers.
	require.True(t,
		strings.Contains(ir, "mba_") || strings.Contains(ir, "_and") || strings.Contains(ir, "_or"),
		"IR should contain MBA transformation artifacts",
	)
}

func TestMBABasicSub(t *testing.T) {
	// Use function calls to prevent constant folding.
	code := `
a = () => { return 50 }
b = () => { return 8 }
check = () => {
	return a() - b()
}
`
	ir := obftest.CompileLLVMIRString(
		t, code, "yak",
		compiler.WithCompileObfuscators("mba"),
	)
	require.NotEmpty(t, ir)
	require.True(t,
		strings.Contains(ir, "mba_"),
		"IR should contain MBA sub transformation artifacts",
	)
}
