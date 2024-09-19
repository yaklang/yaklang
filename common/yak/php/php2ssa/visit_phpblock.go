package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitPhpBlock(raw phpparser.IPhpBlockContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.PhpBlockContext)
	if i == nil {
		return nil
	}

	//==================================================================== visitor
	if y.PreHandler && len(i.AllNamespaceDeclaration()) == 0 {
		return nil
	}
	for _, namespace := range i.AllNamespaceDeclaration() {
		y.VisitNamespaceDeclaration(namespace)
	}
	for _, usedecl := range i.AllUseDeclaration() {
		y.VisitUseDeclaration(usedecl)
	}
	for _, global := range i.AllGlobalConstantDeclaration() {
		y.VisitGlobalConstantDeclaration(global)
	}
	for _, functiondecl := range i.AllFunctionDeclaration() {
		y.VisitFunctionDeclaration(functiondecl)
	}
	for _, classdecl := range i.AllClassDeclaration() {
		y.VisitClassDeclaration(classdecl)
	}
	for _, stmt := range i.AllStatement() {
		y.VisitStatement(stmt)
	}
	for _, enum := range i.AllEnumDeclaration() {
		y.VisitEnumDeclaration(enum)
	}

	//=================================================================== builder
	for _, function := range y.GetProgram().Funcs {
		function.CheckAndFinishBuild()
		function.FixSpinUdChain()
	}
	for _, bluePrint := range y.GetProgram().ClassBluePrint {
		bluePrint.SyntaxMethods()
	}
	return nil
}
