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

	for _, start := range i.AllJspStart() {
		y.VisitJspStart(start)
	}
	if i.Xml() != nil {
		return
	} else if i.Dtd() != nil {
		return
	} else {
		for _, element := range i.AllJspElements() {
			y.VisitJspElements(element)
		}
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
	if i.JspDirective() != nil {
		y.VisitJspDirective(i.JspDirective())
	}
	if i.Scriptlet() != nil {
		y.VisitScriptlet(i.Scriptlet())
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

	if i.JspElement() != nil {
		y.VisitJspElement(i.JspElement())
	} else if i.JspDirective() != nil {
		y.VisitJspDirective(i.JspDirective())
	} else if i.Scriptlet() != nil {
		y.VisitScriptlet(i.Scriptlet())
	}
}

func (y *JSPVisitor) VisitJspElement(raw jspparser.IJspElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.JspElementContext)
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
	i := raw.(*jspparser.HtmlAttributeContext)
	if i == nil {
		return
	}
	if i.HtmlAttributeName() != nil {
		key = i.HtmlAttributeName().GetText()
		if i.HtmlAttributeValue() != nil {
			value = i.HtmlAttributeValue().GetText()
		}
	}
	key = yakunquote.TryUnquote(key)
	value = yakunquote.TryUnquote(value)
	return
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
	if i.JspExpression() != nil {

	} else if i.JspElements() != nil {
		y.VisitJspElements(i.JspElements())
	} else if i.XhtmlCDATA() != nil {

	} else if i.HtmlComment() != nil {

	} else if i.Scriptlet() != nil {
		y.VisitScriptlet(i.Scriptlet())
	} else if i.JspDirective() != nil {
		y.VisitJspDirective(i.JspDirective())
	}
}

func (y *JSPVisitor) VisitScriptlet(raw jspparser.IScriptletContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.ScriptletContext)
	if i == nil {
		return
	}

	if i.SCRIPTLET_OPEN() != nil || i.DECLARATION_BEGIN() != nil {
		if i.BLOB_CONTENT() != nil {
			y.EmitPureCode(i.BLOB_CONTENT().GetText())
			return
		}
	}

	if i.ECHO_EXPRESSION_OPEN() != nil {
		if i.BLOB_CONTENT() != nil {
			y.EmitPureOutput(i.BLOB_CONTENT().GetText())
			return
		}
	}

}

func (y *JSPVisitor) VisitJspDirective(raw jspparser.IJspDirectiveContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.JspDirectiveContext)
	if i == nil {
		return
	}

	if i.HtmlTagName() == nil {
		return
	}

	name := i.HtmlTagName().GetText()
	tag := y.GetDirectiveTag(name)

	attrs := make(map[string]string)
	for _, attr := range i.AllHtmlAttribute() {
		key, value := y.VisitAttribute(attr)
		attrs[key] = value
	}
	y.PushTagInfo(tag, attrs)
	defer y.PopTagInfo()
	y.ParseSingleTag(i.GetText())
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
	for _, data := range i.AllHTML_TEXT() {
		texts := data.GetText()
		for _, text := range strings.Split(texts, "\n") {
			text = strings.TrimSpace(text)
			y.EmitPureText(text)
		}
	}
	if el := i.EL_EXPR(); el != nil {
		expr := y.fixElExpr(el.GetText())
		y.EmitPureOutput(expr)
	}
}
