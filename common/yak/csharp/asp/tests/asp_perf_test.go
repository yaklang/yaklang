package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
)

func TestASPFrontParseMetrics_Front(t *testing.T) {
	files := listASPFixtures(t)
	require.GreaterOrEqual(t, len(files), aspMinFixtureCount)

	type agg struct {
		n, bytes       int
		sll, fallbacks uint64
		ferr           uint64
	}
	var overall agg
	byFam := map[string]*agg{}

	for _, rel := range files {
		raw, err := aspFs.ReadFile(rel)
		require.NoError(t, err)
		src := string(raw)
		fam := aspFixtureFamily(rel)
		if byFam[fam] == nil {
			byFam[fam] = &agg{}
		}
		t.Run(rel, func(t *testing.T) {
			antlr4util.ResetSLLFirstCounters()
			start := time.Now()
			ast, err := asp.Front(src)
			dur := time.Since(start)
			require.NoError(t, err)
			require.NotNil(t, ast)
			require.LessOrEqual(t, dur, savedASPFrontFixtureMaxParseDuration)
			stats := antlr4util.SLLFirstCountersSnapshot()
			require.Zero(t, stats.FallbackError)
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
			t.Logf("ASP_METRICS family=%s file=%s parse=%s bytes=%d sll_attempts=%d fallbacks=%d errors=%d",
				fam, rel, dur, len(src), stats.SLLAttempts, stats.Fallbacks, stats.FallbackError)
		})
	}
	require.Zero(t, overall.ferr)
	t.Logf("ASP_METRICS_OVERALL files=%d bytes=%d sll_attempts=%d fallbacks=%d errors=%d",
		overall.n, overall.bytes, overall.sll, overall.fallbacks, overall.ferr)
}

func TestASPFrontBenchmarkMetrics(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{name: "small", file: "code/ga/tags/paired.aspx"},
		{name: "typical", file: "code/hello.aspx"},
		{name: "large", file: "code/ga/stress/many_tags.aspx"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			raw, err := aspFs.ReadFile(tc.file)
			require.NoError(t, err)
			src := string(raw)
			res := testing.Benchmark(func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					antlr4util.ResetSLLFirstCounters()
					ast, err := asp.Front(src)
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
			require.Greater(t, res.NsPerOp(), int64(0))
			t.Logf("ASP_BENCH name=%s file=%s n=%d ns/op=%d allocs/op=%d bytes/op=%d",
				tc.name, tc.file, res.N, res.NsPerOp(), res.AllocsPerOp(), res.AllocedBytesPerOp())
		})
	}
}

func BenchmarkASPFrontSmall(b *testing.B)   { benchmarkASPFile(b, "code/ga/tags/paired.aspx") }
func BenchmarkASPFrontTypical(b *testing.B) { benchmarkASPFile(b, "code/hello.aspx") }
func BenchmarkASPFrontLarge(b *testing.B)   { benchmarkASPFile(b, "code/ga/stress/many_tags.aspx") }

func benchmarkASPFile(b *testing.B, rel string) {
	b.Helper()
	raw, err := aspFs.ReadFile(rel)
	if err != nil {
		b.Fatal(err)
	}
	src := string(raw)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ast, err := asp.Front(src)
		if err != nil {
			b.Fatal(err)
		}
		if ast == nil {
			b.Fatal("nil")
		}
	}
}
