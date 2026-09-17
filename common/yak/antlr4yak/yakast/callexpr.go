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
	expr, _ := i.Expression().(*yak.ExpressionContext)
	for expr != nil && expr.LParen() != nil && expr.RParen() != nil && len(expr.AllExpression()) == 1 && expr.FunctionCall() == nil && expr.MemberCall() == nil && expr.SliceCall() == nil {
		expr, _ = expr.Expression(0).(*yak.ExpressionContext)
	}
	if expr == nil || (expr.FunctionCall() == nil && expr.InstanceCode() == nil) {
		err := y.newError("go/defer requires a function call")
		y.pushError(err)
		panic(err)
	}

	// functionCallExpr: expression
	// 表达式本身即为调用（如 f() / a.b() / func(){}() / fn{...}），
	// expression 的后缀调用逻辑会生成末尾的 OpCall，交由 go/defer 等语句按需转换。
	y.VisitExpression(i.Expression())

	return nil
}
