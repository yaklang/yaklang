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
	y.writeString("select {")
	y.writeNewLine()
	var ends []*yakvm.Code
	wsIndex := 0
	whitespace := stmt.AllWs()
	for i, c := range cases {
		clause := stmt.SelectClause(i).(*yak.SelectClauseContext)
		for wsIndex < len(whitespace) && whitespace[wsIndex].GetStart().GetTokenIndex() < clause.GetStart().GetTokenIndex() {
			y.writeSelectComments(whitespace[wsIndex])
			wsIndex++
		}
		restoreRange := y.SetRange(clause)
		resultField(0)
		y.pushInteger(i, "")
		y.pushOperator(yakvm.OpEq)
		next := y.pushJmpIfFalse()
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
		next.Unary = y.GetNextCodeIndex()
		restoreRange()
	}
	end := y.GetNextCodeIndex()
	for _, jump := range ends {
		jump.Unary = end
	}
	y.exitSwitchContext(end)
	for ; wsIndex < len(whitespace); wsIndex++ {
		y.writeSelectComments(whitespace[wsIndex])
	}
	y.writeStringWithIndent("}")
	return nil
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
