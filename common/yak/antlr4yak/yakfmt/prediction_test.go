package yakfmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func parsePrediction(source string, fast bool, mode int) (tree parser.IProgramContext, stream *antlr.CommonTokenStream, err error) {
	var lexical *syntaxError
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*syntaxError); ok {
				err = firstError(e, lexical)
			} else {
				panic(r)
			}
		}
	}()
	errors := &errorListener{antlr.NewDefaultErrorListener()}
	lexer := parser.NewYaklangLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errors)
	stream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewYaklangParser(stream)
	p.SetFastPrediction(fast)
	p.RemoveErrorListeners()
	if mode != antlr.PredictionModeSLL {
		p.AddErrorListener(errors)
		// Fresh reference automata prevent optimized parses from warming or
		// influencing the independent original full-context comparison.
		atn := antlr.NewATNDeserializer(nil).Deserialize(parser.GetParserSerializedATN())
		dfas := make([]*antlr.DFA, len(atn.DecisionToState))
		for i, state := range atn.DecisionToState {
			dfas[i] = antlr.NewDFA(state, i)
		}
		p.SetInterpreter(atn, dfas, antlr.NewPredictionContextCache())
		p.GetInterpreter().SetPredictionMode(mode)
		return p.Program(), stream, nil
	}
	p.SetErrorHandler(&bailErrorStrategy{antlr.NewDefaultErrorStrategy()})
	p.GetInterpreter().SetPredictionMode(mode)
	tree, ok, lexical := trySLL(p)
	if !ok {
		tree = retryLL(tree, stream, errors)
	}
	if lexical != nil {
		return nil, stream, lexical
	}
	return tree, stream, nil
}

// Compare every node, boundary, token and trivia, rather than the formatter's
// normalized fingerprint (which intentionally ignores parentheses and ws).
func predictionSnapshot(tree antlr.Tree) string {
	var out strings.Builder
	stack := []antlr.Tree{tree}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := node.(type) {
		case antlr.ParserRuleContext:
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d\n", n, n.GetRuleIndex(), n.GetChildCount(), n.GetStart().GetTokenIndex(), n.GetStop().GetTokenIndex())
		case antlr.TerminalNode:
			token := n.GetSymbol()
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d:%d:%d:%d:%q\n", n, token.GetTokenType(), token.GetTokenIndex(), token.GetStart(), token.GetStop(), token.GetLine(), token.GetColumn(), token.GetChannel(), token.GetText())
		}
		children := node.GetChildren()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return out.String()
}

func checkPrediction(t *testing.T, source string) {
	t.Helper()
	reference, referenceStream, err := parsePrediction(source, false, antlr.PredictionModeLL)
	if err != nil {
		t.Fatal("invalid comparison fixture", err)
	}
	want := predictionSnapshot(reference)
	wantLayout := FormatTree(source, reference, referenceStream)
	for _, fast := range []bool{false, true} {
		tree, stream, err := parsePrediction(source, fast, antlr.PredictionModeSLL)
		if err != nil {
			t.Fatal("SLL-first comparison failed", fast, err)
		}
		if got := predictionSnapshot(tree); got != want {
			t.Fatalf("prediction changed exact AST (fast=%v) for %q", fast, excerpt(source))
		}
		if got := FormatTree(source, tree, stream); got != wantLayout {
			t.Fatalf("prediction changed layout (fast=%v)", fast)
		}
	}
}

func TestFormatPredictionEquivalence(t *testing.T) {
	for i, source := range grammarSamples {
		t.Run(fmt.Sprintf("grammar_%d", i), func(t *testing.T) { checkPrediction(t, source) })
	}
	for _, count := range []int{1, 16, 256} {
		sources := []string{
			"f=func(){" + strings.Repeat("a=1+2; if a {return f(a)};", count) + "return 0}",
			"f(" + strings.Repeat("func(){return 1},", count) + "0)",
			"a=[" + strings.Repeat("x+f(1),", count) + "0]",
			"select{" + strings.Repeat("case ch<-func(){return 1}: a++;", count) + "default: a=0}",
		}
		for i, source := range sources {
			t.Run(fmt.Sprintf("shape_%d_%d", count, i), func(t *testing.T) { checkPrediction(t, source) })
		}
	}
	for _, depth := range []int{8, 64, 128, 256} {
		// Nested call/closure prefixes previously caused cold ATN prediction
		// to explore the entire recursive prefix before choosing a statement.
		source := "f(" + strings.Repeat("func(){return f(", depth) + "1" + strings.Repeat(")}", depth) + ")"
		t.Run(fmt.Sprintf("recursive_call_%d", depth), func(t *testing.T) { checkPrediction(t, source) })
	}
	for _, source := range []string{
		"select\n{}", "select=f;select(1);select.value=1", "f().value=1;f()[0]++;f()~",
		"f(f\"characters ; {} [] ()\",f'${a+1}',f`raw ${b}`)",
		"f(<<<TAG\r\n[] {} () ;\r\nTAG\n)", "f(/* before */func(){\n// in body\nreturn 1},2)",
		"a=func() (int,error){return 1,nil}", "a=func(x){return x}(); a.b.$key[0](func(){return 1})",
		"f(" + strings.Repeat("1,", 4096) + "0)", // exceeds the token scan bound
	} {
		t.Run(fmt.Sprintf("boundary_%d", len(source)), func(t *testing.T) { checkPrediction(t, source) })
	}
	files, err := filepath.Glob("../../yaktest/mustpass/files/*.yak")
	if err != nil || len(files) == 0 {
		t.Fatal("missing real Yak corpus", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			checkPrediction(t, string(source))
		})
	}
}

func TestFormatPredictionDiagnostics(t *testing.T) {
	for _, mode := range []int{antlr.PredictionModeLL, antlr.PredictionModeLLExactAmbigDetection} {
		for _, source := range []string{"f()[0]=1", "a=(x,y)=>x+y", "for v in x{f(v)}", "select{default: f()}"} {
			t.Run(fmt.Sprintf("%d/%s", mode, source), func(t *testing.T) {
				original, _, err := parsePrediction(source, false, mode)
				if err != nil {
					t.Fatal(err)
				}
				optimized, _, err := parsePrediction(source, true, mode)
				if err != nil || predictionSnapshot(original) != predictionSnapshot(optimized) {
					t.Fatal("LL diagnostics mode changed structure", err)
				}
			})
		}
	}
}

func BenchmarkFormatPrediction(b *testing.B) {
	sources := map[string]string{
		"flat_8KiB":          strings.Repeat("a=1+2*3\n", 1024),
		"recursive_call_128": "f(" + strings.Repeat("func(){return f(", 128) + "1" + strings.Repeat(")}", 128) + ")",
		"closure_6KiB":       "f(func(){" + strings.Repeat("a=1+2;", 1000) + "return 1})",
		"over_bound_call":    "f(" + strings.Repeat("1,", 4096) + "0)",
	}
	for name, source := range sources {
		for _, cold := range []bool{false, true} {
			for _, fast := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/cold=%v/fast=%v", name, cold, fast), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					if !cold {
						if _, _, err := parsePrediction(source, fast, antlr.PredictionModeSLL); err != nil {
							b.Fatal(err)
						}
					}
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						lexer := parser.NewYaklangLexer(antlr.NewInputStream(source))
						stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
						p := parser.NewYaklangParser(stream)
						p.SetFastPrediction(fast)
						if cold {
							// Include rebuilding ATN/DFA in both cold measurements;
							// production normally shares them between parser instances.
							atn := antlr.NewATNDeserializer(nil).Deserialize(parser.GetParserSerializedATN())
							dfas := make([]*antlr.DFA, len(atn.DecisionToState))
							for i, state := range atn.DecisionToState {
								dfas[i] = antlr.NewDFA(state, i)
							}
							p.SetInterpreter(atn, dfas, antlr.NewPredictionContextCache())
						}
						p.SetErrorHandler(&bailErrorStrategy{antlr.NewDefaultErrorStrategy()})
						p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
						p.Program()
						if p.HasError() {
							b.Fatal(p.GetError())
						}
					}
				})
			}
		}
	}
}

func TestFormatPredictionInvalid(t *testing.T) {
	for _, source := range []string{
		"a=", "a+=", "a++ +", "if {", "return (", "try{}catch", "switch{case:}",
		"assert", "go (", "defer (", "include 1", "a=func(){return [}",
		"a=func(x) (int,error){return } else", "select{case ch<-func(){return [}:}",
		"a=[func(){return 1},func(){return ]}]", "a=f\"${func(){return [}}\"",
		"f(func(){a=;},\"unterminated)", "f(func(){if{}},\"unterminated)",
		"f(func(){a=;},@)", "f(func(){if{}},@)",
		"f(func(){a=;},\n @)", "f(func(){a=1},\n @)", "a=1\n\"unterminated",
	} {
		t.Run(source, func(t *testing.T) {
			_, _, originalLL := parsePrediction(source, false, antlr.PredictionModeLL)
			if originalLL == nil {
				t.Fatal("invalid fixture accepted by original LL")
			}
			_, _, want := parsePrediction(source, false, antlr.PredictionModeSLL)
			for _, fast := range []bool{false, true} {
				_, _, got := parsePrediction(source, fast, antlr.PredictionModeSLL)
				if got == nil || got.Error() != want.Error() {
					t.Fatalf("changed LL diagnostics (fast=%v): got %v, want %v", fast, got, want)
				}
			}
			if got, err := Format(source); got != "" || err == nil || err.Error() != want.Error() {
				t.Fatalf("formatting changed first error: got %q, %v, want %v", got, err, want)
			}
			if got, err := Format("a=1"); err != nil || got != "a = 1\n" {
				t.Fatal("failed prediction contaminated the next formatter", got, err)
			}
		})
	}
}
