package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitHtmlDocument(raw phpparser.IHtmlDocumentContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.HtmlDocumentContext)
	if i == nil {
		return nil
	}

	return nil
}
