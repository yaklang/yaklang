package region_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/region"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func parseYakProgram(t *testing.T, code string) *ssaapi.Program {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaconfig.WithProjectLanguage(ssaconfig.Yak))
	require.NoError(t, err, "SSA parse failed")
	require.NotNil(t, prog)
	return prog
}

func TestByNameSelector(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
mul = (a, b) => { return a * b }
`
	prog := parseYakProgram(t, code)
	sel := &region.ByName{Names: []string{"add"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 1)
	require.Equal(t, "add", candidates[0].Name)
	require.Equal(t, "explicit", candidates[0].Reason)
}

func TestByNameSelectorMultiple(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
mul = (a, b) => { return a * b }
`
	prog := parseYakProgram(t, code)
	sel := &region.ByName{Names: []string{"add", "mul"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 2)
	names := map[string]bool{}
	for _, c := range candidates {
		names[c.Name] = true
	}
	require.True(t, names["add"])
	require.True(t, names["mul"])
}

func TestByNameSelectorNotFound(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
`
	prog := parseYakProgram(t, code)
	sel := &region.ByName{Names: []string{"nonexistent"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 0)
}

func TestAllSelector(t *testing.T) {
	code := `
add = (a, b) => { return a + b }
mul = (a, b) => { return a * b }
`
	prog := parseYakProgram(t, code)
	sel := &region.All{ExcludeEntry: true}
	candidates := sel.Select(prog.Program)
	// Should find at least add and mul (entry excluded)
	found := map[string]bool{}
	for _, c := range candidates {
		found[c.Name] = true
	}
	require.True(t, found["add"], "should find add")
	require.True(t, found["mul"], "should find mul")
	require.False(t, found["yak_internal_atmain"], "should exclude entry")
}

func TestIsLowerable(t *testing.T) {
	code := `
simple = (a, b) => { return a + b }
`
	prog := parseYakProgram(t, code)
	sel := &region.ByName{Names: []string{"simple"}}
	candidates := sel.Select(prog.Program)
	require.Len(t, candidates, 1)
	require.True(t, region.IsLowerable(candidates[0].Func),
		"simple arithmetic function should be lowerable")
}
