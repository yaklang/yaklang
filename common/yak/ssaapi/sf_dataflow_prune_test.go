package ssaapi

import (
	"context"
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

	unsupported := CreateCheck(contextResult, config)
	unsupported.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Exclude),
		Value:          "* & $filter",
		SyntaxFlowRule: true,
	})
	require.False(t, unsupported.ExcludePrunable(), "non-monotone/unknown exclude shapes must keep the full walk")
}

func TestDataflowIncludeReachPrefilter(t *testing.T) {
	prog := NewTmpProgram("sf-dataflow-include-reach")

	source := fastMatchTestValue(t, prog, 1)
	near := fastMatchTestValue(t, prog, 2)
	far := fastMatchTestValue(t, prog, 3)
	disconnected := fastMatchTestValue(t, prog, 4)

	// Path enumeration from near follows EffectOn and reaches source.
	near.AppendDependOn(source)
	// far depends on near: DependOn reaches near, whose EffectOn reaches source.
	far.AppendDependOn(near)

	reach := newDataflowIncludeReach(context.Background(), map[int64]struct{}{source.GetId(): {}})
	require.True(t, reach.CanReach(source), "the source itself satisfies the include")
	require.True(t, reach.CanReach(near))
	require.True(t, reach.CanReach(far))
	require.False(t, reach.CanReach(disconnected))
	require.False(t, reach.CanReach(disconnected), "a failed component must be memoized as unreachable")
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
	ids, ok := check.SimpleIncludeSourceIDs()
	require.True(t, ok)
	require.Contains(t, ids, source.GetId())

	complexCheck := CreateCheck(result, config)
	complexCheck.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Include),
		Value:          "* & $source as $alias",
		SyntaxFlowRule: true,
	})
	_, ok = complexCheck.SimpleIncludeSourceIDs()
	require.False(t, ok, "include shapes with aliases must disable the prefilter")
}
