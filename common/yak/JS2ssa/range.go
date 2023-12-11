package js2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type CanStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

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

func GetRange(token CanStartStopToken) *ssa.Range {
	startToken := token.GetStart()
	start := ssa.NewPosition(int64(startToken.GetStart()), int64(startToken.GetLine()), int64(startToken.GetColumn()))

	endToken := token.GetStop()
	endLine, endColumn := GetEndPosition(endToken)
	end := ssa.NewPosition(int64(endToken.GetStop()), int64(endLine), int64(endColumn))

	return ssa.NewRange(start, end, token.GetText())
}

func (b *astbuilder) SetRange(token CanStartStopToken) func() {

	// TODO: use antlr4utils.GetRange, when new JS parser ready
	r := GetRange(token)
	backup := b.CurrentRange
	b.CurrentRange = r

	return func() {
		b.CurrentRange = backup
	}
}
