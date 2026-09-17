package antlr4yak

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// Includes actual VM dispatch, Go calls, comparisons and a loop. Compilation
// is warmed outside timing; the ordinary goroutine-aware VM stays enabled.
func BenchmarkVMMemberLoop(b *testing.B) {
	const source = `for i := 0; i < 1000; i++ { assert(memberBindingTarget.Ping() == "pong") }`
	engine := newMemberMethodBindingEngine()
	codes, err := engine.Compile(source)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.GetVM().ExecYakCode(context.Background(), source, codes, yakvm.None); err != nil {
			b.Fatal(err)
		}
	}
}
