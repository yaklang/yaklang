//go:build !no_language
// +build !no_language

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
	if y.GetProgram().CurrentIncludingStack.Len() <= 0 {
		if !y.PreHandler() {
			for _, context := range i.AllNamespaceDeclaration() {
				y.VisitNamespaceOnlyUse(context)
			}
		}
		for _, namespace := range i.AllNamespaceDeclaration() {
			y.VisitNamespaceDeclaration(namespace)
		}
	}
	if y.PreHandler() {
		for _, functiondecl := range i.AllFunctionDeclaration() {
			y.VisitFunctionDeclaration(functiondecl)
		}
		for _, classdecl := range i.AllClassDeclaration() {
			y.VisitClassDeclaration(classdecl)
		}
	} else {
		for _, usedecl := range i.AllUseDeclaration() {
			y.VisitUseDeclaration(usedecl)
		}
		for _, global := range i.AllGlobalConstantDeclaration() {
			y.VisitGlobalConstantDeclaration(global)
		}
		for _, stmt := range i.AllStatement() {
			y.VisitStatement(stmt)
		}
		for _, enum := range i.AllEnumDeclaration() {
			y.VisitEnumDeclaration(enum)
		}
	}
	if len(i.AllNamespaceDeclaration()) <= 0 {
		y.GetProgram().VisitAst(raw)
	}
	return nil
}
