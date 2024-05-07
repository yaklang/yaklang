package go2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
)

func (y *builder) VisitDeclaration(raw goparser.IDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	declar, _ := raw.(*goparser.DeclarationContext)
	if declar == nil {
		return nil
	}
	switch {
	case declar.VarDecl() != nil:
		y.VisitVarDecl(declar.VarDecl())
	case declar.ConstDecl() != nil:
		y.VisitConstDecl(declar.ConstDecl())
	case declar.TypeDecl() != nil:
		y.VisitTypDecl(declar.TypeDecl())
	}
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
	i, _ := raw.(*goparser.VarDeclContext)
	if i == nil {
		return nil
	}
	for _, spec := range i.AllVarSpec() {
		y.VisitVarSpec(spec)
	}
	return nil
}
func (y *builder) VisitVarSpec(raw goparser.IVarSpecContext) interface{} {
	if y == nil && raw == nil {
		return nil
	}
	i, _ := raw.(*goparser.VarSpecContext)
	if i == nil {
		return nil
	}
	list := y.VisitIdentifierList(i.IdentifierList())
	explist := i.ExpressionList().(*goparser.ExpressionListContext)
	if len(list) != len(explist.AllExpression()) {
		log.Warn("var declare fail: variable number and expression number not match")
		return nil
	}
	for j, expr := range explist.AllExpression() {
		variable := y.ir.CreateVariable(list[j])
		value := y.VisitExpression(expr)
		y.ir.AssignVariable(variable, value)
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
