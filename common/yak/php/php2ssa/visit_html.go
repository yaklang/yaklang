package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitHtmlDocument(raw phpparser.IHtmlDocumentContext) interface{} {
	if y == nil || raw == nil {
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

	elements := i.AllHtmlDocumentElement()
	for _, el := range elements {
		y.VisitHtmlDocumentElement(el)
	}

	return nil
}

func (y *builder) VisitHtmlDocumentElement(raw phpparser.IHtmlDocumentElementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.HtmlDocumentElementContext)
	if i == nil {
		return nil
	}

	if ret := i.InlineHtml(); ret != nil {
		y.VisitInlineHtml(ret)
	} else if ret := i.PhpBlock(); ret != nil {
		y.VisitPhpBlock(ret)
	}

	return nil
}

func (y *builder) VisitInlineHtml(raw phpparser.IInlineHtmlContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InlineHtmlContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitInlineHtmlStatement(raw phpparser.IInlineHtmlStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InlineHtmlStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
