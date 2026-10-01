package test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"strings"
	"testing"
)

func TestPredictionSSAValues(t *testing.T) {
	CheckPythonPrintlnValue("x=1\ny=2\nx+=y\nprintln(x)\nif x>0:\n    println(y)\n", []string{"3", "2"}, t)
}

func BenchmarkPythonPredictionSSA(b *testing.B) {
	for _, fixture := range []struct{ name, source string }{
		{"small", "x=1+2\n"},
		{"linear_8KiB", "a=0\nb=1\n" + strings.Repeat("a=b+1\nb=a+1\n", 680)},
		{"lambda_16", "x=" + strings.Repeat("f(lambda x:", 16) + "x" + strings.Repeat(")", 16) + "\n"},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			parse := func() {
				if _, err := ssaapi.Parse(fixture.source, ssaapi.WithLanguage(ssaconfig.PYTHON), ssaapi.WithMemory()); err != nil {
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
