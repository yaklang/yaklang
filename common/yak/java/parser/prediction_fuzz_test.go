package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
)

// Restrict the random oracle's resource use, rather than excluding invalid
// syntax. Larger generic, parameter and recursion bounds have fixed tests.
func predictionFuzzInput(source string) bool {
	if len(source) > 4096 {
		return false
	}
	depth := 0
	for _, r := range source {
		switch r {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		}
		if depth > 48 {
			return false
		}
	}
	return true
}

func predictionMutation(source string, position, action int) string {
	lexer := NewJavaLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokens.Fill()
	var visible []antlr.Token
	for _, token := range tokens.GetAllTokens() {
		if token.GetTokenType() != antlr.TokenEOF && token.GetChannel() == antlr.TokenDefaultChannel {
			visible = append(visible, token)
		}
	}
	if len(visible) == 0 {
		return source
	}
	token := visible[position%len(visible)]
	runes := []rune(source)
	start, end := token.GetStart(), token.GetStop()+1
	if start < 0 || end > len(runes) {
		return source
	}
	replacement := []string{"", ")", ";", "->", "record", "<", "[]", "/*c*/"}[action%8]
	return string(runes[:start]) + replacement + string(runes[end:])
}

func TestPredictionTokenMutations(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		source := methodSource("T<X> a=f(x -> x); return " + expr + ";")
		for action := 0; action < 8; action++ {
			for _, position := range []int{i + 1, i + 7} {
				mutated := predictionMutation(source, position, action)
				t.Run(fmt.Sprintf("%03d_%d_%d", i, action, position), func(t *testing.T) { checkPrediction(t, mutated, reference, false) })
			}
		}
	}
}

func FuzzPrediction(f *testing.F) {
	for _, expr := range predictionExpressions {
		f.Add(methodSource("return "+expr+";"), uint16(0), uint8(0))
	}
	for _, stmt := range predictionStatements {
		f.Add(methodSource(stmt), uint16(0), uint8(0))
	}
	for _, source := range []string{"", "class", "class C {}", "class C { int a=; }", "class C {} junk", "class C { void f( }", "class C { /*", "module a { requires b; }", "@A package a; import b.C; class D {}", "class C { Object m(){ return f((x,y)->x+y); } }"} {
		f.Add(source, uint16(0), uint8(0))
	}
	reference := newPredictionAutomata()
	f.Fuzz(func(t *testing.T, source string, position uint16, action uint8) {
		if !predictionFuzzInput(source) {
			return
		}
		// Half of mutations preserve the source exactly. The other half use
		// token boundaries from the real lexer, so edits target grammar rules.
		if action&1 != 0 {
			source = predictionMutation(source, int(position), int(action>>1))
		}
		if !predictionFuzzInput(source) {
			return
		}
		checkPrediction(t, source, reference, false)
		// Check that failures and cached prefix decisions cannot affect the next
		// parser instance, including source changes at the same token index.
		if strings.Contains(source, "<") {
			got := parsePrediction(methodSource("T<X> x; return x;"), true, nil)
			if got.err != nil || got.next != antlr.TokenEOF {
				t.Fatalf("state leaked after %q: %v", source, got.err)
			}
		}
	})
}
