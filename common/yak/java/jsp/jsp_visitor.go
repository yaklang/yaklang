package jsp

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"strings"
)

type JSPVisitor struct {
	*tl.Visitor
}

func NewJSPVisitor() *JSPVisitor {
	return &JSPVisitor{
		tl.NewVisitor(),
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
	} else {
		for _, element := range i.AllJspElements() {
			y.VisitJspElements(element)
		}
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
		return
	} else if i.Scriptlet() != nil {
		return
	}
}

func (y *JSPVisitor) VisitJspElement(raw jspparser.IJspElementContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch i := raw.(type) {
	case *jspparser.JspElementWithTagAndContentContext:
		tag := y.VisitHtmlTag(i.HtmlTag())
		if tag == JSP_TAG_PURE_HTML {
			y.EmitPureText(i.GetText(), y.CurrentRange)
			return
		}
		attrs := make(map[string]string)
		for _, attr := range i.AllHtmlAttribute() {
			key, value := y.VisitAttribute(attr)
			attrs[key] = value
			log.Infof("Tag: %v, Key: %v, Value: %v", tag, key, value)
		}
		y.ParseTag(tag, attrs)
	case *jspparser.JspElementWithOpenTagOnlyContext:
		tag := y.VisitHtmlTag(i.HtmlTag())
		if tag == JSP_TAG_PURE_HTML {
			y.EmitPureText(i.GetText(), y.CurrentRange)
			return
		}
		attrs := make(map[string]string)
		for _, attr := range i.AllHtmlAttribute() {
			key, value := y.VisitAttribute(attr)
			attrs[key] = value
			log.Infof("Tag: %v, Key: %v, Value: %v", tag, key, value)
		}
		y.ParseTag(tag, attrs)
	case *jspparser.JspElementWithSelfClosingTagContext:
		tag := y.VisitHtmlTag(i.HtmlTag())
		if tag == JSP_TAG_PURE_HTML {
			y.EmitPureText(i.GetText(), y.CurrentRange)
			return
		}
		attrs := make(map[string]string)
		for _, attr := range i.AllHtmlAttribute() {
			key, value := y.VisitAttribute(attr)
			attrs[key] = value
			log.Infof("Tag: %v, Key: %v, Value: %v", tag, key, value)
		}
		y.ParseTag(tag, attrs)
	default:
		log.Errorf("Unknown JSP element type: %T", i)
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

func (y *JSPVisitor) VisitHtmlContent(raw jspparser.IHtmlContentContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i := raw.(*jspparser.HtmlContentContext)
	if i == nil {
		return ""
	}
	return raw.GetText()
}
