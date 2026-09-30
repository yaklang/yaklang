package yakast

import (
	"strings"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitAssignExpressionStmt(raw yak.IAssignExpressionStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AssignExpressionStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
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
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	// assign eq  /  colon assign eq
	if op, op2 := i.AssignEq(), i.ColonAssignEq(); op != nil || op2 != nil {

		// 赋值语句，应该先 visit 右值，再 visit 左值
		y.VisitExpressionList(i.ExpressionList())

		forceAssign := op2 != nil
		y.VisitLeftExpressionList(forceAssign, i.LeftExpressionList())

		y.pushOperator(yakvm.OpAssign)
		return nil
	}

	if i.PlusPlus() != nil { // ++
		y.VisitLeftExpression(false, i.LeftExpression())
		y.pushOperator(yakvm.OpPlusPlus)
		return nil
	} else if i.SubSub() != nil { // --
		y.VisitLeftExpression(false, i.LeftExpression())
		y.pushOperator(yakvm.OpMinusMinus)
		return nil
	}

	if op := i.InplaceAssignOperator(); op != nil {
		y.VisitExpression(i.Expression())
		y.VisitLeftExpression(false, i.LeftExpression())
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
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	allExpr := i.AllLeftExpression()
	for _, le := range allExpr {
		y.VisitLeftExpression(forceNewSymbol, le)
	}
	if allExpr != nil {
		y.pushListWithLen(len(allExpr))
	}

	return len(allExpr)
}

// VisitLeftExpression 处理赋值左值。
//
// 语法已消歧为 `leftExpression: Identifier | expression`（见 YaklangParser.g4），
// 因此左值可能是：
//   - 裸标识符（Identifier 备选）：a = 1
//   - 成员访问表达式（expression 备选，顶层为 memberCall）：a.b = 1 / a.$x = 1
//   - 下标访问表达式（expression 备选，顶层为单下标 sliceCall）：a[0] = 1
//
// 其余表达式（如字面量、函数调用、切片区间 a[1:2]）不是合法左值，在此报编译错误。
func (y *YakCompiler) VisitLeftExpression(forceNewSymbol bool, raw yak.ILeftExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.LeftExpressionContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	// Identifier 备选：裸标识符左值
	if id := i.Identifier(); id != nil {
		y.visitLeftIdentifier(forceNewSymbol, id.GetText())
		return nil
	}

	expr, ok := i.Expression().(*yak.ExpressionContext)
	if !ok || expr == nil {
		y.panicCompilerError(compileError, "invalid left expression")
		return nil
	}

	// 兜底：expression 顶层就是裸标识符（正常会被 Identifier 备选优先命中）
	if id := expr.Identifier(); id != nil && len(expr.AllExpression()) == 0 &&
		expr.MemberCall() == nil && expr.SliceCall() == nil && expr.FunctionCall() == nil {
		y.visitLeftIdentifier(forceNewSymbol, id.GetText())
		return nil
	}

	// 成员赋值 a.b = ... / a.$x = ...
	if m, ok := expr.MemberCall().(*yak.MemberCallContext); ok && m != nil {
		base, ok := expr.Expression(0).(*yak.ExpressionContext)
		if !ok || base == nil {
			y.panicCompilerError(compileError, "invalid left member expression")
			return nil
		}
		y.VisitExpression(base)
		y.visitLeftMemberCallKey(m)
		y.pushListWithLen(2)
		return nil
	}

	// 下标赋值 a[0] = ...（仅单下标 [expr] 形式可作为左值）
	/*
		a[1] = 1

		push slice
		push index
		list 2  // 组合 slice[index] 放入栈，由 assign 判断后续的值
		一定小心，不要乱改
	*/
	if s, ok := expr.SliceCall().(*yak.SliceCallContext); ok && s != nil {
		base, ok := expr.Expression(0).(*yak.ExpressionContext)
		if !ok || base == nil {
			y.panicCompilerError(compileError, "invalid left slice expression")
			return nil
		}
		// 只有单下标 [expr]（无冒号且恰好一个下标表达式）可作为左值
		if len(s.AllColon()) != 0 || len(s.AllExpression()) != 1 {
			y.panicCompilerError(compileError, "cannot assign to slice range expression")
			return nil
		}
		y.VisitExpression(base)
		y.VisitExpression(s.Expression(0))
		y.pushListWithLen(2)
		return nil
	}

	y.panicCompilerError(compileError, "invalid left expression")
	return nil
}

// visitLeftIdentifier 处理裸标识符左值的符号创建
func (y *YakCompiler) visitLeftIdentifier(forceNewSymbol bool, idName string) {
	if forceNewSymbol {
		id, err := y.currentSymtbl.NewSymbolWithReturn(idName)
		if err != nil {
			y.panicCompilerError(forceCreateSymbolFailed, idName)
		}
		y.pushLeftRef(id)
		return
	}
	sym, ok := y.currentSymtbl.GetSymbolByVariableName(idName)
	if !ok {
		var err error
		sym, err = y.currentSymtbl.NewSymbolWithReturn(idName)
		if err != nil {
			y.panicCompilerError(autoCreateSymbolFailed, idName)
		}
	}
	y.pushLeftRef(sym)
}

// visitLeftMemberCallKey 将成员访问的 key 压栈（.id 压字符串常量，.$var 压变量引用）
func (y *YakCompiler) visitLeftMemberCallKey(i *yak.MemberCallContext) {
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	if id := i.Identifier(); id != nil {
		idName := id.GetText()
		y.pushString(idName, idName)
		return
	}

	if id := i.IdentifierWithDollar(); id != nil {
		varName := id.GetText()
		for strings.HasPrefix(varName, "$") {
			varName = varName[1:]
		}
		symbolId, ok := y.currentSymtbl.GetSymbolByVariableName(varName)
		if !ok {
			y.panicCompilerError(notFoundDollarVariable, varName)
		}
		y.pushRef(symbolId)
		return
	}

	y.panicCompilerError(bugMembercall)
}

func (y *YakCompiler) VisitDeclareVariableExpression(raw yak.IDeclareVariableExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclareVariableExpressionContext)
	if i == nil {
		return nil
	}

	if s := i.DeclareVariableOnly(); s != nil {
		y.VisitDeclareVariableOnly(s)
		return nil
	}

	if s := i.DeclareAndAssignExpression(); s != nil {
		y.VisitDeclareAndAssignExpression(s)
		return nil
	}

	return nil
}

func (y *YakCompiler) VisitDeclareAndAssignExpression(raw yak.IDeclareAndAssignExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclareAndAssignExpressionContext)
	if i == nil {
		return nil
	}

	// 赋值语句，应该先 visit 右值，再 visit 左值
	y.VisitExpressionList(i.ExpressionList())

	y.VisitLeftExpressionList(true, i.LeftExpressionList())

	y.pushOperator(yakvm.OpAssign)
	return nil
}

func (y *YakCompiler) VisitDeclareVariableOnly(raw yak.IDeclareVariableOnlyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclareVariableOnlyContext)
	if i == nil {
		return nil
	}
	var count = len(i.AllIdentifier())
	for i := 0; i < count; i++ {
		y.pushUndefined()
	}
	y.pushListWithLen(count)

	for _, idCtx := range i.AllIdentifier() {
		id, err := y.currentSymtbl.NewSymbolWithReturn(idCtx.GetText())
		if err != nil {
			y.panicCompilerError(CreateSymbolError, err.Error())
		}
		y.pushLeftRef(id)
	}
	y.pushListWithLen(count)
	y.pushOperator(yakvm.OpAssign)
	return nil
}

func (y *YakCompiler) VisitDeclareVariableExpressionStmt(raw yak.IDeclareVariableExpressionStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.DeclareVariableExpressionStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	y.VisitDeclareVariableExpression(i.DeclareVariableExpression())
	return nil
}
