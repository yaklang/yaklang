package php2ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

// findTrailingPHPCloseTag returns the rune-index of a trailing "?>"
// (only whitespace may follow it), or -1 if not found.
func findTrailingPHPCloseTag(runes []rune) int {
	s := string(runes)
	trimmed := strings.TrimRight(s, " \t\r\n")
	if !strings.HasSuffix(trimmed, "?>") {
		return -1
	}
	return len([]rune(trimmed)) - 2
}

// rewriteSingleSemicolonNamespaceUseBlock rewrites
//
//	namespace Foo;
//	use ...;
//
// into
//
//	namespace Foo {
//	use ...;
//	...
//	}
//
// It is intentionally narrow:
// - only one declaration-like `namespace` keyword token is allowed
// - the namespace must use the semicolon form
// - the first token after the namespace semicolon must be `use`
func rewriteSingleSemicolonNamespaceUseBlock(src string) (string, bool) {
	if src == "" {
		return src, false
	}

	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	tokens := make([]antlr.Token, 0, 256)
	for {
		tok := lexer.NextToken()
		if tok == nil {
			break
		}
		if tok.GetChannel() == antlr.TokenDefaultChannel || tok.GetTokenType() == antlr.TokenEOF {
			tokens = append(tokens, tok)
		}
		if tok.GetTokenType() == antlr.TokenEOF {
			break
		}
	}

	namespaceIndex := -1
	for i, tok := range tokens {
		if tok.GetTokenType() != phpparser.PHPLexerNamespace {
			continue
		}
		if i > 0 {
			switch tokens[i-1].GetTokenType() {
			case phpparser.PHPLexerObjectOperator, phpparser.PHPLexerNullsafeObjectOperator, phpparser.PHPLexerDoubleColon:
				continue
			}
		}
		if namespaceIndex >= 0 {
			return src, false
		}
		namespaceIndex = i
	}
	if namespaceIndex < 0 {
		return src, false
	}

	var semiTok antlr.Token
	for i := namespaceIndex + 1; i < len(tokens); i++ {
		switch tokens[i].GetTokenType() {
		case phpparser.PHPLexerOpenCurlyBracket:
			return src, false
		case phpparser.PHPLexerSemiColon:
			semiTok = tokens[i]
			if i+1 >= len(tokens) || tokens[i+1].GetTokenType() != phpparser.PHPLexerUse {
				return src, false
			}
			goto rewrite
		case antlr.TokenEOF:
			return src, false
		}
	}
	return src, false

rewrite:
	runes := []rune(src)
	semiPos := semiTok.GetStart()
	if semiPos < 0 || semiPos >= len(runes) {
		return src, false
	}
	runes[semiPos] = '{'

	insertPos := len(runes)
	for i := len(tokens) - 1; i >= 0; i-- {
		tok := tokens[i]
		switch tok.GetTokenType() {
		case antlr.TokenEOF:
			continue
		case phpparser.PHPLexerPHPEnd, phpparser.PHPLexerPHPEndSingleLineComment:
			insertPos = tok.GetStart()
		}
		break
	}
	// When the last statement before ?> is } the lexer consumes PHPEnd
	// entirely (converting it to the next HTML token), so the token-based
	// search above misses it.  Fall back to a direct source scan.
	if insertPos == len(runes) {
		if idx := findTrailingPHPCloseTag(runes); idx >= 0 {
			insertPos = idx
		}
	}
	if insertPos < 0 || insertPos > len(runes) {
		insertPos = len(runes)
	}

	out := make([]rune, 0, len(runes)+2)
	out = append(out, runes[:insertPos]...)
	out = append(out, '\n', '}')
	out = append(out, runes[insertPos:]...)
	return string(out), true
}
