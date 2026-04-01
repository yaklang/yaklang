package callret_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	obftest "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func requireNoOrdinaryYakCalls(t *testing.T, ir string, names ...string) {
	t.Helper()

	for _, name := range names {
		require.NotContains(t, ir, "@"+name+"(")
	}
}

func TestCallRetRemovesInternalFunctionsAndIntrinsics(t *testing.T) {
	code := `
one = () => { return 40 }
two = () => { return 2 }
check = () => {
	return one() + two()
}
`

	afterIR := obftest.CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileObfuscators("callret"),
	)

	requireNoOrdinaryYakCalls(t, afterIR, "one", "two")

	// LLVM-stage lowering replaces these intrinsic calls with alloca+load/store sequences.
	require.NotContains(t, afterIR, "@__yak_obf_vs_push")
	require.NotContains(t, afterIR, "@__yak_obf_vs_pop")
	require.NotContains(t, afterIR, "@__yak_obf_cs_push")
	require.NotContains(t, afterIR, "@__yak_obf_cs_pop")

	require.Contains(t, strings.ToLower(afterIR), "alloca")
}

func TestCallRetRemovesChainedInternalCalls(t *testing.T) {
	code := `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
top = () => { return mid() + leaf() }
check = () => {
	return top() + mid()
}
`

	afterIR := obftest.CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileObfuscators("callret"),
	)

	requireNoOrdinaryYakCalls(t, afterIR, "leaf", "mid", "top")
}

func TestCallRetRemovesClosureInternalCalls(t *testing.T) {
	code := `
makeAdder = (x) => {
	inner = (y) => {
		return x + y
	}
	return inner
}
check = () => {
	add5 = makeAdder(5)
	return add5(7)
}
`

	afterIR := obftest.CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileObfuscators("callret"),
	)

	requireNoOrdinaryYakCalls(t, afterIR, "makeAdder", "inner")
}
