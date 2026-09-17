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
	expr := unwrapCallExpression(i.Expression())
	if expr == nil || (expr.FunctionCall() == nil && expr.InstanceCode() == nil) {
		err := y.newError("defer requires a function call")
		y.pushError(err)
		panic(err)
	}

	// functionCallExpr: expression
	// 表达式本身即为调用（如 f() / a.b() / func(){}() / fn{...}），
	// expression 的后缀调用逻辑会生成末尾的 OpCall。
	y.VisitExpression(i.Expression())

	return nil
}

// Parentheses do not change whether the source is a call or a function literal.
// Do not look through other expressions (including calls returning closures).
func unwrapCallExpression(raw yak.IExpressionContext) *yak.ExpressionContext {
	expr, _ := raw.(*yak.ExpressionContext)
	for expr != nil && expr.ParenExpression() != nil {
		paren := expr.ParenExpression().(*yak.ParenExpressionContext)
		expr, _ = paren.Expression().(*yak.ExpressionContext)
	}
	return expr
}
