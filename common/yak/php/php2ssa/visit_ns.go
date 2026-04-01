package php2ssa

import (
	"strings"

	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"golang.org/x/exp/maps"
)

func (y *builder) VisitQualifiedNamespaceNameList(raw phpparser.IQualifiedNamespaceNameListContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
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

func (y *builder) VisitQualifiedNamespaceName(raw phpparser.IQualifiedNamespaceNameContext) ([]string, string) {
	if y == nil || raw == nil || y.IsStop() {
		return []string{}, ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.QualifiedNamespaceNameContext)
	if i == nil {
		return []string{}, ""
	}
	path := y.VisitNamespacePath(i.NamespacePath())
	if len(path) == 0 {
		return []string{}, ""
	}
	class := path[len(path)-1]
	if len(path) == 1 {
		return path, class
	}
	return path[:len(path)-1], class
}

func (y *builder) VisitNamespaceUseDeclaration(raw phpparser.INamespaceUseDeclarationContext) ([]string, map[string]string) {
	if y == nil || raw == nil || y.IsStop() {
		return []string{}, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.NamespaceUseDeclarationContext)
	if i == nil {
		return nil, nil
	}

	path := y.VisitNamespacePath(i.NamespacePath())
	if tail := i.NamespaceUseTail(); tail != nil {
		return path, y.VisitNamespaceUseTail(tail)
	}
	if len(path) == 0 {
		return nil, nil
	}

	currentName := path[len(path)-1]
	splitPath := path
	if len(path) > 1 {
		splitPath = path[:len(path)-1]
	}
	aliasName := currentName
	if i.Identifier() != nil {
		aliasName = y.VisitIdentifier(i.Identifier())
	}
	return splitPath, map[string]string{currentName: aliasName}
}

func (y *builder) VisitNamespaceUseTail(raw phpparser.INamespaceUseTailContext) map[string]string {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.NamespaceUseTailContext)
	if i == nil {
		return nil
	}
	ret := make(map[string]string)
	for _, clause := range i.AllNamespaceUseClause() {
		m := y.VisitNamespaceUseClause(clause)
		maps.Copy(ret, m)
	}
	return ret
}

func (y *builder) VisitNamespaceUseClause(raw phpparser.INamespaceUseClauseContext) map[string]string {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.NamespaceUseClauseContext)
	if i == nil {
		return nil
	}

	pathList := y.VisitNamespacePath(i.NamespacePath())
	path := strings.Join(pathList, "\\")
	if tail := i.NamespaceUseTail(); tail != nil {
		child := y.VisitNamespaceUseTail(tail)
		ret := make(map[string]string, len(child))
		for key, value := range child {
			ret[path+"\\"+key] = value
		}
		return ret
	}
	if i.Identifier() != nil {
		alias := y.VisitIdentifier(i.Identifier())
		return map[string]string{path: alias}
	}
	return map[string]string{path: path}
}

func (y *builder) VisitNamespacePath(raw phpparser.INamespacePathContext) []string {
	if y == nil || raw == nil || y.IsStop() {
		return []string{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*phpparser.NamespacePathContext)
	var paths []string
	for _, identifierContext := range i.AllIdentifier() {
		paths = append(paths, y.VisitIdentifier(identifierContext))
	}
	return paths
}
