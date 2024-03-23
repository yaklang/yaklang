package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitFunctionDeclaration(raw phpparser.IFunctionDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FunctionDeclarationContext)
	if i == nil {
		return nil
	}
	//var attr string
	if ret := i.Attributes(); ret != nil {
		y.VisitAttributes(ret)
		//_ = attr
	}
	//Ampersand 如果被设置了就是值引用
	isRef := i.Ampersand() != nil
	_ = isRef
	funcName := i.Identifier().GetText()
	y.ir.SetMarkedFunction(funcName)
	newFunction := y.ir.NewFunc(funcName)
	y.ir = y.ir.PushFunction(newFunction)
	{
		y.VisitFormalParameterList(i.FormalParameterList())
		y.VisitBlockStatement(i.BlockStatement())
		y.ir.SetType(y.VisitTypeHint(i.TypeHint()))
		y.ir.Finish()
	}
	y.ir = y.ir.PopFunction()
	variable := y.ir.CreateVariable(funcName)
	y.ir.AssignVariable(variable, newFunction)
	return nil
}

func (y *builder) VisitLambdaFunctionExpr(raw phpparser.ILambdaFunctionExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.LambdaFunctionExprContext)
	if i == nil {
		return nil
	}
	if i.Ampersand() != nil {
		//	doSomethings 在闭包中，不需要做其他特殊处理
	}
	funcName := ""
	newFunc := y.ir.NewFunc(funcName)
	y.ir = y.ir.PushFunction(newFunc)
	{
		y.VisitFormalParameterList(i.FormalParameterList())
		y.ir.SetType(y.VisitTypeHint(i.TypeHint()))
		y.VisitBlockStatement(i.BlockStatement())
		y.VisitExpression(i.Expression())
		y.ir.Finish()
	}
	y.ir = y.ir.PopFunction()
	return newFunc
}

//func (y *builder) VisitLambdaFunctionUseVars(raw phpparser.ILambdaFunctionUseVarsContext, sideEffect bool) interface{} {
//	if y == nil || raw == nil {
//		return nil
//	}
//	recoverRange := y.SetRange(raw)
//	defer recoverRange()
//
//	i, _ := raw.(*phpparser.LambdaFunctionUseVarsContext)
//	if i == nil {
//		return nil
//	}
//	for _, useVars := range i.AllLambdaFunctionUseVar() {
//		y.VisitLambdaFunctionUseVar(useVars)
//	}
//	return nil
//}

//func (y *builder) VisitLambdaFunctionUseVar(raw phpparser.ILambdaFunctionUseVarContext) interface{} {
//	if y == nil || raw == nil {
//		return nil
//	}
//	recoverRange := y.SetRange(raw)
//	defer recoverRange()
//
//	i, _ := raw.(*phpparser.LambdaFunctionUseVarContext)
//	if i == nil {
//		return nil
//	}
//	if i.Ampersand() != nil {
//		//doSomethings
//	}
//	return nil
//}
