package jsp

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
)

type mixedContentCoalescingTokenSource struct {
	antlr.TokenSource
	pending antlr.Token
}

func newMixedContentCoalescingTokenSource(source antlr.TokenSource) antlr.TokenSource {
	return &mixedContentCoalescingTokenSource{TokenSource: source}
}

func (s *mixedContentCoalescingTokenSource) NextToken() antlr.Token {
	first := s.nextRawToken()
	if first == nil || first.GetTokenType() == antlr.TokenEOF {
		return first
	}
	if !isMixedContentToken(first) {
		return first
	}

	var text strings.Builder
	text.WriteString(first.GetText())
	mergedType := first.GetTokenType()
	lastStop := first.GetStop()

	for {
		next := s.nextRawToken()
		if next == nil {
			break
		}
		if !isMixedContentToken(next) {
			s.pending = next
			break
		}
		if next.GetTokenType() == jspparser.JSPLexerJSP_STATIC_CONTENT_CHARS {
			mergedType = jspparser.JSPLexerJSP_STATIC_CONTENT_CHARS
		}
		lastStop = next.GetStop()
		text.WriteString(next.GetText())
	}

	if mergedType != first.GetTokenType() {
		merged := antlr.NewCommonToken(first.GetSource(), mergedType, first.GetChannel(), first.GetStart(), lastStop)
		merged.SetText(text.String())
		return merged
	}

	first.SetText(text.String())
	return first
}

func (s *mixedContentCoalescingTokenSource) nextRawToken() antlr.Token {
	if s.pending != nil {
		token := s.pending
		s.pending = nil
		return token
	}
	return s.TokenSource.NextToken()
}

func isMixedContentToken(token antlr.Token) bool {
	if token == nil {
		return false
	}
	switch token.GetTokenType() {
	case jspparser.JSPLexerJSP_STATIC_CONTENT_CHARS, jspparser.JSPLexerWHITESPACES:
		return true
	default:
		return false
	}
}
