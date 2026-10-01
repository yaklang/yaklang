package java2ssa

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

func TestFrontendPredictionRejectsInvalidSource(t *testing.T) {
	for _, source := range []string{
		`"0000000000000000000000000000000000000`,
		"class C { int x = #1; }", "class C { int x = ; int y = #2; }",
		"class C {} junk", "class C {} @", "class C {} 1", "class C { int x=1; } /* unfinished",
		"class C { int x = ; }", "class C { void m(){ return (x; } }",
		"package p; class C {} package p; class D {}",
	} {
		t.Run(source, func(t *testing.T) {
			for _, cached := range []bool{false, true} {
				builder := CreateBuilder().(*SSABuilder)
				if cached {
					_, err := Frontend(source, builder.GetAntlrCache())
					if err == nil {
						t.Fatal("cached frontend accepted invalid source")
					}
				} else if _, err := Frontend(source); err == nil {
					t.Fatal("frontend accepted invalid source")
				}
				if _, err := Frontend("class Valid { int m(){return 1;} }", builder.GetAntlrCache()); err != nil {
					t.Fatalf("failure leaked into next input: %v", err)
				}
				builder.Clearup()
			}
		})
	}
	for _, source := range []string{"class C { int x=#1; }", "class C { int x=; int y=#2; }"} {
		_, err := Frontend(source)
		if err == nil || !strings.Contains(err.Error(), "token recognition error") {
			t.Fatalf("lexical error disappeared: %v", err)
		}
	}
}

func frontendSnapshot(tree antlr.Tree) string {
	var out strings.Builder
	stack := []antlr.Tree{tree}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := node.(type) {
		case antlr.ParserRuleContext:
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d\n", n, n.GetRuleIndex(), n.GetChildCount(), n.GetStart().GetTokenIndex(), n.GetStop().GetTokenIndex())
		case antlr.TerminalNode:
			v := n.GetSymbol()
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d:%d:%d:%q\n", n, v.GetTokenType(), v.GetTokenIndex(), v.GetStart(), v.GetStop(), v.GetLine(), v.GetColumn(), v.GetText())
		}
		children := node.GetChildren()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return out.String()
}

func TestFrontendPredictionASTEquivalence(t *testing.T) {
	for _, source := range []string{
		"class C { T<X<Y>> x; Object m(){return f(x -> f(x));} }",
		"class C { void m(){ if(x) if(y) f(); else g(); T<? extends A[]> x; }}",
		"class C { Object m(){return (T & U) x -> x;} }",
		"record R(T x) { Object m(){ return switch(x){case 1 -> 2; default -> 3;};} }",
		"package a; import a.b.C; public class D { @A T @B [] x; Object m(){return T[]::new;} }",
		"open module a { requires transitive b; exports a.b to c; }",
	} {
		t.Run(source, func(t *testing.T) {
			want, err := antlr4util.ParseASTWithSLLFirst(source, javaparser.NewJavaLexer, func(input antlr.TokenStream) *javaparser.JavaParser {
				p := javaparser.NewJavaParser(input)
				p.SetFastPrediction(false)
				return p
			}, nil, nil, func(p *javaparser.JavaParser) javaparser.ICompilationUnitContext { return p.CompilationUnit() })
			if err != nil {
				t.Fatal(err)
			}
			got, err := Frontend(source)
			if err != nil {
				t.Fatal(err)
			}
			if frontendSnapshot(want) != frontendSnapshot(got) {
				t.Fatal("frontend changed exact AST")
			}
		})
	}
}

func TestFrontendPredictionConcurrentCaches(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			cache := builder.GetAntlrCache()
			for j := 0; j < 10; j++ {
				if _, err := Frontend("class C { T<X<Y>> x; Object m(){return f(a -> a);} }", cache); err != nil {
					t.Errorf("concurrent frontend: %v", err)
					return
				}
				if _, err := Frontend("class C { int x=#1; }", cache); err == nil {
					t.Error("invalid source accepted")
					return
				}
			}
		}()
	}
	wg.Wait()
}
