package yakast

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

func (y *YakCompiler) SwitchSymbolTableInNewScope(name ...string) func() {
	origin := y.currentSymtbl
	y.currentSymtbl = origin.CreateSubSymbolTable(name...)
	y.pushScope(y.rootSymtbl.MustRoot().GetTableIndex())
	y.addContinueScopeCounter(1)
	y.addNearliestBreakScopeCounter(1)

	return func() {
		defer y.pushOperator(yakvm.OpScopeEnd)
		y.currentSymtbl = origin
		y.addContinueScopeCounter(-1)
		y.addNearliestBreakScopeCounter(-1)
	}
}

func (y *YakCompiler) addContinueScopeCounter(delta int) {
	if y.peekForStartIndex() >= 0 {
		y.peekForContext().continueScopeCounter += delta
	}
}

func (y *YakCompiler) getContinueScopeCounter() int {
	if y.peekForStartIndex() >= 0 {
		return y.peekForContext().continueScopeCounter
	}
	return 0
}

func (y *YakCompiler) addNearliestBreakScopeCounter(delta int) {
	if y.peekForStartIndex() >= 0 && y.peekSwitchStartIndex() >= 0 {
		if y.GetNextCodeIndex()-y.peekForStartIndex() > y.GetNextCodeIndex()-y.peekSwitchStartIndex() {
			// switch 离得近
			y.peekSwitchContext().switchBreakScopeCounter += delta
		} else {
			// for 离得近
			y.peekForContext().breakScopeCounter += delta
		}
		return
	}

	if y.peekForStartIndex() >= 0 {
		y.peekForContext().breakScopeCounter += delta
		return
	}

	if y.peekSwitchStartIndex() >= 0 {
		y.peekSwitchContext().switchBreakScopeCounter += delta
		return
	}
}

func (y *YakCompiler) getNearliestBreakScopeCounter() int {
	if y.peekForStartIndex() >= 0 && y.peekSwitchStartIndex() >= 0 {
		if y.GetNextCodeIndex()-y.peekForStartIndex() > y.GetNextCodeIndex()-y.peekSwitchStartIndex() {
			// switch 离得近
			return y.peekSwitchContext().switchBreakScopeCounter
		} else {
			// for 离得近
			return y.peekForContext().breakScopeCounter
		}
	}

	if y.peekForStartIndex() >= 0 {
		return y.peekForContext().breakScopeCounter
	}

	if y.peekSwitchStartIndex() >= 0 {
		return y.peekSwitchContext().switchBreakScopeCounter
	}
	return 0
}

func (y *YakCompiler) SwitchSymbolTable(name ...string) func() {
	origin := y.currentSymtbl
	y.currentSymtbl = origin.CreateSubSymbolTable(name...)
	return func() {
		y.currentSymtbl = origin
	}
}

func (y *YakCompiler) SwitchCodes() func() {
	origin := y.codes
	originRefName := y.FreeValues
	y.codes = []*yakvm.Code{}
	y.FreeValues = make([]int, 0)
	return func() {
		y.codes = origin
		y.FreeValues = originRefName
	}
}

type canStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
}

func (y *YakCompiler) GetRangeVerbose() string {
	var prefix string
	if y.currentStartPosition != nil && y.currentEndPosition != nil {
		prefix = fmt.Sprintf(`[%v:%v -> %v:%v]`,
			y.currentStartPosition.GetLine(),
			y.currentStartPosition.GetColumn(),
			y.currentEndPosition.GetLine(),
			y.currentEndPosition.GetColumn(),
		)
	}
	return prefix
}

// SetRange 设置当前解析到的范围，用来自动关联 Opcode 的 Range
func (y *YakCompiler) SetRange(b canStartStopToken) func() {
	originEndPosition := y._setCurrentEndPosition(b.GetStop())
	originStartPosition := y._setCurrentStartPosition(b.GetStart())
	return func() {
		y.currentStartPosition = originStartPosition
		y.currentEndPosition = originEndPosition
	}
}

func (y *YakCompiler) _setCurrentStartPosition(t antlr.Token) *memedit.Position {
	origin := y.currentStartPosition
	y.currentStartPosition = memedit.NewPosition(t.GetLine(), t.GetColumn())
	return origin
}

func (y *YakCompiler) _setCurrentEndPosition(t antlr.Token) *memedit.Position {
	origin := y.currentEndPosition
	line, column := GetEndPosition(t)
	if column > 0 {
		column--
	}
	y.currentEndPosition = memedit.NewPosition(line, column)
	return origin
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
