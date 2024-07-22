package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitQualifiedNamespaceNameList(raw phpparser.IQualifiedNamespaceNameListContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.QualifiedNamespaceNameListContext)
	if i == nil {
		return nil
	}
	for _, namespaceName := range i.AllQualifiedNamespaceName() {
		y.VisitQualifiedNamespaceName(namespaceName)
	}
	return nil
}

func (y *builder) VisitQualifiedNamespaceName(raw phpparser.IQualifiedNamespaceNameContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.QualifiedNamespaceNameContext)
	if i == nil {
		return ""
	}
	return raw.GetText()
}

func (y *builder) VisitNamespaceNameList(raw phpparser.INamespaceNameListContext) []string {
	if y == nil || raw == nil {
		return []string{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.NamespaceNameListContext)
	if i == nil {
		return []string{}
	}
	var pkg []string
	for _, identifierContext := range i.AllIdentifier() {
		pkg = append(pkg, y.VisitIdentifier(identifierContext))
	}
	if i.NamespaceNameTail() != nil {
		pkg = append(pkg, y.VisitNamespaceNameTail(i.NamespaceNameTail()))
	}
	return pkg
}

func (y *builder) VisitNamespaceNameTail(raw phpparser.INamespaceNameTailContext) (c string) {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceNameTailContext)
	if i == nil {
		return ""
	}
	if i.OpenCurlyBracket() != nil {
		// check {...}
		for _, tail := range i.AllNamespaceNameTail() {
			return y.VisitNamespaceNameTail(tail)
		}

		//todo as 操作
	} else {
		return y.VisitIdentifier(i.Identifier(0))
	}
	return ""
}
