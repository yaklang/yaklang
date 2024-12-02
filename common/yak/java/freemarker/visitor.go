package freemarker

import (
	"github.com/yaklang/yaklang/common/log"
	freemarkerparser "github.com/yaklang/yaklang/common/yak/java/freemarker/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"strings"
)

type FreeMarkerVisitor struct {
	*tl.Visitor
}

func NewFreeMarkerVisitor() *FreeMarkerVisitor {
	return &FreeMarkerVisitor{
		tl.NewVisitor(),
	}
}

func (y *FreeMarkerVisitor) VisitTemplate(raw freemarkerparser.ITemplateContext) {
	if y == nil || raw == nil {
		return
	}

	i, ok := raw.(*freemarkerparser.TemplateContext)
	if !ok {
		return
	}
	if i.Elements() != nil {
		y.VisitElements(i.Elements())
	}
}

func (y *FreeMarkerVisitor) VisitElements(raw freemarkerparser.IElementsContext) {
	if y == nil || raw == nil {
		return
	}

	i, ok := raw.(*freemarkerparser.ElementsContext)
	if !ok {
		return
	}

	for _, element := range i.AllElement() {
		y.VisitElement(element)
	}
}

func (y *FreeMarkerVisitor) VisitElement(raw freemarkerparser.IElementContext) {
	if y == nil || raw == nil {
		return
	}
	switch i := raw.(type) {
	case *freemarkerparser.RawTextElementContext:
		y.VisitRawTextElement(i.RawText())
	case *freemarkerparser.DirectiveElementContext:
		y.VisitDirectiveElement(i.Directive())
	case *freemarkerparser.InlineExprElementContext:
		texts := i.GetText()
		expr := "elExpr.parse(\"" + texts + "\")"
		if strings.Contains(texts, "?html") {
			y.EmitEscapeOutput(expr)
		} else {
			y.EmitOutput(expr)
		}
	default:
		log.Errorf("Unknown element type: %v", i)
	}
}

func (y *FreeMarkerVisitor) VisitRawTextElement(raw freemarkerparser.IRawTextContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.RawTextContext)
	if !ok {
		return
	}
	texts := i.GetText()
	for _, text := range strings.Split(texts, "\n") {
		text = strings.TrimSpace(text)
		y.EmitPureText(text)
	}
}

func (y *FreeMarkerVisitor) VisitDirectiveElement(raw freemarkerparser.IDirectiveContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveContext)
	if !ok {
		return
	}
	// TODO :弘标签
	if i.DirectiveIf() != nil {
		y.VisitDirectiveIf(i.DirectiveIf())
	} else if i.DirectiveList() != nil {
		y.VisitDirectiveList(i.DirectiveList())
	} else if i.DirectiveAssign() != nil {
		y.VisitDirectiveAssign(i.DirectiveAssign())
	} else if i.DirectiveImport() != nil {

	} else if i.DirectiveInclude() != nil {

	} else if i.DirectiveMacro() != nil {

	} else if i.DirectiveInclude() != nil {

	} else if i.DirectiveReturn() != nil {

	} else if i.DirectiveNested() != nil {

	}
}

func (y *FreeMarkerVisitor) VisitDirectiveIf(raw freemarkerparser.IDirectiveIfContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveIfContext)
	if !ok {
		return
	}

	if i.TagExpr() == nil {
		return
	}

	cond := i.TagExpr().GetText()
	y.EmitPureCode("if (" + cond + ") {")

	if i.DirectiveIfTrueElements() != nil {
		y.VisitDirectiveIfTrueElements(i.DirectiveIfTrueElements())
	}

	for _, elseIfCond := range i.AllTagExprElseIfs() {
		y.EmitPureCode("} else if (" + elseIfCond.GetText() + ") {")
		if i.DirectiveIfTrueElements() != nil {
			y.VisitDirectiveIfTrueElements(i.DirectiveIfTrueElements())
		}
	}

	if i.GetElse_() != nil {
		y.EmitPureCode("} else {")
		if i.DirectiveIfTrueElements() != nil {
			y.VisitDirectiveIfTrueElements(i.DirectiveIfTrueElements())
		}
	}
	y.EmitPureCode("}")
}

func (y *FreeMarkerVisitor) VisitDirectiveIfTrueElements(raw freemarkerparser.IDirectiveIfTrueElementsContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveIfTrueElementsContext)
	if !ok {
		return
	}
	if i.Elements() != nil {
		y.VisitElements(i.Elements())
	}
}

func (y *FreeMarkerVisitor) VisitDirectiveList(raw freemarkerparser.IDirectiveListContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveListContext)
	if !ok {
		return
	}
	if i.TagExpr() == nil {
		return
	}
	if i.GetValue() == nil {
		return
	}

	var cond1, cond2 string

	cond2 = i.TagExpr().GetText()
	cond1 = i.GetValue().GetText()

	y.EmitPureCode("for ( Object " + cond1 + " : " + "elExpr.parse(\"" + cond2 + "\")) {")
	if i.DirectiveListBodyElements() != nil {
		y.VisitDirectiveListBodyElements(i.DirectiveListBodyElements())
	}
	y.EmitPureCode("}")
}

func (y *FreeMarkerVisitor) VisitDirectiveListBodyElements(raw freemarkerparser.IDirectiveListBodyElementsContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveListBodyElementsContext)
	if !ok {
		return
	}
	if i.Elements() != nil {
		y.VisitElements(i.Elements())
	}
}

func (y *FreeMarkerVisitor) VisitDirectiveAssign(raw freemarkerparser.IDirectiveAssignContext) {
	if y == nil || raw == nil {
		return
	}
	i, ok := raw.(*freemarkerparser.DirectiveAssignContext)
	if !ok {
		return
	}
	if i.Elements() != nil {
		y.VisitElements(i.Elements())
	}
}
