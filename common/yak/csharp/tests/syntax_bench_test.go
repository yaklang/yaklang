package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
)

func TestCSharpASTParseMetricsByFamily_Frontend(t *testing.T) {
	files := listCSharpASTFixtures(t)
	require.Equal(t, csharpGAFixtureCount, len(files))

	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	type agg struct {
		n, bytes       int
		sll, fallbacks uint64
		ferr           uint64
	}
	byFam := map[string]*agg{}
	var overall agg

	for _, rel := range files {
		src := mustReadCodeFixture(t, rel)
		fam := csharpFixtureFamily(rel)
		if byFam[fam] == nil {
			byFam[fam] = &agg{}
		}
		t.Run(rel, func(t *testing.T) {
			_, dur, stats := parseCSharpFrontend(t, src, cache)
			a := byFam[fam]
			a.n++
			a.bytes += len(src)
			a.sll += stats.SLLAttempts
			a.fallbacks += stats.Fallbacks
			a.ferr += stats.FallbackError
			overall.n++
			overall.bytes += len(src)
			overall.sll += stats.SLLAttempts
			overall.fallbacks += stats.Fallbacks
			overall.ferr += stats.FallbackError
			t.Logf("METRICS family=%s file=%s parse=%s bytes=%d sll_attempts=%d fallbacks=%d cancelled=%d errors=%d",
				fam, rel, dur, len(src), stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError)
		})
	}

	require.Zero(t, overall.ferr)
	t.Logf("METRICS_OVERALL files=%d bytes=%d sll_attempts=%d fallbacks=%d errors=%d",
		overall.n, overall.bytes, overall.sll, overall.fallbacks, overall.ferr)
	for _, fam := range csharpRequiredFamilies {
		a := byFam[fam]
		require.NotNil(t, a, fam)
		require.Greater(t, a.n, 0, fam)
		t.Logf("METRICS_FAMILY family=%s files=%d bytes=%d sll_attempts=%d fallbacks=%d errors=%d",
			fam, a.n, a.bytes, a.sll, a.fallbacks, a.ferr)
	}
}

func TestCSharpFrontendBenchmarkMetrics(t *testing.T) {
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	cases := []struct {
		name string
		file string
	}{
		{name: "small", file: "code/ga/operators/arith.cs"},
		{name: "public", file: "code/public/CSharp8SwitchExpressions.cs"},
		{name: "large", file: "code/public/AllInOneNoPreprocessor.cs"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			src := mustReadCodeFixture(t, tc.file)
			res := testing.Benchmark(func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					antlr4util.ResetSLLFirstCounters()
					ast, err := csharp2ssa.Frontend(src, cache)
					if err != nil {
						b.Fatal(err)
					}
					if ast == nil {
						b.Fatal("nil ast")
					}
					if antlr4util.SLLFirstCountersSnapshot().FallbackError > 0 {
						b.Fatal("fallback error")
					}
				}
			})
			require.Greater(t, res.N, 0)
			require.Greater(t, res.NsPerOp(), int64(0), "benchmark must report ns/op for %s", tc.name)
			t.Logf("BENCH name=%s file=%s n=%d ns/op=%d allocs/op=%d bytes/op=%d",
				tc.name, tc.file, res.N, res.NsPerOp(), res.AllocsPerOp(), res.AllocedBytesPerOp())
		})
	}
}

func BenchmarkCSharpFrontendSmall(b *testing.B) {
	benchmarkCSharpFrontendFile(b, "code/ga/operators/arith.cs")
}

func BenchmarkCSharpFrontendPublic(b *testing.B) {
	benchmarkCSharpFrontendFile(b, "code/public/CSharp8SwitchExpressions.cs")
}

func BenchmarkCSharpFrontendLarge(b *testing.B) {
	benchmarkCSharpFrontendFile(b, "code/public/AllInOneNoPreprocessor.cs")
}

func benchmarkCSharpFrontendFile(b *testing.B, rel string) {
	b.Helper()
	raw, err := codeFs.ReadFile(rel)
	if err != nil {
		b.Fatal(err)
	}
	src := string(raw)
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	if !ok {
		b.Fatal("builder type")
	}
	defer builder.Clearup()
	cache := builder.GetAntlrCache()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ast, err := csharp2ssa.Frontend(src, cache)
		if err != nil {
			b.Fatal(err)
		}
		if ast == nil {
			b.Fatal("nil ast")
		}
	}
}
