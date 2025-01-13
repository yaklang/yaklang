package jsp

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"strings"
)

type JSPVisitor struct {
	*tl.Visitor
	tagStack *utils.Stack[*TagInfo]
}

func NewJSPVisitor() *JSPVisitor {
	return &JSPVisitor{
		tl.NewVisitor(),
		utils.NewStack[*TagInfo](),
	}
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
		text := y.VisitElExpression(i.ElExpression())
		y.EmitOutput(text)
	} else if i.JspScriptlet() != nil {
		y.VisitJspScriptlet(i.JspScriptlet())
	}
}

func (y *JSPVisitor) VisitElExpression(raw jspparser.IElExpressionContext) string {
	if y == nil || raw == nil {
		return ""
	}

	i := raw.(*jspparser.ElExpressionContext)
	if i == nil {
		return ""
	}
	content := i.GetText()
	return y.appendElParseMethod(content)
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
		defer y.PopTagInfo()
	}

	if i.TAG_SLASH_END() != nil {
		// single tag
		y.AddAttrFunc(func() {
			y.EmitPureText(i.TAG_SLASH_END().GetText())
		})
		y.ParseSingleTag()
	} else {
		if i.CLOSE_TAG_BEGIN() != nil {
			// double tag
			y.AddAttrFunc(func() {
				y.EmitPureText(i.TAG_CLOSE(0).GetText())
			})
			if i.HtmlContents() != nil {
				end := i.CLOSE_TAG_BEGIN().GetText() + i.HtmlTag().GetText() + i.TAG_CLOSE(1).GetText()
				y.ParseDoubleTag(end, func() {
					y.VisitHtmlContents(i.HtmlContents())
				})
			}
		} else {
			// single tag
			y.AddAttrFunc(func() {
				y.EmitPureText(i.TAG_CLOSE(0).GetText())
			})
			y.ParseSingleTag()
		}
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
	y.PushTagInfo(tag)
	y.AddAttrFunc(func() {
		y.EmitPureText(i.TAG_BEGIN().GetText() + i.HtmlTag().GetText())
	})
	for _, attr := range i.AllHtmlAttribute() {
		y.VisitAttribute(attr)
	}
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
	for _, data := range i.AllHtmlChardata() {
		y.VisitHtmlCharData(data)
	}
	for _, content := range i.AllHtmlContent() {
		y.VisitHtmlContent(content)
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

func (y *JSPVisitor) VisitAttribute(raw jspparser.IHtmlAttributeContext) (key, value string) {
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
		y.AddTagAttr(key, value)
	case *jspparser.JSPExpressionAttributeContext:
		y.VisitJspExpression(ret.JspExpression())
	}
	y.AddAttrFunc(func() {
		y.EmitPureText(key)
		y.EmitPureText("=")
		y.EmitPureText(value)
	})
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
	value := ""
	for _, element := range i.AllHtmlAttributeValueElement() {
		value += y.VisitHtmlAttributeValueElement(element)
	}
	return value
}

func (y *JSPVisitor) VisitHtmlAttributeValueElement(raw jspparser.IHtmlAttributeValueElementContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.HtmlAttributeValueElementContext)
	if i == nil {
		return ""
	}
	if i.ATTVAL_ATTRIBUTE() != nil {
		return i.ATTVAL_ATTRIBUTE().GetText()
	} else if i.JspExpression() != nil {
		return y.VisitJspExpression(i.JspExpression())
	} else if i.ElExpression() != nil {
		return y.VisitElExpression(i.ElExpression())
	}
	return ""
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
		text := y.VisitElExpression(i.ElExpression())
		y.EmitOutput(text)
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

func (y *JSPVisitor) EmitPureText(text string) {
	text = y.replaceElExprInText(text)
	y.Visitor.EmitPureText(text)
}
