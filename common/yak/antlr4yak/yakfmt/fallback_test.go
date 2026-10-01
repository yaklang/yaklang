package yakfmt

import (
	"fmt"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// Force cancellation after a complete top-level statement, even on valid
// inputs. This makes successful LL retry/merging reproducible, independent of
// which inputs happen to require LL with the current ANTLR automata.
type checkpointStrategy struct {
	*bailErrorStrategy
	inside bool
}

func (s *checkpointStrategy) Sync(p antlr.Parser) {
	if s.inside {
		for ctx := p.GetParserRuleContext(); ctx != nil; {
			if stmt, ok := ctx.(*parser.StatementContext); ok {
				if list, ok := stmt.GetParent().(*parser.StatementListContext); ok && list.GetChildCount() > 1 {
					if _, topLevel := list.GetParent().(*parser.ProgramContext); topLevel && p.GetParserRuleContext() != ctx {
						panic(antlr.NewParseCancellationException())
					}
				}
			}
			ctx, _ = ctx.GetParent().(antlr.ParserRuleContext)
		}
		return
	}
	if list, ok := p.GetParserRuleContext().(*parser.StatementListContext); ok && list.GetChildCount() > 0 {
		if _, topLevel := list.GetParent().(*parser.ProgramContext); topLevel && p.GetTokenStream().LA(1) != antlr.TokenEOF {
			panic(antlr.NewParseCancellationException())
		}
	}
}

func TestFormatLLCheckpointEquivalence(t *testing.T) {
	prefixes := []string{
		"prefix=1\n",
		"func(a){if(a){return a}else{return 0}}\n",
		"select{case x:= <-ch:f(x);default:}\n",
		"switch a{case 1:a++;default:a--}\n",
		"prefix=f\"原文 ${f(1,2)} ; //\"\n",
		"prefix=<<<TAG\r\n原文 ; // ${x}\r\nTAG\n",
	}
	for i, sample := range grammarSamples {
		t.Run(fmt.Sprintf("%d/whole", i), func(t *testing.T) {
			lex := parser.NewYaklangLexer(antlr.NewInputStream(sample))
			stream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
			tree := retryLL(nil, stream, &errorListener{antlr.NewDefaultErrorListener()})
			full, _, err := parseTest(sample)
			if err != nil || fingerprint(tree, nil) != fingerprint(full, nil) {
				t.Fatal("whole-program LL fallback changed structure", err)
			}
			got := FormatTree(sample, tree, stream)
			want, err := Format(sample)
			if err != nil || got != want {
				t.Fatal("whole-program LL fallback changed layout", err)
			}
		})
		for pi, prefixSource := range prefixes {
			for _, inside := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/prefix%d/inside%v", i, pi, inside), func(t *testing.T) {
					source := "\n// before\n" + prefixSource + "\n// suffix\n" + sample + "\n"
					lex := parser.NewYaklangLexer(antlr.NewInputStream(source))
					stream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
					p := parser.NewYaklangParser(stream)
					p.RemoveErrorListeners()
					p.SetErrorHandler(&checkpointStrategy{bailErrorStrategy: &bailErrorStrategy{antlr.NewDefaultErrorStrategy()}, inside: inside})
					p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
					prefix, ok := trySLL(p)
					if ok || prefix == nil {
						t.Fatal("fixture did not enter checkpoint fallback")
					}
					errors := &errorListener{antlr.NewDefaultErrorListener()}
					tree := retryLL(prefix, stream, errors)
					full, _, err := parseTest(source)
					if err != nil {
						t.Fatal(err)
					}
					if fingerprint(tree, nil) != fingerprint(full, nil) {
						t.Fatal("checkpoint changed syntax structure")
					}
					got := FormatTree(source, tree, stream)
					want, err := Format(source)
					if err != nil || got != want {
						t.Fatalf("checkpoint changed layout: %v\ngot:\n%s\nwant:\n%s", err, excerpt(got), excerpt(want))
					}
				})
			}
		}
	}
}
