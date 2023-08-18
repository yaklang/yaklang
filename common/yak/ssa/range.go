package ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type canStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func (b *builder) SetRange(token canStartStopToken) func() {
	pos := b.currtenPos
	// fmt.Printf("debug %v\n", b.GetText())
	source := strings.Split(token.GetText(), "\n")[0]
	b.currtenPos = &Position{
		SourceCode:  source,
		StartLine:   token.GetStart().GetLine(),
		StartColumn: token.GetStart().GetColumn(),
		EndLine:     token.GetStop().GetLine(),
		EndColumn:   token.GetStop().GetColumn(),
	}

	return func() {
		b.currtenPos = pos
	}
}
