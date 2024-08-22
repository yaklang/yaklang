package parser

import (
	"strings"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type YaklangLexerBase struct {
	*antlr.BaseLexer

	_heredocIdentifier string
	_heredocCRLF       string
	_templateDepth     uint64
	waitForCloseToken  antlr.Token
}

var templateDepthMap = new(sync.Map)

func (l *YaklangLexer) DecreaseTemplateDepth() {
	l._templateDepth--
}

func (l *YaklangLexer) IncreaseTemplateDepth() {
	l._templateDepth++
}

func (l *YaklangLexer) IsInTemplateString() bool {
	return l._templateDepth > 0
}

func (l *YaklangLexer) recordHereDocLabel() {
	l._heredocIdentifier = l.GetText()
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
