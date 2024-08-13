package parser

import (
	"strings"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type YaklangLexerBase struct {
	*antlr.BaseLexer

	_heredocLF         string
	_heredocIdentifier string
	_templateDepth     uint64
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

func (l *YaklangLexer) recordHereDocCRLF() {
	stream := l.GetInputStream()
	preTextIndex := stream.Index() - 2
	text := stream.GetText(preTextIndex, preTextIndex+1)
	if text == "\r\n" {
		l._heredocLF = text
	} else {
		l._heredocLF = "\n"
	}
}

func (l *YaklangLexer) DocEndDistribute() bool {
	end := l.GetText()

	if !strings.HasPrefix(end, l._heredocLF) {
		l.SetType(YaklangLexerHereDocText)
		return false
	}
	stream := l.GetInputStream()
	index := stream.Index()
	nextText := stream.GetText(index, index+len(l._heredocIdentifier)-1)

	if l._heredocIdentifier == nextText {
		l.SetType(YaklangLexerEndDoc)
		l.PopMode()
		return true
	} else {
		l.SetType(YaklangLexerHereDocText)
		return false
	}
}
