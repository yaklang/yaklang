package jsp

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"strings"
)

type JSPVisitor struct {
	*tl.Visitor
	tagStack *utils.Stack[*TagInfo]
}

type TagInfo struct {
	typ   TagType
	attrs map[string]string
}

func newTagInfo(tag TagType, attrs map[string]string) *TagInfo {
	return &TagInfo{
		typ:   tag,
		attrs: attrs,
	}
}

func NewJSPVisitor() *JSPVisitor {
	return &JSPVisitor{
		tl.NewVisitor(),
		utils.NewStack[*TagInfo](),
	}
}

func (y *JSPVisitor) PushTagInfo(tag TagType, attrs map[string]string) {
	y.tagStack.Push(newTagInfo(tag, attrs))
}

func (y *JSPVisitor) PopTagInfo() {
	y.tagStack.Pop()
}

func (y *JSPVisitor) PeekTagInfo() *TagInfo {
	t := y.tagStack.Peek()
	return t
}

func (y *JSPVisitor) VisitJspDocuments(raw jspparser.IJspDocumentsContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.JspDocumentsContext)
	if i == nil {
		return
	}
	for _, doc := range i.AllJspDocument() {
		y.VisitJspDocument(doc)
	}

	for _, start := range i.AllJspStart() {
		y.VisitJspStart(start)
	}
}

func (y *JSPVisitor) VisitJspDocument(raw jspparser.IJspDocumentContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.JspDocumentContext)
	if i == nil {
		return
	}

	if i.Xml() != nil {
		return
	} else if i.Dtd() != nil {
		return
	} else if i.JspElements() != nil {
		y.VisitJspElements(i.JspElements())
	}
}

func (y *JSPVisitor) VisitJspStart(raw jspparser.IJspStartContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.JspStartContext)
	if i == nil {
		return
	}

}

func (y *JSPVisitor) VisitJspElements(raw jspparser.IJspElementsContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.JspElementsContext)
	if i == nil {
		return
	}

	if i.GetBeforeContent() != nil {
		y.VisitHtmlMiscs(i.AllHtmlMiscs()[0])
	}

	if i.HtmlElement() != nil {
		y.VisitHtmlElement(i.HtmlElement())
	} else if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
	} else if i.JspExpression() != nil {
		y.VisitJspExpression(i.JspExpression())
	} else if i.Style() != nil {
		return
	} else if i.JavaScript() != nil {
		return
	}

	if i.GetAfterContent() != nil {
		y.VisitHtmlMiscs(i.AllHtmlMiscs()[1])
	}
}

func (y *JSPVisitor) VisitHtmlMiscs(raw jspparser.IHtmlMiscsContext) {
	if y == nil || raw == nil {
		return
	}

	i := raw.(*jspparser.HtmlMiscsContext)
	for _, misc := range i.AllHtmlMisc() {
		y.VisitHtmlMisc(misc)
	}
}

func (y *JSPVisitor) VisitHtmlMisc(raw jspparser.IHtmlMiscContext) {
	if y == nil || raw == nil {
		return
	}

	i := raw.(*jspparser.HtmlMiscContext)
	if i == nil {
		return
	}

	if i.HtmlComment() != nil {
		return
	} else if i.ElExpression() != nil {
		y.VisitElExpression(i.ElExpression())
	} else if i.JspScriptlet() != nil {
		y.VisitJspScriptlet(i.JspScriptlet())
	}
}

func (y *JSPVisitor) VisitElExpression(raw jspparser.IElExpressionContext) {
	if y == nil || raw == nil {
		return
	}

	i := raw.(*jspparser.ElExpressionContext)
	if i == nil {
		return
	}
}

func (y *JSPVisitor) VisitHtmlElement(raw jspparser.IHtmlElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.HtmlElementContext)
	if i == nil {
		return
	}

	if i.HtmlBegin() != nil {
		y.VisitHtmlBegin(i.HtmlBegin())
	}
	defer y.PopTagInfo()

	// self closing tag
	if i.TAG_SLASH_END() != nil {
		y.ParseSingleTag(i.GetText())
		return
	}

	if i.CLOSE_TAG_BEGIN() != nil {
		openTag := i.HtmlBegin().GetText() + i.TAG_CLOSE(0).GetText()
		closedTag := i.CLOSE_TAG_BEGIN().GetText() + i.HtmlTag().GetText() + i.TAG_CLOSE(1).GetText()
		y.ParseDoubleTag(openTag, closedTag, func() {
			y.VisitHtmlContents(i.HtmlContents())
		})
	} else {
		// only open tag
		y.ParseSingleTag(i.GetText())
	}
}

func (y *JSPVisitor) VisitHtmlBegin(raw jspparser.IHtmlBeginContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlBeginContext)
	if i == nil {
		return
	}
	if i.HtmlTag() == nil {
		return
	}
	tag := y.VisitHtmlTag(i.HtmlTag())
	attrs := make(map[string]string)
	for _, attr := range i.AllHtmlAttribute() {
		key, value := y.VisitAttribute(attr)
		attrs[key] = value
	}
	y.PushTagInfo(tag, attrs)
}

func (y *JSPVisitor) VisitHtmlContents(raw jspparser.IHtmlContentsContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlContentsContext)
	if i == nil {
		return
	}
	for _, content := range i.AllHtmlContent() {
		y.VisitHtmlContent(content)
	}
	for _, data := range i.AllHtmlChardata() {
		y.VisitHtmlCharData(data)
	}
}

func (y *JSPVisitor) VisitHtmlTag(raw jspparser.IHtmlTagContext) (tagType TagType) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlTagContext)
	if i == nil {
		return
	}

	// pure HTML tag
	if i.JSP_JSTL_COLON() == nil {
		tagType = JSP_TAG_PURE_HTML
		return
	}

	// jstl tag
	names := i.AllHtmlTagName()
	if len(names) != 2 {
		log.Errorf("Invalid JSP tag: %v", i.GetText())
		return
	}

	category := strings.ToLower(names[0].GetText())
	// core jstl tag
	if category == "c" {
		tagType = y.GetCoreJSTLTag(strings.ToLower(names[1].GetText()))
		return
	}
	return
}

func (y *JSPVisitor) VisitAttribute(raw jspparser.IHtmlAttributeContext) (key string, value string) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	switch ret := raw.(type) {
	case *jspparser.PureHTMLAttributeContext:
		key = ret.HtmlAttributeName().GetText()
	case *jspparser.EqualHTMLAttributeContext:
		key = ret.HtmlAttributeName().GetText()
		value = y.VisitHtmlAttributeValue(ret.HtmlAttributeValue())
	case *jspparser.JSPExpressionAttributeContext:
		y.VisitJspExpression(ret.JspExpression())
	}
	key = yakunquote.TryUnquote(key)
	value = yakunquote.TryUnquote(value)
	return
}

func (y *JSPVisitor) VisitHtmlAttributeValue(raw jspparser.IHtmlAttributeValueContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlAttributeValueContext)
	if i == nil {
		return ""
	}
	// TODO:return value
	for _, element := range i.AllHtmlAttributeValueElement() {
		y.VisitHtmlAttributeValueElement(element)
	}
	return ""
}

func (y *JSPVisitor) VisitHtmlAttributeValueElement(raw jspparser.IHtmlAttributeValueElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.HtmlAttributeValueElementContext)
	if i == nil {
		return
	}
	if i.ATTVAL_ATTRIBUTE() != nil {
		y.EmitPureText(i.ATTVAL_ATTRIBUTE().GetText())
	} else if i.JspExpression() != nil {
		y.VisitJspExpression(i.JspExpression())
	} else if i.ElExpression() != nil {
		y.VisitElExpression(i.ElExpression())
	}
}

func (y *JSPVisitor) VisitHtmlContent(raw jspparser.IHtmlContentContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlContentContext)
	if i == nil {
		return
	}
	if i.ElExpression() != nil {
		y.VisitElExpression(i.ElExpression())
	} else if i.JspElements() != nil {
		y.VisitJspElements(i.JspElements())
	} else if i.XhtmlCDATA() != nil {
		return
	} else if i.HtmlComment() != nil {
		return
	}
}

func (y *JSPVisitor) VisitHtmlCharData(raw jspparser.IHtmlChardataContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlChardataContext)
	if i == nil {
		return
	}

	if i.JSP_STATIC_CONTENT_CHARS() != nil {
		str := i.JSP_STATIC_CONTENT_CHARS().GetText()
		y.EmitPureText(str)
	}
}
