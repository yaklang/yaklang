package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
)

func predictionTokens(source string) *antlr.CommonTokenStream {
	lexer := NewJavaLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.LA(1)
	return stream
}

func TestPredictionDeclarationScanner(t *testing.T) {
	for _, fixture := range []struct {
		source string
		kind   int
	}{
		{"C()", 5}, {"C /*c*/ ()", 5}, {"int x;", 4}, {"int[] x[],y;", 4}, {"T<X<Y>> x=f();", 4},
		{"a.b.T<X>.U<Y>[] x;", 4}, {"T<? extends X[]> x;", 4}, {"T<? super X> x;", 4}, {"T<?> x;", 4},
		{"void m()", 2}, {"T m()", 2}, {"T<X>[] m()", 2}, {"boolean m()", 2},
		{"record R(){}", 0}, {"enum E{}", 0}, {"@A T x;", 0}, {"T @A [] x;", 0}, {"<T> T m()", 0},
		{"void x;", 0}, {"T x[1];", 0}, {"T<@A X> x;", 0}, {"T<f()> x;", 0}, {"T<X x;", 0}, {"T<X>>x;", 0},
		{"x.f()", 0}, {"T x", 0}, {"T . ;", 0}, {"int [ x", 0}, {"T<", 0}, {"", 0}, {";", 0},
	} {
		t.Run(fixture.source, func(t *testing.T) {
			stream := predictionTokens(fixture.source)
			p := NewJavaParser(stream)
			index := stream.Index()
			for i := 0; i < 2; i++ {
				if got := p.declarationPrefix(stream); got != fixture.kind {
					t.Fatalf("kind=%d want=%d", got, fixture.kind)
				}
				if stream.Index() != index {
					t.Fatal("scanner consumed input")
				}
			}
		})
	}
	for _, n := range []int{512, 513, 1400} {
		source := strings.Repeat("T<", n) + "X" + strings.Repeat(">", n) + " x;"
		want := 4
		if n > declarationPrefixDepth {
			want = 0
		}
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			stream := predictionTokens(source)
			if got := NewJavaParser(stream).declarationPrefix(stream); got != want {
				t.Fatalf("kind=%d want=%d", got, want)
			}
			if len(stream.GetAllTokens()) > declarationPrefixTokens+1 {
				t.Fatal("scanner exceeded token bound")
			}
		})
	}
	stream := predictionTokens("T " + strings.Repeat("/*c*/", 4096) + " x;")
	if NewJavaParser(stream).declarationPrefix(stream) != 0 {
		t.Fatal("hidden tokens must also count toward the scan limit")
	}
}

func TestPredictionQualifiedTypeScanner(t *testing.T) {
	for _, fixture := range []struct {
		source string
		alt    int
	}{
		{"T.X", 1}, {"T<X>.Y", 1}, {"T<X<Y>> /*c*/ .Z", 1}, {"T<X<Y>> name", 2}, {"T[]", 2}, {"T;", 2},
		{"T.class", 2}, {"T<X>.class", 2}, {"T<X>./*c*/class", 2}, {"T. @A X", 1},
		{"@A T", 0}, {"int", 0}, {"T<@A X>", 0}, {"T<(X)>", 0}, {"T<X", 0}, {"", 0},
	} {
		t.Run(fixture.source, func(t *testing.T) {
			stream := predictionTokens(fixture.source)
			index := stream.Index()
			if got := qualifiedTypePrefix(stream); got != fixture.alt {
				t.Fatalf("alt=%d want=%d", got, fixture.alt)
			}
			if stream.Index() != index {
				t.Fatal("scanner consumed input")
			}
		})
	}
	for _, n := range []int{513, 1400} {
		if qualifiedTypePrefix(predictionTokens(strings.Repeat("T<", n)+"X"+strings.Repeat(">", n))) != 0 {
			t.Fatal("qualified scan exceeded bound")
		}
	}
	if qualifiedTypePrefix(predictionTokens("T."+strings.Repeat("/*c*/", declarationPrefixTokens)+"class")) != 0 {
		t.Fatal("lookahead beyond a qualified dot exceeded the raw-token budget")
	}
}

type otherPredictionStream struct{ antlr.TokenStream }

func TestPredictionScannerStreamReuse(t *testing.T) {
	stream := predictionTokens("T x;")
	p := NewJavaParser(stream)
	if p.declarationPrefix(stream) != 4 {
		t.Fatal("initial declaration")
	}
	if p.declarationPrefix(otherPredictionStream{stream}) != 0 || qualifiedTypePrefix(otherPredictionStream{stream}) != 0 {
		t.Fatal("non-common stream must use ATN")
	}
	for _, source := range []string{"T m()", "C()", "record R(){}", "T<X> x;", "T x[1];"} {
		fresh := predictionTokens(source)
		p.SetInputStream(fresh)
		want := NewJavaParser(fresh).declarationPrefix(fresh)
		if got := p.declarationPrefix(fresh); got != want {
			t.Fatalf("SetInputStream retained %d instead of %d", got, want)
		}
	}
	for _, source := range []string{"C()", "T m()", "@A T x;", "T[] x;"} {
		stream.SetTokenSource(NewJavaLexer(antlr.NewInputStream(source)))
		stream.LA(1)
		want := NewJavaParser(stream).declarationPrefix(stream)
		if got := p.declarationPrefix(stream); got != want {
			t.Fatalf("SetTokenSource retained %d instead of %d", got, want)
		}
	}
	stream = predictionTokens("T x; T m()")
	p = NewJavaParser(stream)
	if p.declarationPrefix(stream) != 4 {
		t.Fatal("first declaration")
	}
	stream.Fill()
	var second int
	for _, token := range stream.GetAllTokens() {
		if token.GetText() == "T" && token.GetTokenIndex() > 0 {
			second = token.GetTokenIndex()
			break
		}
	}
	stream.Seek(second)
	if p.declarationPrefix(stream) != 2 {
		t.Fatal("second declaration")
	}
	stream.Seek(0)
	if p.declarationPrefix(stream) != 4 {
		t.Fatal("seek retained the wrong cached prefix")
	}
	// A stream selecting hidden-channel trivia cannot use default-channel scans.
	lexer := NewJavaLexer(antlr.NewInputStream("/*comment*/ T x;"))
	hidden := antlr.NewCommonTokenStream(lexer, antlr.TokenHiddenChannel)
	hidden.LA(1)
	if p.declarationPrefix(hidden) != 0 || qualifiedTypePrefix(hidden) != 0 {
		t.Fatal("hidden-channel stream used fast scan")
	}
	uninitialized := antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream("T x;")), antlr.TokenDefaultChannel)
	if p.declarationPrefix(uninitialized) != 0 || qualifiedTypePrefix(uninitialized) != 0 {
		t.Fatal("uninitialized stream must use ATN")
	}
	if qualifiedTypePrefix(predictionTokens("T"+strings.Repeat("/*c*/", declarationPrefixTokens)+";")) != 0 {
		t.Fatal("qualified scan exceeded hidden-token bound")
	}
}
