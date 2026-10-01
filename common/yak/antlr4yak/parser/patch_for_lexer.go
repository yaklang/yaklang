package parser

import (
	"strings"
	"sync"

	"github.com/yaklang/antlr/v4"
)

type YaklangLexerBase struct {
	*antlr.BaseLexer

	_heredocIdentifier string
	_heredocCRLF       string
	_templateDepth     uint64
	_templateBraces    []uint64
	waitForCloseToken  antlr.Token
}

var templateDepthMap = new(sync.Map)

func (l *YaklangLexerBase) clearInputState() {
	l._heredocIdentifier, l._heredocCRLF = "", ""
	l._templateDepth = 0
	l._templateBraces = l._templateBraces[:0]
	l.waitForCloseToken = nil
}

func (l *YaklangLexerBase) Reset() {
	l.BaseLexer.Reset()
	l.clearInputState()
}

func (l *YaklangLexerBase) SetInputStream(input antlr.CharStream) {
	l.BaseLexer.SetInputStream(input)
	l.clearInputState()
}

func (l *YaklangLexer) DecreaseTemplateDepth() {
	l._templateDepth--
	l._templateBraces = l._templateBraces[:len(l._templateBraces)-1]
}

func (l *YaklangLexer) IncreaseTemplateDepth() {
	l._templateDepth++
	l._templateBraces = append(l._templateBraces, 0)
}

func (l *YaklangLexer) IsInTemplateString() bool {
	// A brace inside a map or closure belongs to that expression. Only a
	// brace at the current template's base depth closes its interpolation.
	return l._templateDepth > 0 && l._templateBraces[len(l._templateBraces)-1] == 0
}

func (l *YaklangLexer) recordHereDocLabel() {
	// The action runs at the token end, after both optional quotes. Capture
	// the complete label regardless of the shared DFA's preceding inputs.
	l._heredocIdentifier = strings.Trim(l.GetText(), "'")
}

func (l *YaklangLexer) recordHereDocLF() {
	l._heredocCRLF = l.GetText()
}

func (l *YaklangLexer) hereDocModeDistribute() {
	l.PopMode()
	if l._heredocCRLF == "\r\n" {
		l.PushMode(YaklangLexerCRLFHereDoc)
	} else {
		l.PushMode(YaklangLexerLFHereDoc)
	}
}

func (l *YaklangLexer) DocEndDistribute() bool {
	text := l.GetText()
	if strings.HasSuffix(text, l._heredocCRLF+l._heredocIdentifier) {
		l.PopMode()
		return true
	} else {
		if l._heredocCRLF == "\r\n" {
			l.SetType(YaklangLexerCRLFHereDocText)
		} else {
			l.SetType(YaklangLexerLFHereDocText)
		}
		return false
	}
}

func (l *YaklangLexerBase) NextToken() antlr.Token {
	if l.waitForCloseToken != nil {
		next := l.waitForCloseToken
		l.waitForCloseToken = nil
		return next
	}

	next := l.BaseLexer.NextToken()
	if l._templateDepth > 0 {
		depth := &l._templateBraces[len(l._templateBraces)-1]
		switch next.GetTokenType() {
		case YaklangLexerLBrace:
			*depth++
		case YaklangLexerRBrace:
			*depth--
		}
	}

	if next.GetTokenType() == YaklangLexerRBrace || next.GetTokenType() == -1 {
		semit := l.GetTokenFactory().Create(
			l.GetTokenSourceCharStreamPair(), YaklangLexerSemiColon, ";", next.GetChannel(),
			next.GetStart(), next.GetStop()-1,
			next.GetLine(), next.GetColumn(),
		)
		l.waitForCloseToken = next
		next = semit
	}

	return next
}
