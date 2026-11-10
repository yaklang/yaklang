package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestSFCheckExcludePrunePatternAndNode(t *testing.T) {
	prog, err := Parse(`
package demo;
class C {
    static String escapeHtml(String s) { return s; }
    void f(String name) {
        String safe = escapeHtml(name);
        g(name);
    }
    void g(String s) {}
}
`, WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`*?{opcode: call} as $call`)
	require.NoError(t, err)
	calls := res.GetValues("call")
	require.NotEmpty(t, calls)
	var call, callee *Value
	for _, candidate := range calls {
		c := candidate.GetCallee()
		if c == nil {
			t.Logf("call %q has no callee", candidate.String())
			continue
		}
		if strings.Contains(c.GetName(), "escapeHtml") {
			call, callee = candidate, c
			break
		}
	}
	require.NotNil(t, call, "the escapeHtml call must be found")
	require.NotNil(t, callee)

	config := sf.NewConfig()
	contextResult := sf.NewSFResult(nil, config)
	contextResult.SymbolTable.Set("filter", ToSFVMValues(Values{callee}))

	check := CreateCheck(contextResult, config)
	check.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Exclude),
		Value:          "*<getCallee> & $filter",
		SyntaxFlowRule: true,
	})
	require.True(t, check.ExcludePrunable(), "the simple getCallee exclude must enable branch pruning")
	require.False(t, check.CheckExcludeNode(callee), "a non-call node cannot match *<getCallee>")
	require.True(t, check.CheckExcludeNode(call), "a call whose callee is in $filter must prune the branch")
	require.Len(t, check.matchItem, 1)
	require.Nil(t, check.matchItem[0].frame, "a simple exclude must not compile a sub-rule frame")
	require.False(t, check.CheckMatch(sf.Values{call}), "the fast path must match the filter call on the path")
	require.True(t, check.CheckMatch(sf.Values{callee}), "a path without the filter call must not be excluded")

	unsupported := CreateCheck(contextResult, config)
	unsupported.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Exclude),
		Value:          "* & $filter",
		SyntaxFlowRule: true,
	})
	require.False(t, unsupported.ExcludePrunable(), "non-monotone/unknown exclude shapes must keep the full walk")
}

func TestSimpleIncludeSourceIDs(t *testing.T) {
	prog := NewTmpProgram("sf-dataflow-simple-include")
	source := fastMatchTestValue(t, prog, 7)

	config := sf.NewConfig()
	result := sf.NewSFResult(nil, config)
	result.SymbolTable.Set("source", sf.Values{source})

	check := CreateCheck(result, config)
	check.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Include),
		Value:          "* & $source",
		SyntaxFlowRule: true,
	})
	require.Len(t, check.matchItem, 1)
	require.Nil(t, check.matchItem[0].frame, "a simple include must not compile a sub-rule frame")
	require.True(t, check.CheckMatch(sf.Values{source}))
	require.False(t, check.CheckMatch(sf.Values{fastMatchTestValue(t, prog, 99)}))

	complexCheck := CreateCheck(result, config)
	complexCheck.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Include),
		Value:          "* & $source as $alias",
		SyntaxFlowRule: true,
	})
	require.NotNil(t, complexCheck.matchItem[0].frame, "include shapes with aliases must keep the full sub-query")
}
