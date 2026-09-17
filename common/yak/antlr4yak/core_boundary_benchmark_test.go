package antlr4yak

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// Exercise both directions across the native boundary, with ordinary goroutine
// isolation enabled. Compilation is excluded and callback results are checked.
func BenchmarkCoreBoundaryExecution(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"callback", "result=0; for i in 1000 { result += callback((x)=>x+1, i) }; assert result==500500"},
		{"callbackReused", "assert repeat((n)=>n+1)==500500"},
		{"callbackForeign", "assert foreign((n)=>n+1)==500500"},
		{"channel", "for i in 1000 { ch <- i; x = <-ch; assert x==i }"},
		{"asyncNative", "for i in 100 { go noop() }"},
		{"map100", "for i in 10 { for k,v = range data { } }"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			e := New()
			data := make(map[string]int)
			for i := 0; i < 100; i++ {
				data[fmt.Sprint(i)] = i
			}
			repeat := func(f func(int) int) int {
				result := 0
				for i := 0; i < 1000; i++ {
					result += f(i)
				}
				return result
			}
			e.ImportLibs(map[string]any{"repeat": repeat, "foreign": func(f func(int) int) int {
				done := make(chan int, 1)
				go func() { done <- repeat(f) }()
				return <-done
			}, "callback": func(f func(int) int, n int) int { return f(n) }, "ch": make(chan int, 1), "noop": func() {}, "data": data})
			codes, err := e.Compile(tc.source)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := e.GetVM().ExecYakCode(context.Background(), tc.source, codes, yakvm.None); err != nil {
					b.Fatal(err)
				}
				if tc.name == "asyncNative" {
					e.GetVM().AsyncWait()
				}
			}
		})
	}
}
func BenchmarkCoreDecode(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
		value func(int) any
	}{
		{"int10", 10, func(i int) any { return i }},
		{"int1000", 1000, func(i int) any { return i }},
		{"string1000", 1000, func(i int) any { return fmt.Sprintf("literal %d 甲🙂", i) }},
		{"float1000", 1000, func(i int) any { return float64(i) + 0.5 }},
		{"bool1000", 1000, func(i int) any { return i%2 == 0 }},
		{"map100", 100, func(i int) any { return map[string]any{"value": float64(i), "text": "literal"} }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			count := tc.count
			codes := make([]*yakvm.Code, 0, 2*count)
			for i := 0; i < count; i++ {
				codes = append(codes, &yakvm.Code{Opcode: yakvm.OpPush, Op1: yakvm.NewAutoValue(tc.value(i))}, &yakvm.Code{Opcode: yakvm.OpPop})
			}
			m := yakvm.NewCodesMarshaller()
			buf, err := m.Marshal(yakvm.NewSymbolTable(), codes)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(buf)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := m.Unmarshal(buf); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
