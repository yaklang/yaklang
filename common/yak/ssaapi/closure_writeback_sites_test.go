package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestClosureWritebackSitesSurviveFinish guards the data the AOT compiler needs
// to write a captured scalar back to its free-value cell.
//
// Finish releases SideEffectsReturn because its Variable keys keep the body
// scope alive, which matters for long batch scans. A backend that runs after
// Finish (ssa2llvm) still has to know which instruction carried the write, so
// SetReturnSideEffects keeps a scope-free copy in SideEffectReturnSites. When
// that copy was missing, `retry(100, () => { count++ })` compiled to a binary
// that left count at 0.
func TestClosureWritebackSitesSurviveFinish(t *testing.T) {
	code := `
count = 0
retry(100, () => {
    count++
    if count > 3 {
        return false
    }
    return true
})
println(count)
`
	prog, err := Parse(code, WithLanguage(ssaconfig.Yak))
	require.NoError(t, err)

	var closure *ssa.Function
	prog.Program.EachFunction(func(fn *ssa.Function) {
		if fn == nil || len(fn.SideEffectReturnSites) == 0 {
			return
		}
		if fn.GetParent() != nil {
			closure = fn
		}
	})
	require.NotNil(t, closure, "the closure writing the captured count must record a writeback site")
	require.True(t, closure.IsFinished(), "the site list has to be readable after Finish")
	require.Empty(t, closure.SideEffectsReturn,
		"Finish must still release the scope-retaining maps")

	var site *ssa.FunctionSideEffectSite
	for i := range closure.SideEffectReturnSites {
		if closure.SideEffectReturnSites[i].Name == "count" {
			site = &closure.SideEffectReturnSites[i]
			break
		}
	}
	require.NotNil(t, site, "the captured count write must be recorded")
	require.Equal(t, "count", site.VariableName)
	require.Equal(t, ssa.NormalSideEffect, site.Kind)
	require.Greater(t, site.Modify, int64(0))
	_, ok := closure.GetValueById(site.Modify)
	require.True(t, ok, "the recorded modify must still resolve to a live instruction")
}
