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
	endLine, endColumn := yakast.GetEndPosition(token.GetStop())
	pos := &ssa.Position{
		SourceCode:  source,
		StartLine:   token.GetStart().GetLine(),
		StartColumn: token.GetStart().GetColumn(),
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
	backup := b.SetPosition(pos)

	return func() {
		b.SetPosition(backup)
	}
}
