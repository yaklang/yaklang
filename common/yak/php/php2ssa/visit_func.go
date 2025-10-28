//go:build !no_language
// +build !no_language

package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitFunctionDeclaration(raw phpparser.IFunctionDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
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
	newFunction := y.NewFunc(funcName)
	variable := y.CreateVariable(funcName)
	y.AssignVariable(variable, newFunction)
	y.GetProgram().SetExportValue(funcName, newFunction)
	store := y.StoreFunctionBuilder()
	newFunction.AddLazyBuilder(func() {
		switchHandler := y.SwitchFunctionBuilder(store)
		defer switchHandler()
		y.SetMarkedFunction(funcName)
		y.FunctionBuilder = y.FunctionBuilder.PushFunction(newFunction)
		{
			y.VisitFormalParameterList(i.FormalParameterList())
			y.VisitBlockStatement(i.BlockStatement())
			y.SetType(y.VisitTypeHint(i.TypeHint()))
			y.Finish()
		}
		y.FunctionBuilder = y.PopFunction()
		variable := y.CreateVariable(funcName)
		y.AssignVariable(variable, newFunction)
	})
	return nil
}

func (y *builder) VisitReturnTypeDecl(raw phpparser.IReturnTypeDeclContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ReturnTypeDeclContext)
	if i == nil {
		return nil
	}

	allowNull := i.QuestionMark() != nil
	t := y.VisitTypeHint(i.TypeHint())
	_ = allowNull
	// t.Union(Null)

	return t
}

func (y *builder) VisitBaseCtorCall(raw phpparser.IBaseCtorCallContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.BaseCtorCallContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitFormalParameterList(raw phpparser.IFormalParameterListContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FormalParameterListContext)
	if i == nil {
		return nil
	}

	for index, param := range i.AllFormalParameter() {
		y.VisitFormalParameter(param, index)
	}

	return nil
}

func (y *builder) VisitFormalParameter(raw phpparser.IFormalParameterContext, index int) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FormalParameterContext)
	if i == nil {
		return
	}

	// PHP8 annotation
	if i.Attributes() != nil {
		_ = i.Attributes().GetText()
	}
	// member modifier cannot be used in function formal params
	allowNull := i.QuestionMark() != nil
	_ = allowNull

	typeHint := y.VisitTypeHint(i.TypeHint())
	if typeHint.RawString() == "" {
		typeHint = ssa.CreateAnyType()
	}
	isRef := i.Ampersand() != nil
	isVariadic := i.Ellipsis()
	_, _, _ = typeHint, isRef, isVariadic
	formalParams, defaultValue := y.VisitVariableInitializer(i.VariableInitializer())
	param := y.NewParam(formalParams)
	if defaultValue != nil {
		param.SetDefault(defaultValue)
		if t := defaultValue.GetType(); t != nil {
			param.SetType(t)
		}
	}
	if typeHint != nil {
		param.SetType(typeHint)
	}
	if isRef {
		y.ReferenceParameter(formalParams, index, ssa.PointerSideEffect)
	}
	return
}

func (y *builder) VisitLambdaFunctionExpr(raw phpparser.ILambdaFunctionExprContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
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
	newFunc := y.NewFunc(funcName)

	//todo: 还有问题，"类"闭包在类中还有问题
	y.FunctionBuilder = y.PushFunction(newFunc)
	{
		y.VisitLambdaFunctionUseVars(i.LambdaFunctionUseVars())
		y.VisitFormalParameterList(i.FormalParameterList())
		y.SetType(y.VisitTypeHint(i.TypeHint()))
		y.VisitBlockStatement(i.BlockStatement())
		y.VisitExpression(i.Expression())
		y.Finish()
	}
	y.FunctionBuilder = y.PopFunction()
	return newFunc
}
func (y *builder) VisitLambdaFunctionUseVars(raw phpparser.ILambdaFunctionUseVarsContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.LambdaFunctionUseVarsContext)
	if i == nil {
		return nil
	}
	for _, lambda := range i.AllLambdaFunctionUseVar() {
		y.VisitLambdaFunctionUseVar(lambda)
	}
	return nil
}
func (y *builder) VisitLambdaFunctionUseVar(raw phpparser.ILambdaFunctionUseVarContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.LambdaFunctionUseVarContext)
	if i == nil {
		return nil
	}
	current := y.SupportClosure
	y.SupportClosure = true
	defer func() {
		y.SupportClosure = current
	}()
	if i.Ampersand() != nil {
		y.FunctionBuilder.AddCaptureFreevalue(i.VarName().GetText())
		//if !utils.IsNil(value) {
		//	freeValue := y.BuildFreeValue(i.VarName().GetText())
		//	p, ok := ssa.ToParameter(value)
		//	if ok && p.GetDefault() != nil {
		//		freeValue.SetDefault(p.GetDefault())
		//	}
		//	freeValue.SetType(value.GetType())
		//}
	} else {
		value := y.PeekValue(i.VarName().GetText())
		variable := y.CreateLocalVariable(i.VarName().GetText())
		variable.Assign(value)
	}
	return nil
}
