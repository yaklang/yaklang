package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
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
		y.ir.Finish()
	}
	y.ir = y.ir.PopFunction()
	variable := y.ir.CreateVariable(funcName)
	y.ir.AssignVariable(variable, newFunction)
	return nil
}
