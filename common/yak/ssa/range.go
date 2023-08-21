package ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
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
	endline, endcolumn := yakast.GetEndPosition(token.GetStop())
	b.currtenPos = &Position{
		SourceCode:  source,
		StartLine:   token.GetStart().GetLine(),
		StartColumn: token.GetStart().GetColumn(),
		EndLine:     endline,
		EndColumn:   endcolumn,
	}

	return func() {
		b.currtenPos = pos
	}
}
