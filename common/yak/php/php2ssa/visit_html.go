package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitHtmlDocument(raw phpparser.IHtmlDocumentContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.HtmlDocumentContext)
	if i == nil {
		return nil
	}
	if i.Shebang() != nil {
		// handle shebang
	}

	for _, child := range i.GetChildren() {
		switch current := child.(type) {
		case phpparser.IHtmlDocumentElementContext:
			y.VisitHtmlDocumentElement(current)
		case phpparser.IPhpBlockContext:
			y.VisitPhpBlock(current)
		}
	}
	return nil
}

func (y *builder) VisitHtmlDocumentElement(raw phpparser.IHtmlDocumentElementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.HtmlDocumentElementContext)
	if i == nil {
		return nil
	}

	for _, child := range i.GetChildren() {
		if current, ok := child.(phpparser.IHtmlElementContext); ok {
			y.VisitHtmlElement(current)
		}
	}
	return nil
}

func (y *builder) VisitInlineHtml(raw phpparser.IInlineHtmlContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InlineHtmlContext)
	if i == nil {
		return nil
	}
	for _, elementContext := range i.AllHtmlElement() {
		y.VisitHtmlElement(elementContext)
	}
	y.VisitScriptText(i.ScriptText())
	return nil
}

func (y *builder) VisitInlineHtmlStatement(raw phpparser.IInlineHtmlStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InlineHtmlStatementContext)
	if i == nil {
		return nil
	}
	echoFunc := y.ReadOrCreateVariable("echo")
	call := y.NewCall(echoFunc, []ssa.Value{y.EmitConstInstPlaceholder(raw.GetText())})
	y.EmitCall(call)
	return nil
}

func (y *builder) VisitHtmlElement(raw phpparser.IHtmlElementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.HtmlElementContext)
	if i == nil {
		return nil
	}
	return nil
}

func (y *builder) VisitScriptText(raw phpparser.IScriptTextContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ScriptTextContext)
	if i == nil {
		return nil
	}
	return nil
}
