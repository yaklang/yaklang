package antlr4util

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Token interface {
	GetStart() int
	GetStop() int
	GetLine() int
	GetColumn() int

	GetText() string
}
type CanStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func GetEndPosition(t Token) (int, int) {
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
	endToken := token.GetStop()
	if startToken == nil || endToken == nil {
		return nil
	}

	start := ssa.NewPosition(int64(startToken.GetStart()), int64(startToken.GetLine()), int64(startToken.GetColumn()))

	endLine, endColumn := GetEndPosition(endToken)
	end := ssa.NewPosition(int64(endToken.GetStop()), int64(endLine), int64(endColumn))

	return ssa.NewRange(start, end, token.GetText())
}
