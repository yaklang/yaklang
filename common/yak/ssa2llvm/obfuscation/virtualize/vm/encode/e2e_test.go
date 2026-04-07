package encode_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/encode"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/executor"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/lowering"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/region"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestEndToEndProtectedPipeline tests the full pipeline:
// Yak source → SSA → Region select → PIR lower → Encode → Decode → Execute
func TestEndToEndProtectedPipeline(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
`
	// 1. Parse to SSA
	prog, err := ssaapi.Parse(code, ssaconfig.WithProjectLanguage(ssaconfig.Yak))
	require.NoError(t, err)

	// 2. Select region
	sel := &region.ByName{Names: []string{"add"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 1)

	// 3. Lower to PIR
	pirRegion, err := lowering.LowerRegion(candidates)
	require.NoError(t, err)
	require.Len(t, pirRegion.Functions, 1)
	t.Logf("PIR:\n%s", pirRegion.Functions[0].Dump())

	// 4. Encode with seed
	s, err := seed.Generate()
	require.NoError(t, err)
	blob, err := encode.Encode(pirRegion, s)
	require.NoError(t, err)
	t.Logf("Blob size: %d bytes", len(blob))

	// 5. Decode with same seed
	decoded, err := encode.Decode(blob, s)
	require.NoError(t, err)
	require.Len(t, decoded.Functions, 1)

	// 6. Execute
	result, err := executor.Execute(decoded.Functions[0], []int64{17, 25}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.Value)
}

// TestEndToEndWithBranch tests the full pipeline with control flow.
func TestEndToEndWithBranch(t *testing.T) {
	code := `
max = (a, b) => {
	if a > b {
		return a
	}
	return b
}
`
	prog, err := ssaapi.Parse(code, ssaconfig.WithProjectLanguage(ssaconfig.Yak))
	require.NoError(t, err)

	sel := &region.ByName{Names: []string{"max"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 1)

	pirRegion, err := lowering.LowerRegion(candidates)
	require.NoError(t, err)

	s, err := seed.Generate()
	require.NoError(t, err)
	blob, err := encode.Encode(pirRegion, s)
	require.NoError(t, err)

	decoded, err := encode.Decode(blob, s)
	require.NoError(t, err)

	// Test a > b case
	result, err := executor.Execute(decoded.Functions[0], []int64{10, 5}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(10), result.Value)
}
