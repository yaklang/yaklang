package javaparser

import (
	"testing"

	antlr "github.com/yaklang/antlr/v4"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// JLS 3.8 allows a lone `$` as a Java identifier. The lexer must reserve only
// `${` for the package macro. Otherwise a valid obfuscated field such as
// `private ByteVector $;` lexes as the Dollar token and stops parsing
// (aspectjweaver aj/org/objectweb/asm/MethodWriter).
func TestBareDollarIsIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		wantDollar  bool
		wantIdent   int
		wantNoError bool
	}{
		{"field", "class A { private Object $; }", false, 1, true},
		{"member access", "class A { Object f() { return this.$; } }", false, 1, true},
		{"declaration and use", "class A { int $; int g() { return $; } }", false, 2, true},
		{"package macro", "package ${package}.p; class A {}", true, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lexer := NewJavaLexer(antlr.NewInputStream(tc.src))
			lexer.RemoveErrorListeners()
			stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
			stream.Fill()
			dollars, identifiers := 0, 0
			for _, token := range stream.GetAllTokens() {
				switch token.GetTokenType() {
				case JavaLexerDollar:
					dollars++
					if token.GetText() != "${" {
						t.Fatalf("Dollar token carries %q, want %q", token.GetText(), "${")
					}
				case JavaLexerIDENTIFIER:
					if token.GetText() == "$" {
						identifiers++
					}
				}
			}
			if tc.wantDollar != (dollars > 0) {
				t.Fatalf("dollar tokens = %d, want presence %v (tokens: %v)", dollars, tc.wantDollar, tokenTexts(stream))
			}
			if identifiers != tc.wantIdent {
				t.Fatalf("bare-$ identifiers = %d, want %d (tokens: %v)", identifiers, tc.wantIdent, tokenTexts(stream))
			}
			ast, err := antlr4util.ParseASTWithSLLFirst(
				tc.src,
				NewJavaLexer,
				NewJavaParser,
				nil,
				nil,
				func(parser *JavaParser) ICompilationUnitContext { return parser.CompilationUnit() },
			)
			if tc.wantNoError && (err != nil || ast == nil) {
				t.Fatalf("parse failed: %v", err)
			}
		})
	}
}

func tokenTexts(stream *antlr.CommonTokenStream) []string {
	var out []string
	for _, token := range stream.GetAllTokens() {
		out = append(out, token.GetText())
	}
	return out
}
