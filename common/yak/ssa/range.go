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

func (f *Function) SetRange(b canStartStopToken) func() {
	pos := f.currtenPos
	// fmt.Printf("debug %v\n", b.GetText())
	source := strings.Split(b.GetText(), "\n")[0]
	f.currtenPos = &Position{
		SourceCode:  source,
		StartLine:   b.GetStart().GetLine(),
		StartColumn: b.GetStart().GetColumn(),
		EndLine:     b.GetStop().GetLine(),
		EndColumn:   b.GetStop().GetColumn(),
	}

	return func() {
		f.currtenPos = pos
	}
}
