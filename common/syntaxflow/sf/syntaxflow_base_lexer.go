package sf

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"strings"
)

type SyntaxFlowBaseLexer struct {
	*antlr.BaseLexer

	_heredocIdentifier, _heredocCRLF string
}

func (l *SyntaxFlowLexer) recordHereDocLabel() {
	l._heredocIdentifier = l.GetText()
}

func (l *SyntaxFlowLexer) recordHereDocLF() {
	l._heredocCRLF = l.GetText()
}

func (l *SyntaxFlowLexer) DocEndDistribute() bool {
	text := l.GetText()
	if strings.HasSuffix(text, l._heredocCRLF+l._heredocIdentifier) {
		l.PopMode()
		return true
	} else {
		if l._heredocCRLF == "\r\n" {
			l.SetType(SyntaxFlowLexerCRLFHereDocText)
		} else {
			l.SetType(SyntaxFlowLexerLFHereDocText)
		}
		return false
	}
}
