package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
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
	i, _ := raw.(*phpparser.QualifiedNamespaceNameContext)
	if i == nil {
		return ""
	}

	if i.Namespace() != nil {
		// declare namespace mode
		list := i.NamespaceNameList().(*phpparser.NamespaceNameListContext)
		if ret := list.NamespaceNameTail(); ret != nil {

		}
		// return y.EmitConstInst(nil)
		return ""
	}

	if nameList := i.NamespaceNameList(); nameList != nil {
		// use namespace mode
		return y.VisitNamespaceNameList(nameList.(*phpparser.NamespaceNameListContext))
	}

	return ""
}

func (y *builder) VisitNamespaceNameList(raw phpparser.INamespaceNameListContext) string {
	if y == nil || raw == nil {
		return ""
	}

	i, _ := raw.(*phpparser.NamespaceNameListContext)
	if i == nil {
		return ""
	}

	// ir := y
	var lastValue string
	lastValue = i.GetText()
	// for _, id := range i.AllIdentifier() {
	// name := id.GetText()
	// val := ir.ReadOrCreateVariable(name)
	// if lastValue != nil {
	// lastValue = ir.CreateMemberCallVariable(lastValue, val).GetValue()
	// lastValue =
	// } else {
	// 	lastValue = val
	// }
	// }
	// if i.NamespaceNameTail() != nil {
	// 	log.Warn("namespace tail build unfinished")
	// }
	return lastValue
}

func (y *builder) VisitNamespaceNameTail(raw phpparser.INamespaceNameTailContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceNameTailContext)
	if i == nil {
		return nil
	}

	if i.OpenCurlyBracket() != nil {
		// check {...}
		for _, tail := range i.AllNamespaceNameTail() {
			y.VisitNamespaceNameTail(tail)
		}
	} else {
		// check ... as?
		for _, id := range i.AllIdentifier() {
			if ret := y.VisitIdentifier(id); ret != "" {
				log.Warnf("fetch identifier: %v", ret)
			}
		}
	}

	return nil
}
