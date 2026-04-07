package lowering_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/lowering"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/region"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func parseAndSelect(t *testing.T, code string, names []string) []region.Candidate {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaconfig.WithProjectLanguage(ssaconfig.Yak))
	require.NoError(t, err)
	sel := &region.ByName{Names: names}
	candidates := sel.Select(prog.Program)
	require.NotEmpty(t, candidates, "expected at least one candidate for names=%v", names)
	return candidates
}

func TestLowerSimpleAdd(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
`
	candidates := parseAndSelect(t, code, []string{"add"})
	pirFunc, hostSyms, err := lowering.LowerFunction(candidates[0].Func)
	require.NoError(t, err)
	require.NotNil(t, pirFunc)
	require.Equal(t, "add", pirFunc.Name)
	require.Equal(t, 2, pirFunc.NumArgs)
	require.True(t, pirFunc.NumRegs >= 3, "should have at least 3 regs (2 args + 1 result)")
	require.NotEmpty(t, pirFunc.Blocks)

	_ = hostSyms

	dump := pirFunc.Dump()
	t.Logf("PIR dump:\n%s", dump)
	require.Contains(t, dump, "arg 0")
	require.Contains(t, dump, "arg 1")
	require.Contains(t, dump, "add")
	require.Contains(t, dump, "ret")
}

func TestLowerWithBranch(t *testing.T) {
	code := `
max = (a, b) => {
	if a > b {
		return a
	}
	return b
}
`
	candidates := parseAndSelect(t, code, []string{"max"})
	pirFunc, _, err := lowering.LowerFunction(candidates[0].Func)
	require.NoError(t, err)
	require.NotNil(t, pirFunc)
	require.Equal(t, "max", pirFunc.Name)
	require.True(t, len(pirFunc.Blocks) >= 2, "branch should produce multiple blocks")

	dump := pirFunc.Dump()
	t.Logf("PIR dump:\n%s", dump)
	require.Contains(t, dump, "gt")
}

func TestLowerWithCall(t *testing.T) {
	code := `
helper = () => { return 42 }
caller = () => { return helper() }
`
	candidates := parseAndSelect(t, code, []string{"caller"})
	pirFunc, _, err := lowering.LowerFunction(candidates[0].Func)
	require.NoError(t, err)
	require.NotNil(t, pirFunc)

	dump := pirFunc.Dump()
	t.Logf("PIR dump:\n%s", dump)
	require.Contains(t, dump, "hostcall")
}

func TestLowerRegion(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
mul = (a, b) => { return a * b }
`
	candidates := parseAndSelect(t, code, []string{"add", "mul"})
	rgn, err := lowering.LowerRegion(candidates)
	require.NoError(t, err)
	require.NotNil(t, rgn)
	require.Len(t, rgn.Functions, 2)

	names := map[string]bool{}
	for _, f := range rgn.Functions {
		names[f.Name] = true
	}
	require.True(t, names["add"])
	require.True(t, names["mul"])
}

func TestLowerConstant(t *testing.T) {
	code := `
answer = () => { return 42 }
`
	candidates := parseAndSelect(t, code, []string{"answer"})
	pirFunc, _, err := lowering.LowerFunction(candidates[0].Func)
	require.NoError(t, err)

	dump := pirFunc.Dump()
	t.Logf("PIR dump:\n%s", dump)
	require.Contains(t, dump, "const 42")
	require.Contains(t, dump, "ret")
}

func TestLowerArithmetic(t *testing.T) {
	code := `
calc = (a, b) => {
	c = a + b
	d = a - b
	e = c * d
	return e
}
`
	candidates := parseAndSelect(t, code, []string{"calc"})
	pirFunc, _, err := lowering.LowerFunction(candidates[0].Func)
	require.NoError(t, err)

	dump := pirFunc.Dump()
	t.Logf("PIR dump:\n%s", dump)

	// Should contain all arithmetic ops
	hasOps := map[pir.Opcode]bool{}
	for _, blk := range pirFunc.Blocks {
		for _, inst := range blk.Insts {
			hasOps[inst.Op] = true
		}
	}
	require.True(t, hasOps[pir.OpAdd], "should have add")
	require.True(t, hasOps[pir.OpSub], "should have sub")
	require.True(t, hasOps[pir.OpMul], "should have mul")
}
