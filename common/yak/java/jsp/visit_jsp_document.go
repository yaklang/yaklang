package jsp

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"regexp"
	"strings"
)

var jspIfBlockPattern = regexp.MustCompile(`(?s)^<c:if\b([^>]*)>(.*)</c:if>$`)
var jspIfBlockTestAttrPattern = regexp.MustCompile(`(?i)\btest\s*=\s*(?:"([^"]*)"|'([^']*)')`)

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
	for _, child := range i.GetChildren() {
		switch ret := child.(type) {
		case jspparser.IJspStartContext:
			y.VisitJspStart(ret)
		case jspparser.IJspDocumentContext:
			y.VisitJspDocument(ret)
		}
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
	} else if i.JspIfBlock() != nil {
		y.VisitJspIfBlock(i.JspIfBlock())
	} else if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
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
	if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
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
	} else if i.HtmlCloseElement() != nil {
		y.VisitHtmlCloseElement(i.HtmlCloseElement())
	} else if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
	} else if i.JspExpression() != nil {
		y.VisitJspExpression(i.JspExpression())
	} else if i.JspIfBlock() != nil {
		y.VisitJspIfBlock(i.JspIfBlock())
	} else if i.Style() != nil {
		return
	} else if i.JavaScript() != nil {
		return
	}

	if i.GetAfterContent() != nil {
		y.VisitHtmlMiscs(i.AllHtmlMiscs()[1])
	}
}

func (y *JSPVisitor) VisitHtmlCloseElement(raw jspparser.IHtmlCloseElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.HtmlCloseElementContext)
	if i == nil || i.CLOSE_TAG_BEGIN() == nil || i.HtmlTag() == nil || i.TAG_CLOSE() == nil {
		return
	}

	y.EmitPureText(i.CLOSE_TAG_BEGIN().GetText())
	y.EmitPureText(i.HtmlTag().GetText())
	y.EmitPureText(i.TAG_CLOSE().GetText())
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
	} else if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
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
	for _, element := range i.AllHtmlBeginElement() {
		y.VisitHtmlBeginElement(element)
	}
}

func (y *JSPVisitor) VisitHtmlBeginElement(raw jspparser.IHtmlBeginElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlBeginElementContext)
	if i == nil {
		return
	}
	if i.HtmlAttribute() != nil {
		y.VisitAttribute(i.HtmlAttribute())
		return
	}
	if i.TagJspFragment() != nil {
		y.AddAttrFunc(func() {
			y.VisitTagJspFragment(i.TagJspFragment())
		})
		return
	}
	if i.JspScriptlet() != nil {
		y.AddAttrFunc(func() {
			y.VisitJspScriptlet(i.JspScriptlet())
		})
	}
}

func (y *JSPVisitor) VisitTagJspFragment(raw jspparser.ITagJspFragmentContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.TagJspFragmentContext)
	if i == nil || i.TAG_JSP_IF_FRAGMENT() == nil {
		return
	}
	ast, err := Front(i.TAG_JSP_IF_FRAGMENT().GetText())
	if err != nil {
		log.Errorf("parse embedded jsp fragment error: %v", err)
		return
	}
	y.VisitJspDocuments(ast)
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
	for _, child := range i.GetChildren() {
		switch current := child.(type) {
		case jspparser.IHtmlChardataContext:
			y.VisitHtmlCharData(current)
		case jspparser.IHtmlContentContext:
			y.VisitHtmlContent(current)
		}
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

	tagType = JSP_TAG_PURE_HTML

	// pure HTML tag
	if i.JSP_JSTL_COLON() == nil {
		return
	}

	// jstl tag
	names := i.AllHtmlTagName()
	if len(names) != 2 {
		log.Errorf("Invalid JSP tag: %v", i.GetText())
		return
	}

	category := strings.ToLower(names[0].GetText())
	if category == "yak" && strings.EqualFold(names[1].GetText(), "fragment") {
		tagType = JSP_TAG_SYNTHETIC_FRAGMENT
		return
	}
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
	} else if i.JspElements() != nil || i.JspScript() != nil || i.JspIfBlock() != nil {
		// For unsupported nested template fragments inside attribute values,
		// preserve the original text so template2java keeps compiling.
		return i.GetText()
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
	} else if i.JspIfBlock() != nil {
		y.VisitJspIfBlock(i.JspIfBlock())
	} else if i.JspScript() != nil {
		y.VisitJspScript(i.JspScript())
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

func (y *JSPVisitor) VisitJspIfBlock(raw jspparser.IJspIfBlockContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.JspIfBlockContext)
	if i == nil || i.JSP_IF_BLOCK() == nil {
		return
	}
	matches := jspIfBlockPattern.FindStringSubmatch(i.JSP_IF_BLOCK().GetText())
	if len(matches) != 3 {
		y.EmitPureText(i.JSP_IF_BLOCK().GetText())
		return
	}
	attrText := matches[1]
	body := matches[2]
	attrMatches := jspIfBlockTestAttrPattern.FindStringSubmatch(attrText)
	if len(attrMatches) < 3 {
		y.EmitPureText(i.JSP_IF_BLOCK().GetText())
		return
	}
	condition := attrMatches[1]
	if condition == "" {
		condition = attrMatches[2]
	}
	condition = y.appendElParseMethod(condition)
	y.EmitPureCode("if (" + condition + ") {")
	if strings.TrimSpace(body) != "" {
		trimmedBody := strings.TrimSpace(body)
		if !strings.Contains(trimmedBody, "<") && !strings.Contains(trimmedBody, "${") && !strings.Contains(trimmedBody, "#{") {
			y.EmitPureText(body)
		} else {
			ast, err := Front(body)
			if err != nil {
				log.Errorf("parse jsp if body error: %v", err)
				y.EmitPureText(body)
			} else {
				y.VisitJspDocuments(ast)
			}
		}
	}
	y.EmitPureCode("}")
}
