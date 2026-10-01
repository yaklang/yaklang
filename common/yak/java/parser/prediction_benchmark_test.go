package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func benchmarkPredictionSources() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"small", methodSource("return a+b*f(1);")},
		{"flat_32KiB", methodSource(strings.Repeat("a=f(1); if(a) f(a); b+=2;", 1400))},
		{"lambda_64", methodSource("return " + strings.Repeat("f(x -> ", 64) + "x" + strings.Repeat(")", 64) + ";")},
		{"generics_64", "class C {" + strings.Repeat("T<", 64) + "X" + strings.Repeat(">", 64) + " x;}"},
		{"calls_128", methodSource("return " + strings.Repeat("f(", 128) + "x" + strings.Repeat(")", 128) + ";")},
		{"arguments_1024", methodSource("f(" + strings.Repeat("x,", 1023) + "x);")},
		{"switch_128", methodSource("switch(x){" + switchPredictionCases(128) + "default: return 0;}")},
	}
}

func switchPredictionCases(n int) string {
	var out strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&out, "case %d: f(%d); break;", i, i)
	}
	return out.String()
}

// Measures the complete lexer + AST operation. Cold rebuilds the parser ATN,
// DFA and context cache on every iteration; lexer automata are prewarmed in both
// modes. Warm has an independent parser cache for each benchmark configuration.
func BenchmarkJavaPrediction(b *testing.B) {
	for _, fixture := range benchmarkPredictionSources() {
		for _, cold := range []bool{true, false} {
			for _, fast := range []bool{false, true} {
				mode, predictor := "warm", "original"
				if cold {
					mode = "cold"
				}
				if fast {
					predictor = "fast"
				}
				b.Run(fixture.name+"/"+mode+"/"+predictor, func(b *testing.B) {
					cache := newPredictionAutomata()
					parse := func() {
						if cold {
							cache = newPredictionAutomata()
						}
						lexer := NewJavaLexer(antlr.NewInputStream(fixture.source))
						lexer.RemoveErrorListeners()
						p := NewJavaParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
						p.RemoveErrorListeners()
						cache.apply(p)
						p.SetFastPrediction(fast)
						p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
						p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
						p.CompilationUnit()
						if p.GetTokenStream().LA(1) != antlr.TokenEOF {
							b.Fatal("benchmark did not consume source")
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
	}
}
