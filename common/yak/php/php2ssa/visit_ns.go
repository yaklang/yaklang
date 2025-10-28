//go:build !no_language
// +build !no_language

package php2ssa

import (
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
	var class string
	list, m := y.VisitNamespaceNameList(i.NamespaceNameList())
	for key, _ := range m {
		class = key
		break
	}
	return list, class
}

func (y *builder) VisitNamespaceNameList(raw phpparser.INamespaceNameListContext) ([]string, map[string]string) {
	if y == nil || raw == nil || y.IsStop() {
		return []string{}, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch ret := raw.(type) {
	case *phpparser.NamespaceIdentifierContext:
		var (
			CurrentName string
			SplitPath   []string
			aliasName   string
		)

		path := y.VisitNamespacePath(ret.NamespacePath())
		if len(path) > 1 {
			CurrentName = path[len(path)-1]
			SplitPath = path[:len(path)-1]
		} else {
			CurrentName = path[0]
			SplitPath = path
		}
		aliasName = CurrentName
		if ret.Identifier() != nil {
			aliasName = y.VisitIdentifier(ret.Identifier())
		}
		return SplitPath, map[string]string{CurrentName: aliasName}
	case *phpparser.NamespaceListNameTailContext:
		path := y.VisitNamespacePath(ret.NamespacePath())
		tail := y.VisitNamespaceNameTail(ret.NamespaceNameTail())
		return path, tail
	}
	return nil, nil
}

func (y *builder) VisitNamespaceNameTail(raw phpparser.INamespaceNameTailContext) map[string]string {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.NamespaceNameTailContext)
	if i == nil {
		return nil
	}
	switch {
	case len(i.AllIdentifier()) != 0:
		paths := y.VisitIdentifier(i.Identifier(0))
		alias := y.VisitIdentifier(i.Identifier(1))
		if alias == "" {
			return map[string]string{paths: paths}
		} else {
			return map[string]string{paths: alias}
		}
	case len(i.AllNamespaceNameTail()) != 0:
		var (
			_map = make(map[string]string)
		)

		for _, tail := range i.AllNamespaceNameTail() {
			m := y.VisitNamespaceNameTail(tail)
			maps.Copy(m, _map)
		}
		return _map
	}
	return nil
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
