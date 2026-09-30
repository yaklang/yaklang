package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitMakeExpression(raw yak.IMakeExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.MakeExpressionContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	y.VisitTypeLiteral(i.TypeLiteral())

	n := 0
	expressions := i.ExpressionListMultiline()
	if expressions != nil {
		if esi, _ := expressions.(*yak.ExpressionListMultilineContext); esi != nil {
			n = y.VisitExpressionListMultiline(esi)
		}
	}
	y.pushMake(n)

	return nil
}
