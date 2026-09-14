package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
)

func TestCSharpASTStressFullCorpusWalk_Frontend(t *testing.T) {
	files := listCSharpASTFixtures(t)
	require.Equal(t, csharpGAFixtureCount, len(files))

	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	large := mustReadCodeFixture(t, "code/public/AllInOneNoPreprocessor.cs")
	t.Run("large", func(t *testing.T) {
		parseCSharpFrontend(t, large, cache)
	})
	t.Run("stress-many-types", func(t *testing.T) {
		src := mustReadCodeFixture(t, "code/ga/stress/many_types.cs")
		ast, _, _ := parseCSharpFrontend(t, src, cache)
		names := collectTypeNames(ast)
		require.Contains(t, names, "GaStressHost")
		require.GreaterOrEqual(t, len(names), 40)
	})

	for _, rel := range files {
		rel := rel
		src := mustReadCodeFixture(t, rel)
		t.Run("walk/"+rel, func(t *testing.T) {
			parseCSharpFrontend(t, src, cache)
		})
	}
}

func TestCSharpASTRedundantParseEquivalentTrees_Frontend(t *testing.T) {
	reps := []string{
		"code/basic.cs",
		"code/ga/types/abstract_sealed.cs",
		"code/ga/stress/many_types.cs",
		"code/public/CSharp8SwitchExpressions.cs",
		"code/public/AllInOneNoPreprocessor.cs",
		"code/ga/preproc/elif_chain.cs",
	}
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	for _, rel := range reps {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			src := mustReadCodeFixture(t, rel)
			a1, _, _ := parseCSharpFrontend(t, src, cache)
			a2, _, _ := parseCSharpFrontend(t, src, cache)
			u1, ok1 := a1.(csharpparser.ICompilation_unitContext)
			u2, ok2 := a2.(csharpparser.ICompilation_unitContext)
			require.True(t, ok1)
			require.True(t, ok2)
			require.Equal(t, collectTypeNames(u1), collectTypeNames(u2), "second parse type names must match")
			require.Equal(t, u1.GetText(), u2.GetText(), "second parse GetText must match")
		})
	}
}
