package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitQualifiedNamespaceNameList(raw phpparser.IQualifiedNamespaceNameListContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.QualifiedNamespaceNameListContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitQualifiedNamespaceName(raw phpparser.IQualifiedNamespaceNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.QualifiedNamespaceNameContext)
	if i == nil {
		return nil
	}

	if i.Namespace() != nil {
		// declare namespace mode
		list := i.NamespaceNameList().(*phpparser.NamespaceNameListContext)
		if ret := list.NamespaceNameTail(); ret != nil {

		}
		return y.ir.EmitConstInst(nil)
	}

	if nameList := i.NamespaceNameList(); nameList != nil {
		// use namespace mode
		return y.VisitNamespaceNameList(nameList.(*phpparser.NamespaceNameListContext))
	}

	return nil
}

func (y *builder) VisitNamespaceNameList(raw phpparser.INamespaceNameListContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.NamespaceNameListContext)
	if i == nil {
		return nil
	}

	ir := y.ir
	var lastValue ssa.Value
	for _, id := range i.AllIdentifier() {
		val := ir.ReadValue(id.GetText())
		if lastValue != nil {
			lastValue = ir.CreateMemberCallVariable(lastValue, val).GetValue()
		} else {
			lastValue = val
		}
	}
	if i.NamespaceNameTail() != nil {
		log.Warn("namespace tail build unfinished")
	}
	return lastValue
}

func (y *builder) VisitNamespaceNameTail(raw phpparser.INamespaceNameTailContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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
