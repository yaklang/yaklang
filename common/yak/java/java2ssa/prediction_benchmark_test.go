package java2ssa

import (
	"strings"
	"testing"
)

func BenchmarkJavaFrontend(b *testing.B) {
	for _, fixture := range []struct{ name, source string }{
		{"small", "class C { int m(){ return 1+2; } }"},
		{"flat_32KiB", "class C { int m(){ int a=0,b=1;" + strings.Repeat("a=b+1; if(a>0) b=a;", 1800) + "return a;}}"},
		{"lambda_64", "class C { Object m(){ return " + strings.Repeat("f(x -> ", 64) + "x" + strings.Repeat(")", 64) + "; }}"},
		{"generics_64", "class C {" + strings.Repeat("T<", 64) + "X" + strings.Repeat(">", 64) + " x;}"},
		{"reference_generics_32", "class C { Object m(){ return " + strings.Repeat("T<", 32) + "X" + strings.Repeat(">", 32) + "::new; }}"},
		{"shift_chain_256", "class C { int m(int a){ return a" + strings.Repeat("<<a", 256) + "; }}"},
		{"class_literal_qualified", "class C { Object m(){ return a.b.T.class; }}"},
		{"class_literal_generics_32", "class C { Object m(){ return " + strings.Repeat("T<", 32) + "X" + strings.Repeat(">", 32) + ".class; }}"},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			cache := builder.GetAntlrCache()
			if _, err := Frontend(fixture.source, cache); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Frontend(fixture.source, cache); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
