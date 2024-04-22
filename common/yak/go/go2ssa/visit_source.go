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

	}
	for _, importDecl := range i.AllImportDecl() {
		y.VisitImportDecl(importDecl)
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
