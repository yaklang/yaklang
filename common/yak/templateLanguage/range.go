package templateLanguage

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type CanStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func (y *Visitor) SetRange(token CanStartStopToken) func() {
	if y.Editor == nil {
		return func() {}
	}
	r := GetRange(y.Editor, token)
	if r == nil {
		return func() {}
	}
	backup := y.CurrentRange
	y.CurrentRange = r

	return func() {
		y.CurrentRange = backup
	}
}

func GetRange(editor *memedit.MemEditor, token CanStartStopToken) *memedit.Range {
	startToken := token.GetStart()
	endToken := token.GetStop()
	if startToken == nil || endToken == nil {
		return nil
	}

	endLine, endColumn := GetEndPosition(endToken)
	return editor.GetRangeByPosition(
		editor.GetPositionByLine(startToken.GetLine(), startToken.GetColumn()+1),
		editor.GetPositionByLine(endLine, endColumn+1),
	)
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
