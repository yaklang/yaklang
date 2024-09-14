package java2ssa

import javaparser "github.com/yaklang/yaklang/common/yak/java/parser"

func (y *builder) VisitModuleDeclaration(raw javaparser.IModuleDeclarationContext) interface{} {
	if y == nil || raw == nil || y.isStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ModuleDeclarationContext)
	if i == nil {
		return nil
	}

	// Java9 MODEULS

	return nil
}
