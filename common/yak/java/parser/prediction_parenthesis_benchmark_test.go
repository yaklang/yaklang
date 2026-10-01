package javaparser

import (
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func parenthesisPredictionSources() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"lambda_inferred_32", methodSource("return " + strings.Repeat("f((x,y) -> ", 32) + "x" + strings.Repeat(")", 32) + ";")},
		{"lambda_typed_32", methodSource("return " + strings.Repeat("f((T x) -> ", 32) + "x" + strings.Repeat(")", 32) + ";")},
		{"lambda_lvti_32", methodSource("return " + strings.Repeat("f((var x) -> ", 32) + "x" + strings.Repeat(")", 32) + ";")},
		{"grouped_calls_32", methodSource("return " + strings.Repeat("(f(", 32) + "x" + strings.Repeat("))", 32) + ";")},
		{"reference_generics_32", methodSource("return " + strings.Repeat("T<", 32) + "X" + strings.Repeat(">", 32) + "::new;")},
		{"relational_chain_512", methodSource("return a" + strings.Repeat("<a", 512) + ";")},
		{"member_chain_512", methodSource("return a" + strings.Repeat(".a", 512) + ";")},
	}
}

func BenchmarkJavaParenthesisPrediction(b *testing.B) {
	for _, fixture := range parenthesisPredictionSources() {
		for _, cold := range []bool{true, false} {
			mode := "warm"
			if cold {
				mode = "cold"
			}
			b.Run(fixture.name+"/"+mode, func(b *testing.B) {
				cache := newPredictionAutomata()
				parse := func() {
					if cold {
						cache = newPredictionAutomata()
					}
					lexer := NewJavaLexer(antlr.NewInputStream(fixture.source))
					lexer.RemoveErrorListeners()
					p := NewJavaParser(antlr.NewCommonTokenStream(lexer, 0))
					p.RemoveErrorListeners()
					cache.apply(p)
					p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
					p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
					p.CompilationUnit()
					if p.GetTokenStream().LA(1) != antlr.TokenEOF {
						b.Fatal("unconsumed source")
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
