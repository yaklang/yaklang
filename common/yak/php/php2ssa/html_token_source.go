package php2ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

type htmlCoalescingTokenSource struct {
	antlr.TokenSource
	pending antlr.Token
}

func newHTMLCoalescingTokenSource(source antlr.TokenSource) antlr.TokenSource {
	return &htmlCoalescingTokenSource{TokenSource: source}
}

func (s *htmlCoalescingTokenSource) NextToken() antlr.Token {
	first := s.nextRawToken()
	if first == nil || first.GetTokenType() == antlr.TokenEOF {
		return first
	}
	if !isHTMLChunkToken(first) {
		return first
	}

	var text strings.Builder
	text.WriteString(first.GetText())

	for {
		next := s.nextRawToken()
		if next == nil {
			break
		}
		if !isHTMLChunkToken(next) {
			s.pending = next
			break
		}
		text.WriteString(next.GetText())
	}

	first.SetText(text.String())
	return first
}

func (s *htmlCoalescingTokenSource) nextRawToken() antlr.Token {
	if s.pending != nil {
		token := s.pending
		s.pending = nil
		return token
	}
	return s.TokenSource.NextToken()
}

func isHTMLChunkToken(token antlr.Token) bool {
	if token == nil {
		return false
	}

	switch token.GetTokenType() {
	case antlr.TokenEOF,
		phpparser.PHPLexerXmlStart,
		phpparser.PHPLexerXmlText,
		phpparser.PHPLexerXmlClose,
		phpparser.PHPLexerPHPStart,
		phpparser.PHPLexerPHPStartInside,
		phpparser.PHPLexerPHPStartInsideQuoteString,
		phpparser.PHPLexerPHPStartDoubleQuoteString,
		phpparser.PHPLexerPHPStartInsideScript,
		phpparser.PHPLexerError,
		phpparser.PHPLexerErrorInside,
		phpparser.PHPLexerErrorHtmlQuote,
		phpparser.PHPLexerErrorHtmlDoubleQuote:
		return false
	}

	return token.GetTokenType() >= phpparser.PHPLexerSeaWhitespace &&
		token.GetTokenType() <= phpparser.PHPLexerStyleBody
}
