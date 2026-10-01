package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// These inputs changed successful prefixes or suppressed original diagnostics
// before compilation-unit scoping. A direct entry need not consume all input.
func TestPredictionPartialRuleRegressions(t *testing.T) {
	for _, fixture := range []predictionRuleFixture{
		{"cast_lambda", "(T & U) x -> x", func(p *JavaParser) antlr.Tree { return p.Expression() }},
		{"initializer", "record T[]{x,y}", func(p *JavaParser) antlr.Tree { return p.VariableInitializer() }},
		{"block", "{ T->X> x; return x; }", func(p *JavaParser) antlr.Tree { return p.Block() }},
		{"block_list", "T<X> x-> f(x);", func(p *JavaParser) antlr.Tree { return p.BlockStatementList() }},
		{"block_statement", "T<X-> x=f(a->a);", func(p *JavaParser) antlr.Tree { return p.BlockStatement() }},
		{"switch_label", "case x->y:", func(p *JavaParser) antlr.Tree { return p.SwitchLabel() }},
		{"cast_suffix", "(T & U) x  x", func(p *JavaParser) antlr.Tree { return p.Expression() }},
		{"guarded_pattern", "(T t && p(t)) && q/*c*/t)", func(p *JavaParser) antlr.Tree { return p.GuardedPattern() }},
	} {
		t.Run(fixture.name, func(t *testing.T) { checkPredictionRule(t, fixture.source, fixture.entry, nil, false) })
	}
}

func TestPredictionRootChangesOnReusedParser(t *testing.T) {
	p := NewJavaParser(predictionTokens("class C {}"))
	p.RemoveErrorListeners()
	p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
	for _, fixture := range []struct {
		source      string
		entry       func(*JavaParser) antlr.Tree
		compilation bool
	}{
		{"class C { T<X> x; }", func(p *JavaParser) antlr.Tree { return p.CompilationUnit() }, true},
		{"(T & U) x -> x", func(p *JavaParser) antlr.Tree { return p.Expression() }, false},
		{"class D {}", func(p *JavaParser) antlr.Tree { return p.CompilationUnit() }, true},
		{"{ f(x); }", func(p *JavaParser) antlr.Tree { return p.Block() }, false},
		{"class E {}", func(p *JavaParser) antlr.Tree { return p.CompilationUnit() }, true},
		{"int", func(p *JavaParser) antlr.Tree { return p.PrimitiveType() }, false},
		{"class F {}", func(p *JavaParser) antlr.Tree { return p.CompilationUnit() }, true},
	} {
		p.SetTokenStream(predictionTokens(fixture.source))
		fixture.entry(p)
		if p.compilationUnitPrediction != fixture.compilation {
			t.Fatalf("root scope leaked for %q", fixture.source)
		}
	}
	// Cancellation leaves nested contexts until the input is reset. The next
	// standalone recursion entry must still turn off compilation-unit shortcuts.
	p.SetTokenStream(predictionTokens("class C { int x=; }"))
	cancelled := false
	func() { defer func() { cancelled = recover() != nil }(); p.CompilationUnit() }()
	if !cancelled {
		t.Fatal("invalid compilation unit did not cancel")
	}
	p.SetTokenStream(predictionTokens("x -> x"))
	p.Expression()
	if p.compilationUnitPrediction {
		t.Fatal("cancelled root leaked into a standalone expression")
	}
}

var challengeTypeReferences = []string{
	"T<X>::new", "T<X>::m", "T<X>::<Y>m", "T<X>.U<Y>::new", "T<X>[]::new", "T<X>[][]::m", "int[]::new", "int[][]::new",
	"T<X>.var::new", "T<X>.yield::new", "var<X>.T::new", "yield<X>.T::new", "T<@A X>::new", "T<X>.@A U::new", "T<X> @A []::new",
	"T<X>::", "T<>::new", "T<X,,Y>::new", "T<X[>::new", "T<X>::?", "T<X>+y", "a[x]::m", "T<X>.m()", "a<b>>c", "a<b>c", "T<X> x",
}

func TestPredictionTypeReferenceConflicts(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range challengeTypeReferences {
		for j, pattern := range []string{"return %s;", "f(%s);", "Object x=%s;", "return (%s);", "return a?%s:b;", "if(%s) f();"} {
			t.Run(fmt.Sprintf("%d_%d", i, j), func(t *testing.T) { checkPrediction(t, methodSource(fmt.Sprintf(pattern, expr)), reference, false) })
		}
		source := methodSource("return " + expr + ";")
		for action := 0; action < 8; action++ {
			for _, position := range []int{2, i + 5, i + 11} {
				checkPrediction(t, predictionMutation(source, position, action), reference, false)
			}
		}
	}
	for _, depth := range []int{1, 8, 32, 64, 128} {
		expr := strings.Repeat("T<", depth) + "X" + strings.Repeat(">", depth) + "::new"
		checkPrediction(t, methodSource("return "+expr+";"), reference, true)
	}
}

func TestPredictionTypeReferenceScannerContract(t *testing.T) {
	for _, fixture := range []struct {
		source string
		want   bool
	}{
		{"T<X>::new", true}, {"T<X>.U<Y>[]::m", true}, {"int[][]::new", true},
		{"T<X>.var::new", false}, {"T<X>.yield::new", false}, {"T<@A X>::new", false}, {"T<X>.@A U::new", false},
		{"a[0]::m", false}, {"T<X> x", false}, {"T<X>.int::new", false}, {"T<X>[]x", false}, {"T<X+>::new", false},
	} {
		stream := predictionTokens(fixture.source)
		index := stream.Index()
		if got := typeReferencePrefix(stream); got != fixture.want {
			t.Fatalf("prefix %q: %v != %v", fixture.source, got, fixture.want)
		}
		if stream.Index() != index {
			t.Fatal("reference scan consumed input")
		}
	}
	for _, depth := range []int{511, 512, 513} {
		stream := predictionTokens(strings.Repeat("T<", depth) + "X" + strings.Repeat(">", depth) + "::new")
		if typeReferencePrefix(stream) != (depth <= declarationPrefixDepth) {
			t.Fatalf("depth bound %d", depth)
		}
	}
	for _, count := range []int{4090, 4091, 4092, 4093, 4094, 4095, 4096} {
		stream := predictionTokens("T<" + strings.Repeat("/*c*/", count) + "X>::new")
		got := typeReferencePrefix(stream)
		if got != (count <= 4091) { // T, <, X, > and :: occupy five raw tokens.
			t.Fatalf("raw-token bound %d: %v", count, got)
		}
		if len(stream.GetAllTokens()) > declarationPrefixTokens+1 {
			t.Fatal("reference scanner exceeded raw-token budget")
		}
	}
	stream := predictionTokens("T<X>::new")
	if typeReferencePrefix(otherPredictionStream{stream}) {
		t.Fatal("non-common stream must use ATN")
	}
	hidden := antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream(" /*c*/ T<X>::new")), antlr.TokenHiddenChannel)
	hidden.LA(1)
	if typeReferencePrefix(hidden) {
		t.Fatal("hidden-channel stream must use ATN")
	}
	empty := antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream("T<X>::new")), 0)
	if typeReferencePrefix(empty) {
		t.Fatal("uninitialized stream must use ATN")
	}
}

func TestPredictionParserResetClearsCancellation(t *testing.T) {
	for _, name := range []string{"tokens", "input"} {
		t.Run(name, func(t *testing.T) {
			p := NewJavaParser(predictionTokens("class C { int x=; }"))
			p.RemoveErrorListeners()
			p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
			p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			cancelled := false
			func() { defer func() { cancelled = recover() != nil }(); p.CompilationUnit() }()
			if !cancelled || !p.HasError() {
				t.Fatal("fixture did not leave a cancelled recognition error")
			}
			stream := predictionTokens("class Valid { Object x=T<X>::new; }")
			if name == "tokens" {
				p.SetTokenStream(stream)
			} else {
				p.SetInputStream(stream)
			}
			if p.HasError() {
				t.Fatal("reset retained old recognition error")
			}
			p.CompilationUnit()
			if p.HasError() || p.GetTokenStream().LA(1) != antlr.TokenEOF {
				t.Fatal("valid source failed after reset")
			}
		})
	}
}

func TestPredictionUnknownPrimaryPrefixesStayWithATN(t *testing.T) {
	for _, source := range []string{"?", "$", "@", "{", "*"} {
		stream := predictionTokens(source)
		index := stream.Index()
		if primaryPrefix(stream) != 0 {
			t.Fatalf("unknown primary %q bypassed ATN", source)
		}
		if stream.Index() != index {
			t.Fatal("primary lookahead consumed input")
		}
	}
}

func TestPredictionLongNonReferencePrefixes(t *testing.T) {
	reference := newPredictionAutomata()
	for _, n := range []int{32, 128, 512} {
		for _, expr := range []string{"a" + strings.Repeat("<a", n), "a" + strings.Repeat(".a", n), "a<" + strings.Repeat("/*c*/", n) + "b"} {
			checkPrediction(t, methodSource("return "+expr+";"), reference, true)
		}
	}
}
