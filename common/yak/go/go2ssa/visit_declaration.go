package go2ssa

import goparser "github.com/yaklang/yaklang/common/yak/go/parser"

func (y *builder) VisitDeclaration(raw goparser.IDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.DeclarationContext)
	if declar == nil {
		return nil
	}
	y.VisitConstDecl(declar.ConstDecl())
	y.VisitTypDecl(declar.TypeDecl())
	y.VisitVarDecl(declar.VarDecl())
	return nil
}
func (y *builder) VisitConstDecl(raw goparser.IConstDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.ConstDeclContext)
	if declar == nil {
		return nil
	}
	return nil
}

func (y *builder) VisitTypDecl(raw goparser.ITypeDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.TypeDeclContext)
	if declar == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitVarDecl(raw goparser.IVarDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.VarDeclContext)
	if declar == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitConstSpec(raw goparser.IConstSpecContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.ConstSpecContext)
	if declar == nil {
		return nil
	}
	return nil
}

func (y *builder) VisitIdentifierList(raw goparser.IIdentifierListContext) []string {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.IdentifierListContext)
	if declar == nil {
		return nil
	}
	var result []string
	for _, node := range declar.AllIDENTIFIER() {
		result = append(result, node.GetText())
	}
	return result
}
