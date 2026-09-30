package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitFunctionCall(raw yak.IFunctionCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.FunctionCallContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	// 函数调用需要先把参数压栈
	// 调用的时候，call n 表示要取多少数出来
	argCount := 0
	if i.OrdinaryArguments() != nil {
		argCount, _ = y.VisitOrdinaryArguments(i.OrdinaryArguments())
	}

	if i.Wavy() != nil {
		y.pushCallWithWavy(argCount)
	} else {
		y.pushCall(argCount)
	}
	return nil
}

func (y *YakCompiler) VisitOrdinaryArguments(raw yak.IOrdinaryArgumentsContext) (int, bool) {
	if y == nil || raw == nil {
		return 0, false
	}

	i, _ := raw.(*yak.OrdinaryArgumentsContext)
	if i == nil {
		return 0, false
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	ellipsis := i.Ellipsis()
	allExpressions := i.AllExpression()
	lenOfAllExpressions := len(allExpressions)

	for _, expr := range allExpressions {
		y.VisitExpression(expr)
	}
	if ellipsis != nil && lenOfAllExpressions > 0 {
		y.pushEllipsis(lenOfAllExpressions)
	}
	return lenOfAllExpressions, ellipsis != nil
}
