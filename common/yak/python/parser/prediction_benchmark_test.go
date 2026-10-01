package pythonparser

import (
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func benchmarkPredictionSources() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"small", "x=f(1+2)\n"},
		{"flat_32KiB", strings.Repeat("a=f(1); b=a+2\n", 2400)},
		{"lambda_64", "x=" + strings.Repeat("f(lambda x:", 64) + "x" + strings.Repeat(")", 64) + "\n"},
		{"calls_128", "x=" + strings.Repeat("f(", 128) + "x" + strings.Repeat(")", 128) + "\n"},
		{"arguments_1024", "f(" + strings.Repeat("x,", 1023) + "x)\n"},
		{"comprehension_64", "x=[x " + strings.Repeat("for x in xs if x ", 64) + "]\n"},
		{"conditional_64", "x=" + strings.Repeat("x if y else ", 64) + "x\n"},
	}
}

// Measures the complete lexer + AST operation. Cold rebuilds the parser ATN,
// DFA and context cache on every iteration; lexer automata are prewarmed in both
// modes. Warm has an independent parser cache for each benchmark configuration.
func BenchmarkPythonPrediction(b *testing.B) {
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
						lexer := NewPythonLexer(antlr.NewInputStream(fixture.source))
						lexer.RemoveErrorListeners()
						p := NewPythonParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
						p.RemoveErrorListeners()
						cache.apply(p)
						p.SetFastPrediction(fast)
						p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
						p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
						p.Root()
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
