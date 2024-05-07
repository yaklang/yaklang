package go2ssa

import (
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
)

func (y *builder) VisitSourceFile(raw goparser.ISourceFileContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	//recoverRange := y.SetRange(raw)
	//defer recoverRange()

	i, _ := raw.(*goparser.SourceFileContext)
	if i == nil {
		return nil
	}
	if i.PackageClause() != nil {
		//do package handle
	}
	for _, importDecl := range i.AllImportDecl() {
		y.VisitImportDecl(importDecl)
	}
	for _, functionDecl := range i.AllFunctionDecl() {
		//
		_ = functionDecl
	}
	for _, declaration := range i.AllDeclaration() {
		y.VisitDeclaration(declaration)
	}
	for _, methodDecl := range i.AllMethodDecl() {
		_ = methodDecl
	}
	return nil
}

func (y *builder) VisitPackageClause(raw goparser.IPackageClauseContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	//recoverRange := y.SetRange(raw)
	//defer recoverRange()

	i, _ := raw.(*goparser.PackageClauseContext)
	if i == nil {
		return nil
	}

	return nil
}
func (y *builder) VisitImportDecl(raw goparser.IImportDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	//recoverRange := y.SetRange(raw)
	//defer recoverRange()

	i, _ := raw.(*goparser.ImportDeclContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitImportSpec(raw goparser.IImportSpecContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	//recoverRange := y.SetRange(raw)
	//defer recoverRange()
	i, _ := raw.(*goparser.ImportSpecContext)
	if i == nil {
		return nil
	}
	imports := i.ImportPath().(*goparser.ImportPathContext)
	importPath := y.VisitString_(imports.String_())
	_ = importPath
	return nil
}
func (y *builder) VisitFunctionDecl(raw goparser.IFunctionDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.FunctionDeclContext)
	funcName := i.IDENTIFIER().GetText()
	y.ir.SetMarkedFunction(funcName)
	function := y.ir.NewFunc(funcName)
	y.ir.PushFunction(function)
	{
		//visit signature
		//visit block
		y.VisitSignature(i.Signature())
		y.VisitBlock(i.Block())
		y.ir.Finish()
	}
	y.ir.PopFunction()
	variable := y.ir.CreateVariable(funcName)
	y.ir.AssignVariable(variable, function)
	return nil
}
func (y *builder) VisitSignature(raw goparser.ISignatureContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.SignatureContext)
	_ = i
	//params, types := y.VisitParameters(i.Parameters())
	return nil
}
func (y *builder) VisitBlock(raw goparser.IBlockContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.BlockContext)
	if list := i.StatementList(); list != nil {
		statList := list.(*goparser.StatementListContext)
		for _, stat := range statList.AllStatement() {
			y.VisitStatement(stat)
		}
	}
	return nil
}
func (y *builder) VisitString_(raw goparser.IString_Context) string {
	if y == nil || raw == nil {
		return ""
	}
	//recoverRange := y.SetRange(raw)
	//defer recoverRange()

	i, _ := raw.(*goparser.String_Context)
	if i == nil {
		return ""
	}
	if i.RAW_STRING_LIT() != nil {
		return i.RAW_STRING_LIT().GetText()
	}
	return i.INTERPRETED_STRING_LIT().GetText()
}
