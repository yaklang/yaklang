package antlr4yak

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func BenchmarkSelectExecution(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"ordinaryReceive", `for i in 100 { ch <- i; v=<-ch; assert v==i }`},
		{"selectReceive", `for i in 100 { ch <- i; select {case v:=<-ch: assert v==i} }`},
		{"ordinarySend", `for i in 100 { ch <- i; assert <-ch==i }`},
		{"selectSend", `for i in 100 { select {case ch<-i:}; assert <-ch==i }`},
		{"selectDefault", `for i in 100 {select {case <-empty: panic("receive"); default:}}`},
	} {
		b.Run(tc.name, func(b *testing.B) {
			engine := New()
			engine.ImportLibs(map[string]any{"ch": make(chan int, 1), "empty": make(chan int)})
			codes, err := engine.Compile(tc.source)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := engine.GetVM().ExecYakCode(ctx, tc.source, codes, yakvm.None); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*100), "ns/iteration")
		})
	}
}
