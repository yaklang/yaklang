package python2ssa

import (
	"strings"
	"testing"
)

func BenchmarkPythonFrontend(b *testing.B) {
	for _, fixture := range []struct{ name, source string }{
		{"small", "x=f(1+2)\n"},
		{"flat_32KiB", strings.Repeat("a=f(1); b=a+2\n", 2400)},
		{"lambda_64", "x=" + strings.Repeat("f(lambda x:", 64) + "x" + strings.Repeat(")", 64) + "\n"},
		{"calls_128", "x=" + strings.Repeat("f(", 128) + "x" + strings.Repeat(")", 128) + "\n"},
		{"arguments_1024", "f(" + strings.Repeat("x,", 1023) + "x)\n"},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			cache := builder.GetAntlrCache()
			if _, err := FrontendWithCache(fixture.source, cache); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := FrontendWithCache(fixture.source, cache); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
