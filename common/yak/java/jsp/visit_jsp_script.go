package jsp

import jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"

func (y *JSPVisitor) VisitJspScript(raw jspparser.IJspScriptContext) {
	if y == nil || raw == nil {
		return
	}

	i := raw.(*jspparser.JspScriptContext)
	if i == nil {
		return
	}

	if i.JspDirective() != nil {
		y.VisitJspDirective(i.JspDirective())
	} else if i.JspScriptlet() != nil {
		y.VisitJspScriptlet(i.JspScriptlet())
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

func (y *JSPVisitor) VisitJspScriptlet(raw jspparser.IJspScriptletContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.JspScriptletContext)
	if i == nil {
		return
	}

	if i.ScriptletStart() != nil {
		code := y.VisitScriptletContent(i.ScriptletContent())
		if code != "" {
			y.EmitPureCode(code)
		}
	} else if i.JspExpression() != nil {
		y.VisitJspExpression(i.JspExpression())
	}
}

func (y *JSPVisitor) VisitJspExpression(raw jspparser.IJspExpressionContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.JspExpressionContext)
	if i == nil {
		return
	}
	if i.ScriptletContent() != nil {
		expr := y.VisitScriptletContent(i.ScriptletContent())
		if expr != "" {
			y.EmitOutput(expr)
		}
	}
}

func (y *JSPVisitor) VisitScriptletContent(raw jspparser.IScriptletContentContext) string {
	if y == nil || raw == nil {
		return ""
	}
	i := raw.(*jspparser.ScriptletContentContext)
	if i == nil {
		return ""
	}
	return i.BLOB_CONTENT().GetText()
}
