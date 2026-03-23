package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

func benchmarkFrontendFixture(b *testing.B, fixture string) {
	raw, err := syntaxFs.ReadFile(fixture)
	if err != nil {
		b.Fatalf("read fixture %s: %v", fixture, err)
	}

	src := string(raw)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := php2ssa.Frontend(src, phpTestAntlrCache); err != nil {
			b.Fatalf("parse fixture %s: %v", fixture, err)
		}
	}
}

func BenchmarkFrontendHelloFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "syntax/hello.php")
}

func BenchmarkFrontendPfsenseSystemInformationFixture(b *testing.B) {
	benchmarkFrontendFixture(b, "syntax/pfsense/system_information.widget.php")
}
