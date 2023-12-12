package js2ssa

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func GetEndPosition(t antlr.Token) (int, int) {
	var line, column int
	str := strings.Split(t.GetText(), "\n")
	if len(str) > 1 {
		line = t.GetLine() + len(str) - 1
		column = len(str[len(str)-1])
	} else {
		line = t.GetLine()
		column = t.GetColumn() + len(str[0])
	}
	return line, column
}

type canStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func (b *astbuilder) SetRange(token canStartStopToken) func() {
	source := strings.Split(token.GetText(), "\n")[0]
	start := token.GetStart()
	stop := token.GetStop()
	endline, endcolumn := GetEndPosition(stop)
	pos := &ssa.Position{
		SourceCode:  source,
		StartOffset: int64(start.GetStart()),
		StartLine:   start.GetLine(),
		StartColumn: start.GetColumn(),
		EndOffset:   int64(stop.GetStop()),
		EndLine:     endline,
		EndColumn:   endcolumn,
	}
	backup := b.SetPosition(pos)

	return func() {
		b.SetPosition(backup)
	}
}
