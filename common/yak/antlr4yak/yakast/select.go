package yakast

import (
	"reflect"
	"strings"

	"github.com/yaklang/antlr/v4"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitSelectStmt(raw yak.ISelectStmtContext) interface{} {
	stmt := raw.(*yak.SelectStmtContext)
	restoreRange := y.SetRange(stmt)
	defer restoreRange()
	cases, err := yak.SelectCommunications(stmt)
	if err != nil {
		y.panicCompilerError(compileError, err.Error())
	}
	if len(cases) > 65535 {
		y.panicCompilerError(compileError, "select has too many cases")
	}
	restoreScope := y.SwitchSymbolTableInNewScope("select")
	defer restoreScope()
	// Reuse break patching/scope accounting; case bodies remain in this function.
	y.enterSwitchContext(y.GetNextCodeIndex())
	resultID := y.currentSymtbl.NewSymbolWithoutName()
	y.pushIdentifierName(yakvm.SelectBuiltinName)
	headings := make([]string, len(cases))
	for i, c := range cases {
		restoreFormat := y.switchFormatBuffer()
		if c.Channel == nil {
			y.pushInteger(int(reflect.SelectDefault), "")
			y.pushUndefined()
			y.pushUndefined()
		} else {
			dir := reflect.SelectRecv
			if c.Send != nil {
				dir = reflect.SelectSend
			}
			y.pushInteger(int(dir), "")
			if c.Send == nil {
				y.writeString("<-")
			}
			y.VisitExpression(c.Channel)
			if c.Send != nil {
				y.writeString(" <- ")
				y.VisitExpression(c.Send)
			} else {
				y.pushUndefined()
			}
		}
		y.pushListWithLen(3)
		headings[i] = restoreFormat()
	}
	if len(cases) == 0 {
		// OpList(0) is a no-op, whereas OpNewSlice(0) creates an empty argument.
		y.pushOperator(yakvm.OpNewSlice)
	} else {
		y.pushListWithLen(len(cases))
	}
	y.pushCall(1)
	y.pushListWithLen(1)
	y.pushLeftRef(resultID)
	y.pushListWithLen(1)
	y.pushOperator(yakvm.OpAssign)

	resultField := func(index int) {
		y.pushRef(resultID)
		y.pushInteger(index, "")
		y.pushBool(false)
		y.pushIterableCall(1)
	}
	var bodyJumps, invalidJumps []*yakvm.Code
	if len(cases) >= 16 {
		// Large selects dispatch by a balanced decision tree. Keep small
		// selects' short linear path and avoid repeatedly indexing the tuple.
		chosenID := y.currentSymtbl.NewSymbolWithoutName()
		resultField(0)
		y.pushListWithLen(1)
		y.pushLeftRef(chosenID)
		y.pushListWithLen(1)
		y.pushOperator(yakvm.OpAssign)
		bodyJumps, invalidJumps = y.selectDispatchTree(chosenID, len(cases))
	}
	y.writeString("select {")
	y.writeNewLine()
	var ends []*yakvm.Code
	wsIndex := 0
	whitespace := stmt.AllWs()
	clauses := stmt.AllSelectClause()
	for i, c := range cases {
		clause := clauses[i].(*yak.SelectClauseContext)
		for wsIndex < len(whitespace) && whitespace[wsIndex].GetStart().GetTokenIndex() < clause.GetStart().GetTokenIndex() {
			y.writeSelectComments(whitespace[wsIndex])
			wsIndex++
		}
		restoreRange := y.SetRange(clause)
		var next *yakvm.Code
		if bodyJumps == nil {
			resultField(0)
			y.pushInteger(i, "")
			y.pushOperator(yakvm.OpEq)
			next = y.pushJmpIfFalse()
		} else {
			bodyJumps[i].Unary = y.GetNextCodeIndex()
		}
		restoreCase := y.SwitchSymbolTableInNewScope("select case")
		y.writeIndent()
		if c.Channel == nil {
			y.writeString("default")
		} else {
			y.writeString("case ")
			if c.Left != nil {
				leftCount := len(c.Left.(*yak.LeftExpressionListContext).AllLeftExpression())
				for j := 0; j < leftCount; j++ {
					resultField(j + 1)
				}
				y.pushListWithLen(leftCount)
				y.VisitLeftExpressionList(c.Declare, c.Left)
				if c.Declare {
					y.writeString(" := ")
				} else {
					y.writeString(" = ")
				}
				y.pushOperator(yakvm.OpAssign)
			}
			y.writeString(headings[i])
		}
		y.writeString(":")
		y.writeNewLine()
		y.incIndent()
		if body := clause.StatementList(); body != nil {
			for _, raw := range body.(*yak.StatementListContext).AllStatement() {
				s := raw.(*yak.StatementContext)
				if s.Empty() != nil {
					y.writeSelectComments(s.Empty())
					continue
				}
				restoreFormat := y.switchFormatBuffer()
				y.writeIndent()
				y.VisitStatement(s)
				y.writeString(strings.TrimRight(restoreFormat(), " \t\r\n"))
				y.writeNewLine()
			}
		}
		y.decIndent()
		restoreCase()
		ends = append(ends, y.pushJmp())
		if next != nil {
			next.Unary = y.GetNextCodeIndex()
		}
		restoreRange()
	}
	end := y.GetNextCodeIndex()
	for _, jump := range ends {
		jump.Unary = end
	}
	for _, jump := range invalidJumps {
		jump.Unary = end
	}
	y.exitSwitchContext(end)
	for ; wsIndex < len(whitespace); wsIndex++ {
		y.writeSelectComments(whitespace[wsIndex])
	}
	y.writeStringWithIndent("}")
	return nil
}

// The tree changes only dispatch, after all communication operands have already
// been evaluated. Leaf equality checks preserve the linear path's behavior for
// invalid indexes, including an interrupted selection's -1 result.
func (y *YakCompiler) selectDispatchTree(chosenID, count int) ([]*yakvm.Code, []*yakvm.Code) {
	bodies := make([]*yakvm.Code, count)
	invalid := make([]*yakvm.Code, 0, count)
	var emit func(int, int)
	emit = func(lo, hi int) {
		if hi-lo == 1 {
			y.pushRef(chosenID)
			y.pushInteger(lo, "")
			y.pushOperator(yakvm.OpEq)
			invalid = append(invalid, y.pushJmpIfFalse())
			bodies[lo] = y.pushJmp()
			return
		}
		mid := lo + (hi-lo)/2
		y.pushRef(chosenID)
		y.pushInteger(mid, "")
		y.pushOperator(yakvm.OpLt)
		right := y.pushJmpIfFalse()
		emit(lo, mid)
		right.Unary = y.GetNextCodeIndex()
		emit(mid, hi)
	}
	emit(0, count)
	return bodies, invalid
}

func (y *YakCompiler) writeSelectComments(tree antlr.Tree) {
	if token, ok := tree.(antlr.TerminalNode); ok {
		switch token.GetSymbol().GetTokenType() {
		case yak.YaklangParserCOMMENT, yak.YaklangParserLINE_COMMENT:
			y.writeStringWithIndent(strings.TrimSpace(token.GetText()))
			y.writeNewLine()
		}
		return
	}
	for _, child := range tree.GetChildren() {
		y.writeSelectComments(child)
	}
}
