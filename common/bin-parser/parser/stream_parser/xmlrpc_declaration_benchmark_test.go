package stream_parser

import (
	"regexp"
	"strings"
	"testing"
)

var xmlrpcDeclarationBenchmarkMatch bool

// Compare precompiled, warmed matching only (no XML decode or compilation):
// go test ./common/bin-parser/parser/stream_parser -run '^$' -bench '^BenchmarkXMLRPCDeclaration$' -benchmem -benchtime=1000x -count=3 -cpu=1
// For near-limit inputs, use BenchmarkXMLRPCDeclarationNearLimit and 20x.
// Go allocation metrics do not measure the native matcher's C memory pool.
func BenchmarkXMLRPCDeclaration(b *testing.B) {
	benchmarkXMLRPCDeclaration(b, 64<<10, false)
}

func BenchmarkXMLRPCDeclarationNearLimit(b *testing.B) {
	benchmarkXMLRPCDeclaration(b, (1<<20)-128, true)
}

func benchmarkXMLRPCDeclaration(b *testing.B, size int, onlyLong bool) {
	oracle := regexp.MustCompile(xmlrpcDeclaration.String())
	for _, sample := range xmlrpcDeclarationSamples(size) {
		if onlyLong && !strings.HasPrefix(sample.name, "long_") {
			continue
		}
		for _, engine := range []string{"RE2", "PCRE2"} {
			b.Run(sample.name+"/"+engine, func(b *testing.B) {
				match := func() (bool, error) { return oracle.MatchString(sample.input), nil }
				if engine == "PCRE2" {
					match = func() (bool, error) { return xmlrpcDeclaration.MatchString(sample.input) }
				}
				if got, err := match(); err != nil || got != sample.want {
					b.Fatalf("warm match=%v error=%v", got, err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(sample.input)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var err error
					xmlrpcDeclarationBenchmarkMatch, err = match()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
