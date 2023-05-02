package yakast

import (
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitAssertStmt(raw yak.IAssertStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AssertStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("assert ")

	exps := i.AllExpression()
	lenOfExps := len(exps)
	for index, Iexp := range i.AllExpression() {
		exp, ok := Iexp.(*yak.ExpressionContext)
		if !ok {
			y.panicCompilerError(assertExpressionError)
		}
		y.VisitExpression(exp)
		if index < lenOfExps-1 {
			y.writeString(", ")
		}
	}

	var desc = i.GetText()
	if len(exps) > 0 {
		desc = exps[0].GetText()
	}
	y.pushAssert(len(exps), desc)
	return nil
}
