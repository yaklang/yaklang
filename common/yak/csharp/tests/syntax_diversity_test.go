package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
)

func TestCSharpASTCorpusIsExactly100Diverse(t *testing.T) {
	files := listCSharpASTFixtures(t)
	require.Equal(t, csharpGAFixtureCount, len(files), "committed C# AST fixtures must be exactly 100")

	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	rawSeen := map[string]string{}
	normSeen := map[string]string{}
	byFamily := map[string][]string{}
	var metrics []csharpParseMetrics

	for _, rel := range files {
		src := mustReadCodeFixture(t, rel)
		require.NotEmpty(t, src, rel)
		raw := sha256Hex(src)
		if prev, ok := rawSeen[raw]; ok {
			t.Fatalf("byte-identical clone: %s and %s", prev, rel)
		}
		rawSeen[raw] = rel
		norm := sha256Hex(compactCSharpSource(src))
		if prev, ok := normSeen[norm]; ok {
			t.Fatalf("whitespace-normalized clone: %s and %s", prev, rel)
		}
		normSeen[norm] = rel

		fam := csharpFixtureFamily(rel)
		byFamily[fam] = append(byFamily[fam], rel)

		t.Run("parse/"+rel, func(t *testing.T) {
			_, dur, stats := parseCSharpFrontend(t, src, cache)
			metrics = append(metrics, csharpParseMetrics{
				Path:     rel,
				Family:   fam,
				Bytes:    len(src),
				Duration: dur,
				SLL:      stats,
			})
			t.Logf("csharp fixture=%s family=%s parse=%s bytes=%d sll_attempts=%d fallbacks=%d cancelled=%d errors=%d",
				rel, fam, dur, len(src), stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError)
		})
	}

	for _, fam := range csharpRequiredFamilies {
		require.NotEmpty(t, byFamily[fam], "required construct family %s has no fixture", fam)
	}
	require.GreaterOrEqual(t, len(byFamily), len(csharpRequiredFamilies))
	for fam, members := range byFamily {
		t.Logf("family %s count=%d", fam, len(members))
	}
	_ = metrics
}

func TestCSharpASTNamedConstructsPerFamily_Frontend(t *testing.T) {
	cases := []struct {
		file string
		want string
	}{
		{"code/ga/types/abstract_sealed.cs", "GaTypesSealedLeaf"},
		{"code/ga/control/if_else_chain.cs", "GaControlIf"},
		{"code/ga/async_linq/await_task.cs", "GaAsyncAwait"},
		{"code/ga/preproc/define_if.cs", "GaPreprocIf"},
		{"code/ga/interpolated/regular.cs", "GaInterpRegular"},
		{"code/ga/patterns/switch_expr.cs", "GaPatternsSwitchExpr"},
		{"code/ga/operators/arith.cs", "GaOpsArith"},
		{"code/ga/generics/class_where_class.cs", "GaGenBox"},
		{"code/ga/attributes/obsolete.cs", "GaAttrObsolete"},
		{"code/ga/using/ns_alias.cs", "GaUsingAlias"},
		{"code/ga/stress/many_types.cs", "GaStressHost"},
		{"code/public/CSharp8SwitchExpressions.cs", "CSharp8SwitchExpressions"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			src := mustReadCodeFixture(t, tc.file)
			ast, err := csharp2ssa.Frontend(src)
			require.NoError(t, err)
			require.NotNil(t, ast)
			unit, ok := ast.(csharpparser.ICompilation_unitContext)
			require.True(t, ok)
			names := collectTypeNames(unit)
			require.Contains(t, names, tc.want, "shipped Frontend AST must contain named type %s", tc.want)
		})
	}
}
