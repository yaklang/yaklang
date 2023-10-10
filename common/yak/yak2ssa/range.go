package yak2ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type canStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func (b *astbuilder) SetRange(token canStartStopToken) func() {
	// fmt.Printf("debug %v\n", b.GetText())
	source := strings.Split(token.GetText(), "\n")[0]
	end := token.GetStop()
	start := token.GetStart()
	if end == nil || start == nil {
		return func() {}
	}
	endLine, endColumn := yakast.GetEndPosition(end)
	pos := &ssa.Position{
		SourceCode:  source,
		StartLine:   start.GetLine(),
		StartColumn: start.GetColumn(),
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
	backup := b.SetPosition(pos)

	return func() {
		b.SetPosition(backup)
	}
}
