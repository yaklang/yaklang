package yakast

import (
	"strings"
	yak "yaklang/common/yak/antlr4yak/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitAssignExpressionStmt(raw yak.IAssignExpressionStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AssignExpressionStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if i.AssignExpression() != nil {
		y.VisitAssignExpression(i.AssignExpression())
	}

	return nil
}

func (y *YakCompiler) VisitAssignExpression(raw yak.IAssignExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AssignExpressionContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	// assign eq  /  colon assign eq
	if op, op2 := i.AssignEq(), i.ColonAssignEq(); op != nil || op2 != nil {

		// 赋值语句，应该先 visit 右值，再 visit 左值
		recoverFormatBufferFunc := y.switchFormatBuffer()
		y.VisitExpressionList(i.ExpressionList())
		buf := recoverFormatBufferFunc()

		forceAssign := op2 != nil
		y.VisitLeftExpressionList(forceAssign, i.LeftExpressionList())
		if op != nil {
			y.writeStringWithWhitespace(op.GetText())
		} else {
			y.writeStringWithWhitespace(op2.GetText())
		}
		y.writeString(buf)

		y.pushOperator(yakvm.OpAssign)
		return nil
	}

	if i.PlusPlus() != nil { // ++
		y.VisitLeftExpression(false, i.LeftExpression())
		y.writeString("++")
		y.pushOperator(yakvm.OpPlusPlus)
		return nil
	} else if i.SubSub() != nil { // --
		y.VisitLeftExpression(false, i.LeftExpression())
		y.writeString("--")
		y.pushOperator(yakvm.OpMinusMinus)
		return nil
	}

	if op := i.InplaceAssignOperator(); op != nil {
		recoverFormatBufferFunc := y.switchFormatBuffer()
		y.VisitExpression(i.Expression())
		buf := recoverFormatBufferFunc()
		y.VisitLeftExpression(false, i.LeftExpression())
		y.writeStringWithWhitespace(op.GetText())
		y.writeString(buf)
		switch op.GetText() {
		case "+=":
			y.pushOperator(yakvm.OpPlusEq)
		case "-=":
			y.pushOperator(yakvm.OpMinusEq)
		case "*=":
			y.pushOperator(yakvm.OpMulEq)
		case "/=":
			y.pushOperator(yakvm.OpDivEq)
		case "%=":
			y.pushOperator(yakvm.OpModEq)
		case "<<=":
			y.pushOperator(yakvm.OpShlEq)
		case ">>=":
			y.pushOperator(yakvm.OpShrEq)
		case "&=":
			y.pushOperator(yakvm.OpAndEq)
		case "|=":
			y.pushOperator(yakvm.OpOrEq)
		case "^=":
			y.pushOperator(yakvm.OpXorEq)
		case "&^=":
			y.pushOperator(yakvm.OpAndNotEq)
		default:
			y.panicCompilerError(notImplemented, op.GetText())
		}
	}

	return nil
}

func (y *YakCompiler) VisitLeftExpressionList(forceNewSymbol bool, raw yak.ILeftExpressionListContext) int {
	if y == nil || raw == nil {
		return -1
	}

	i, _ := raw.(*yak.LeftExpressionListContext)
	if i == nil {
		return -1
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	allExpr := i.AllLeftExpression()
	lenOfAllExpr := len(allExpr)
	for index, le := range allExpr {
		y.VisitLeftExpression(forceNewSymbol, le)
		if index < lenOfAllExpr-1 {
			y.writeString(", ")
		}
	}
	if allExpr != nil {
		y.pushListWithLen(len(allExpr))
	}

	return len(allExpr)
}

func (y *YakCompiler) VisitLeftExpression(forceNewSymbol bool, raw yak.ILeftExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.LeftExpressionContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if id := i.Identifier(); id != nil {
		// 左值中的 identifier 要创建符号
		idName := id.GetText()
		y.writeString(idName)
		if forceNewSymbol {
			id, err := y.currentSymtbl.NewSymbolWithReturn(idName)
			if err != nil {
				y.panicCompilerError(forceCreateSymbolFailed, idName)
			}
			y.pushLeftRef(id)
			return nil
		}
		var sym, ok = y.currentSymtbl.GetSymbolByVariableName(idName)
		if !ok {
			var err error
			sym, err = y.currentSymtbl.NewSymbolWithReturn(idName)
			if err != nil {
				y.panicCompilerError(autoCreateSymbolFailed, idName)
			}
		}
		y.pushLeftRef(sym)
		return nil
	}

	if e := i.Expression(); e != nil {
		y.VisitExpression(e)

		/*
			a[1] = 1

			push 1
			list 1

			push slice
			push index
			list 2  // 组合 slice[index] 放入栈
			list 1

			assign
		*/
		if s := i.LeftSliceCall(); s != nil {
			y.VisitLeftSliceCall(s)
			// 这里复用 list 的操作，由 assign 判断后续的值
			// list 2 可以把上述两个值组合成一个可以被赋值的操作
			// 一定小心，不要乱改
			y.pushListWithLen(2)
			return nil
		}

		if m := i.LeftMemberCall(); m != nil {
			y.VisitLeftMemberCall(m)
			y.pushListWithLen(2)
			return nil
		}
		return nil
	}

	return nil
}

func (y *YakCompiler) VisitLeftMemberCall(raw yak.ILeftMemberCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.LeftMemberCallContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	y.writeString(".")
	if i := i.Identifier(); i != nil {
		idName := i.GetText()
		y.writeString(idName)
		y.pushString(idName, idName)
		return nil
	}

	if i := i.IdentifierWithDollar(); i != nil {
		varName := i.GetText()
		y.writeString(varName)
		for strings.HasPrefix(varName, "$") {
			varName = varName[1:]
		}
		symbolId, ok := y.currentSymtbl.GetSymbolByVariableName(varName)
		if !ok {
			y.panicCompilerError(notFoundDollarVariable, varName)
		}
		y.pushRef(symbolId)
		return nil
	}

	y.panicCompilerError(bugMembercall)
	return nil

}

func (y *YakCompiler) VisitLeftSliceCall(raw yak.ILeftSliceCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.LeftSliceCallContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	y.writeString("[")
	y.VisitExpression(i.Expression())
	y.writeString("]")

	return nil
}

func (y *YakCompiler) VisitDeclearVariableExpression(raw yak.IDeclearVariableExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclearVariableExpressionContext)
	if i == nil {
		return nil
	}

	if s := i.DeclearVariableOnly(); s != nil {
		y.VisitDeclearVariableOnly(s)
		return nil
	}

	if s := i.DeclearAndAssignExpression(); s != nil {
		y.VisitDeclearAndAssignExpression(s)
		return nil
	}

	return nil
}

func (y *YakCompiler) VisitDeclearAndAssignExpression(raw yak.IDeclearAndAssignExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclearAndAssignExpressionContext)
	if i == nil {
		return nil
	}

	y.writeString("var ")

	// 赋值语句，应该先 visit 右值，再 visit 左值
	recoverFormat := y.switchFormatBuffer()
	y.VisitExpressionList(i.ExpressionList())
	buf := recoverFormat()
	y.VisitLeftExpressionList(true, i.LeftExpressionList())

	y.writeString(" = ")
	y.writeString(buf)

	y.pushOperator(yakvm.OpAssign)
	return nil
}

func (y *YakCompiler) VisitDeclearVariableOnly(raw yak.IDeclearVariableOnlyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclearVariableOnlyContext)
	if i == nil {
		return nil
	}
	var count = len(i.AllIdentifier())
	for i := 0; i < count; i++ {
		y.pushUndefined()
	}
	y.pushListWithLen(count)

	// format
	y.writeString("var ")

	for index, idCtx := range i.AllIdentifier() {
		id, err := y.currentSymtbl.NewSymbolWithReturn(idCtx.GetText())
		if err != nil {
			y.panicCompilerError(CreateSymbolError, err.Error())
		}
		y.pushLeftRef(id)

		// format variables
		y.writeString(idCtx.GetText())
		if index != count-1 {
			y.writeString(", ")
		}
	}
	y.pushListWithLen(count)
	y.pushOperator(yakvm.OpAssign)
	return nil
}

func (y *YakCompiler) VisitDeclearVariableExpressionStmt(raw yak.IDeclearVariableExpressionStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclearVariableExpressionStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	y.VisitDeclearVariableExpression(i.DeclearVariableExpression())
	return nil
}
