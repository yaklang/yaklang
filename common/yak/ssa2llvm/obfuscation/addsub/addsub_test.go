package addsub_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	obftest "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

func TestAddSubRewriteAddSSAIR(t *testing.T) {
	code := `
check = () => {
	return 40 + 2
}
`

	beforeIR := obftest.CompileFunctionSSAString(t, code, "yak", "check", nil)
	require.Contains(t, beforeIR, "add(40, 2)")

	afterIR := obftest.CompileFunctionSSAString(t, code, "yak", "check", func(program *ssa.Program) error {
		return obfuscation.ApplySSA(program, []string{"addsub"})
	})
	require.Contains(t, afterIR, "sub(40,")
	require.Contains(t, afterIR, "add(2,")
	require.NotContains(t, afterIR, "add(40, 2)")
}

func TestAddSubRewriteSubSSAIR(t *testing.T) {
	code := `
check = () => {
	return 50 - 8
}
`

	beforeIR := obftest.CompileFunctionSSAString(t, code, "yak", "check", nil)
	require.Contains(t, beforeIR, "sub(50, 8)")

	afterIR := obftest.CompileFunctionSSAString(t, code, "yak", "check", func(program *ssa.Program) error {
		return obfuscation.ApplySSA(program, []string{"addsub"})
	})
	require.Contains(t, afterIR, "sub(add(50,")
	require.Contains(t, afterIR, "add(8,")
	require.NotContains(t, afterIR, "sub(50, 8)")
}
