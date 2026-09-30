package yakast

import (
	"reflect"

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
	for _, c := range cases {
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
			y.VisitExpression(c.Channel)
			if c.Send != nil {
				y.VisitExpression(c.Send)
			} else {
				y.pushUndefined()
			}
		}
		y.pushListWithLen(3)
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
	var ends []*yakvm.Code
	clauses := stmt.AllSelectClause()
	for i, c := range cases {
		clause := clauses[i].(*yak.SelectClauseContext)
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
		if c.Channel != nil && c.Left != nil {
			leftCount := len(c.Left.(*yak.LeftExpressionListContext).AllLeftExpression())
			for j := 0; j < leftCount; j++ {
				resultField(j + 1)
			}
			y.pushListWithLen(leftCount)
			y.VisitLeftExpressionList(c.Declare, c.Left)
			y.pushOperator(yakvm.OpAssign)
		}
		if body := clause.StatementList(); body != nil {
			for _, raw := range body.(*yak.StatementListContext).AllStatement() {
				s := raw.(*yak.StatementContext)
				if s.Empty() != nil {
					continue
				}
				y.VisitStatement(s)
			}
		}
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
