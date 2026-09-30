package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitAssertStmt(raw yak.IAssertStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AssertStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	exps := i.AllExpression()
	for _, Iexp := range exps {
		exp, ok := Iexp.(*yak.ExpressionContext)
		if !ok {
			y.panicCompilerError(assertExpressionError)
		}
		y.VisitExpression(exp)
	}

	var desc = i.GetText()
	if len(exps) > 0 {
		desc = exps[0].GetText()
	}
	y.pushAssert(len(exps), desc)
	return nil
}
