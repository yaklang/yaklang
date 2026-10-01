package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

var adversarialConflictExpressions = []string{
	"(@A T)x", "(@A T<X> & U) x -> x", "(@A T @B [])x", "((T)x).m()", "(T<U<V>>)x",
	"T<@A X>[]::new", "T<X>.Y<Z>::new", "a::<T<X>>m", "a.<T<X>>m(x -> x)",
	"new T<X>().<Y>m()", "new T<>(){ T<X> x; }.m()", "new int[][]{{1},{2}}[0]",
	"(final var x, @A var y) -> x", "(@A T<X> x, final U y) -> x", "() -> { T<X> x; return x; }",
	"switch(x){case null, default -> 1;}", "switch(x){case T t && p(t) -> 1; default -> 0;}",
	"a<b>>c", "a>>>b<c", "a?b:c?d:e", "a=b=c", "(零) -> 零", "\"\"\"\ntext\n\"\"\"",
}

func TestPredictionAdditionalGrammarConflicts(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range adversarialConflictExpressions {
		for j, pattern := range []string{"return %s;", "f(%s);", "Object x=%s;", "return (%s);", "return a?%s:b;", "if(%s) f();"} {
			t.Run(fmt.Sprintf("expr_%d_%d", i, j), func(t *testing.T) { checkPrediction(t, methodSource(fmt.Sprintf(pattern, expr)), reference, true) })
		}
	}
	for _, name := range []string{"record", "var", "yield", "sealed", "permits", "module", "open", "requires", "transitive", "enum", "零"} {
		for _, pattern := range []string{"T<X> %s=f(x -> x); return %s;", "int %s=1; %s+=2;", "%s: for(;;) break %s;", "f(%s -> %s);"} {
			source := methodSource(fmt.Sprintf(pattern, name, name))
			checkPrediction(t, source, reference, true)
		}
	}
	for _, decl := range []string{"@A T<@B X> @C [] x=f(a -> a);", "T<X>.Y<? extends Z[]>[] x;", "public <T extends X & Y> T m(T... x){return x[0];}", "record N<T>(T x){}", "sealed interface N permits A,B {}", "class N { N(){} <T> N(T x){} }"} {
		checkPrediction(t, "class C {"+decl+"}", reference, true)
	}
}

// Non-bailing SLL recovery is a supported public parser configuration. Prefix
// shortcuts may change where recovery starts, changing errors and error nodes.
func TestPredictionRecoveringSLLDiagnostics(t *testing.T) {
	for _, mode := range []int{antlr.PredictionModeSLL, antlr.PredictionModeLL, antlr.PredictionModeLLExactAmbigDetection} {
		for _, source := range []string{
			methodSource("return a+;"), methodSource("return a=;"), methodSource("return a+=;"), methodSource("return f(1,,2);"), methodSource("return a -> ;"), methodSource("return f(x -> );"), methodSource("return f(;"), methodSource("return new ;"), methodSource("if(x) f(;"),
			methodSource("int x=; return x;"), methodSource("return (T & )x;"), methodSource("return (x,)->;"),
			"class C { T<X x; int y=1; }", "class C { T<> x; }", "class C { T<X,> x; }", "class C { T<? extends > x; }", "class C { T<@A> x; }", "class C { int a[1]; }", "class C { void m( }", "class C { Object x=new ; int y=1; }",
		} {
			t.Run(fmt.Sprintf("mode_%d_%q", mode, source), func(t *testing.T) {

				original, fast := parseRecoveringPrediction(source, false, mode), parseRecoveringPrediction(source, true, mode)
				if fmt.Sprint(original.err) != fmt.Sprint(fast.err) || original.next != fast.next || predictionSnapshot(original.tree) != predictionSnapshot(fast.tree) {
					t.Fatalf("SLL recovery changed\noriginal: %v\nfast: %v", original.err, fast.err)
				}
			})
		}
	}
}

func TestPredictionAdditionalTokenMutations(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range adversarialConflictExpressions {
		source := methodSource("T<X>.Y<Z> before; return " + expr + ";")
		for action := 0; action < 8; action++ {
			for _, position := range []int{i + 2, i + 13, i + 21} {
				mutated := predictionMutation(source, position, action)
				checkPrediction(t, mutated, reference, false)
			}
		}
	}
}

func TestPredictionTriviaAtScannerLimits(t *testing.T) {
	reference := newPredictionAutomata()
	for _, count := range []int{4094, 4095, 4096, 4097} {
		source := "class C {T<" + strings.Repeat("/*trivia*/ ", count) + "X> x;}"
		checkPrediction(t, source, reference, true)
	}
}

func parseRecoveringPrediction(source string, fast bool, mode int) predictionResult {
	errors := antlr4util.NewErrorListener()
	lexer := NewJavaLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errors)
	p := NewJavaParser(antlr.NewCommonTokenStream(lexer, 0))
	newPredictionAutomata().apply(p)
	p.SetFastPrediction(fast)
	p.GetInterpreter().SetPredictionMode(mode)
	p.RemoveErrorListeners()
	p.AddErrorListener(errors)
	tree := p.CompilationUnit()
	return predictionResult{tree, errors.Error(), p.GetTokenStream().LA(1)}
}
func checkRecoveringPrediction(t *testing.T, source string) {
	t.Helper()
	original, fast := parseRecoveringPrediction(source, false, antlr.PredictionModeSLL), parseRecoveringPrediction(source, true, antlr.PredictionModeSLL)
	if fmt.Sprint(original.err) != fmt.Sprint(fast.err) || original.next != fast.next || predictionSnapshot(original.tree) != predictionSnapshot(fast.tree) {
		t.Fatalf("changed recovering SLL result for %q\noriginal: %v\nfast: %v", source, original.err, fast.err)
	}
}

func TestPredictionPublicPrimaryEntry(t *testing.T) {
	for _, source := range []string{"?", "*", "{", "(", "<T>"} {
		parse := func(fast bool) (snapshot string, err error, next int) {
			tree, err := antlr4util.ParseASTWithSLLFirst(source, NewJavaLexer, func(input antlr.TokenStream) *JavaParser {
				p := NewJavaParser(input)
				newPredictionAutomata().apply(p)
				p.SetFastPrediction(fast)
				return p
			}, nil, nil, func(p *JavaParser) IPrimaryContext { tree := p.Primary(); next = p.GetTokenStream().LA(1); return tree })
			return predictionSnapshot(tree), err, next
		}
		original, err, next := parse(false)
		fast, fastErr, fastNext := parse(true)
		if err == nil || fmt.Sprint(err) != fmt.Sprint(fastErr) || next != fastNext || original != fast {
			t.Fatalf("standalone primary changed diagnostics/AST for %q: %v / %v", source, err, fastErr)
		}
	}
}

type recoveringPredictionStrategy struct{ *antlr.DefaultErrorStrategy }

func (*recoveringPredictionStrategy) BailsOnSyntaxError() bool { return false }
func TestPredictionNonBailingMarker(t *testing.T) {
	for _, source := range []string{"class C { T<> x; }", methodSource("return a -> ;")} {
		parse := func(fast bool) predictionResult {
			errors := antlr4util.NewErrorListener()
			p := NewJavaParser(antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream(source)), 0))
			newPredictionAutomata().apply(p)
			p.SetFastPrediction(fast)
			p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			p.SetErrorHandler(&recoveringPredictionStrategy{antlr.NewDefaultErrorStrategy()})
			p.RemoveErrorListeners()
			p.AddErrorListener(errors)
			tree := p.CompilationUnit()
			return predictionResult{tree, errors.Error(), p.GetTokenStream().LA(1)}
		}
		want, got := parse(false), parse(true)
		if fmt.Sprint(want.err) != fmt.Sprint(got.err) || want.next != got.next || predictionSnapshot(want.tree) != predictionSnapshot(got.tree) {
			t.Fatal("false bailout marker enabled shortcuts")
		}
	}
}

func TestPredictionNativeBailStrategy(t *testing.T) {
	for _, source := range []string{"x ->", "x=", "x+=", "f(1,,2)", "x"} {
		parse := func(fast bool) (snapshot string, err error, next int, panicText string) {
			errors := antlr4util.NewErrorListener()
			p := NewJavaParser(antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream(source)), 0))
			newPredictionAutomata().apply(p)
			p.SetFastPrediction(fast)
			p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			p.SetErrorHandler(antlr.NewBailErrorStrategy())
			p.RemoveErrorListeners()
			p.AddErrorListener(errors)
			defer func() {
				if r := recover(); r != nil {
					panicText = fmt.Sprintf("%T:%v", r, r)
				}
				err = errors.Error()
				next = p.GetTokenStream().LA(1)
			}()
			snapshot = predictionSnapshot(p.Expression())
			return
		}
		want, err, next, panicked := parse(false)
		got, gotErr, gotNext, gotPanic := parse(true)
		if panicked != gotPanic || fmt.Sprint(err) != fmt.Sprint(gotErr) || next != gotNext || want != got {
			t.Fatalf("native Bail changed for %q: panic=%q/%q next=%d/%d errors=%v/%v", source, panicked, gotPanic, next, gotNext, err, gotErr)
		}
	}
}
