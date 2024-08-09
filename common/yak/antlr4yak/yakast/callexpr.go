package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitCallExpr(raw yak.ICallExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.CallExprContext)
	if i == nil {
		return nil
	}

	if s := i.InstanceCode(); s != nil {
		y.VisitInstanceCode(s)
	} else if s := i.FunctionCallExpr(); s != nil {
		y.VisitFunctionCallExpr(s)
	}

	return nil
}

func (y *YakCompiler) VisitFunctionCallExpr(raw yak.IFunctionCallExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.FunctionCallExprContext)
	if i == nil {
		return nil
	}

	y.VisitExpression(i.Expression())
	y.VisitFunctionCall(i.FunctionCall())

	return nil
}
