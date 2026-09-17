package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/google/uuid"
)

func (y *YakCompiler) VisitGoStmt(raw yak.IGoStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.GoStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("go ")

	call := i.CallExpr().(*yak.CallExprContext)
	if instance := call.InstanceCode(); instance != nil {
		y.VisitInstanceCode(instance)
	} else {
		rawExpr := call.FunctionCallExpr().(*yak.FunctionCallExprContext).Expression()
		expr := unwrapCallExpression(rawExpr)
		switch {
		case expr != nil && (expr.FunctionCall() != nil || expr.InstanceCode() != nil):
			// Preserve call-site argument evaluation and invoke only this call;
			// its result is never automatically invoked, even if it is a closure.
			y.VisitExpression(rawExpr)
		case expr != nil && expr.AnonymousFunctionDecl() != nil:
			// Only a syntactic function definition gets an implicit () call.
			y.VisitExpression(rawExpr)
			decl := expr.AnonymousFunctionDecl().(*yak.AnonymousFunctionDeclContext)
			if name := decl.FunctionNameDecl(); name != nil {
				// Named declarations assign their function instead of leaving it
				// on the operand stack, so load the newly declared binding.
				id, _ := y.currentSymtbl.GetSymbolByVariableName(name.GetText())
				y.pushRef(id)
			}
			y.pushCall(0)
		default:
			// Evaluate the entire expression in the worker, including nested
			// calls, short-circuit branches and channel operations.
			y.pushGoExpression(rawExpr)
		}
	}
	lastCode := y.codes[y.GetCodeIndex()]
	if lastCode.Opcode != yakvm.OpCall {
		y.panicCompilerError(compileError, "go statement did not produce a call")
	}
	lastCode.Opcode = yakvm.OpAsyncCall
	return nil
}

func (y *YakCompiler) pushGoExpression(expr yak.IExpressionContext) {
	fn := func() *yakvm.Function {
		restoreCodes := y.SwitchCodes()
		defer restoreCodes()
		restoreSymbols := y.SwitchSymbolTable("go-expression", uuid.New().String())
		defer restoreSymbols()
		y.VisitExpression(expr)
		y.pushOpPop()
		y.pushOperator(yakvm.OpReturn)
		fn := yakvm.NewFunction(y.codes, y.currentSymtbl)
		fn.FreeValue = y.FreeValues
		if y.sourceCodePointer != nil {
			fn.SetSourceCode(*y.sourceCodePointer)
		}
		return fn
	}()
	value := &yakvm.Value{TypeVerbose: "anonymous-function", Value: fn}
	if len(fn.FreeValue) == 0 {
		y.pushValue(value)
	} else {
		y.pushValueWithCopy(value)
	}
	y.pushCall(0)
}
