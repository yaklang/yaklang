package antlr4yak

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"testing"
)

// The same sources run on both sides of the A/B comparison, with compilation
// excluded. One operation is a complete 1,000-iteration script.
func BenchmarkCoreExecution(b *testing.B) {
	for name, source := range map[string]string{
		"arithmetic": "result = 0; for i in 1000 { result += i }; assert result == 499500",
		"functions":  "f = (a,b) => a-b; result = 0; for i in 1000 { result += f(i,1) }; assert result == 498500",
		"native":     "result = 0; for i in 1000 { result += host(i,1) }; assert result == 498500",
		"containers": "for i in 1000 { a = make([]int,8); a[0]=i; b = a[0:1]; assert b[0] == i }",
	} {
		b.Run(name, func(b *testing.B) {
			e := New()
			e.ImportLibs(map[string]any{"host": func(a, b int) int { return a - b }})
			codes, err := e.Compile(source)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := e.GetVM().ExecYakCode(context.Background(), source, codes, yakvm.None); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
