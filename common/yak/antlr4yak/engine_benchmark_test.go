package antlr4yak

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func benchmarkYakFunction(b *testing.B, ctx context.Context) {
	engine := New()
	if err := engine.SafeEvalWithoutCache(context.Background(), `
handler = () => {
    total = 0
    for i in 10 {
        total += i
    }
    return total
}
`); err != nil {
		b.Fatal(err)
	}
	raw, ok := engine.GetVar("handler")
	if !ok {
		b.Fatal("handler not found")
	}
	function, ok := raw.(*yakvm.Function)
	if !ok {
		b.Fatalf("handler has unexpected type %T", raw)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.CallYakFunctionNative(ctx, function); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCallYakFunctionBackgroundContext(b *testing.B) {
	benchmarkYakFunction(b, context.Background())
}

func BenchmarkCallYakFunctionCancelableContext(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)
	benchmarkYakFunction(b, ctx)
}
