package tests

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestPredictionSSAValues(t *testing.T) {
	CheckJavaPrintlnValue(`int x = 1, y = 2;
        x += y;
        println(x);
        if (x > 0) { println(y); }
`, []string{"3", "2"}, t)
}

func TestPredictionSSAReferenceAndPrecedence(t *testing.T) {
	CheckJavaPrintlnValue(`Object factory = T<X>::new;
        int a = 1, b = 2, c = 3;
        println(a + b * c);
        println((a + b) * c);
        a += b * c;
        println(a);
`, []string{"7", "9", "7"}, t)
}

func TestPredictionSSAShiftPrecedence(t *testing.T) {
	CheckJavaPrintlnValue(`int a = 1;
        println(a << a << a);
        println(a << 1 + 1);
        println((a << 1) + 1);
`, []string{"4", "4", "3"}, t)
}

func BenchmarkJavaPredictionSSA(b *testing.B) {
	for _, fixture := range []struct{ name, source string }{
		{"small", "class C { int m(){ return 1+2; } }"},
		{"flat_8KiB", "class C { int m(){ int a=0,b=1;" + strings.Repeat("a=b+1; if(a>0) b=a;", 440) + "return a;}}"},
		{"linear_8KiB", "class C { int m(){ int a=0,b=1;" + strings.Repeat("a=b+1; b=a+1;", 630) + "return a;}}"},
		{"lambda_16", "class C { Object m(){ return " + strings.Repeat("f(x -> ", 16) + "x" + strings.Repeat(")", 16) + "; }}"},
		{"generics_16", "class C {" + strings.Repeat("T<", 16) + "X" + strings.Repeat(">", 16) + " x;}"},
		{"reference_generics_16", "class C { Object m(){ return " + strings.Repeat("T<", 16) + "X" + strings.Repeat(">", 16) + "::new; }}"},
		{"shift_chain_128", "class C { int m(int a){ return a" + strings.Repeat("<<a", 128) + "; }}"},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			parse := func() {
				if _, err := ssaapi.Parse(fixture.source, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithMemory()); err != nil {
					b.Fatal(err)
				}
			}
			parse()
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parse()
			}
		})
	}
}
