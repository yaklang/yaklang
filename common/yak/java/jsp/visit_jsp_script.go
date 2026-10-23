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

	y.PushTagInfo(tag)
	defer y.PopTagInfo()
	for _, attr := range i.AllHtmlAttribute() {
		y.VisitAttribute(attr)
	}
	y.ParseSingleTag()
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
		start := i.ScriptletStart().(*jspparser.ScriptletStartContext)
		code := y.VisitScriptletContent(i.ScriptletContent())
		if code != "" {
			if start.SCRIPTLET_OPEN() != nil {
				y.EmitPureCode(code)
			} else if start.DECLARATION_BEGIN() != nil {
				y.EmitDeclarationCode(code)
			}
		}
	} else if i.JspExpression() != nil {
		if expr := y.VisitJspExpression(i.JspExpression()); expr != "" {
			y.EmitOutput(expr)
		}
	}
}

func (y *JSPVisitor) VisitJspExpression(raw jspparser.IJspExpressionContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*jspparser.JspExpressionContext)
	if i == nil {
		return ""
	}
	if i.ScriptletContent() != nil {
		// Callers decide whether this is a statement (<%= 1+1 %>) or an
		// attribute value (test="<%= expr %>"). Emitting here put out.print
		// inside switch (true) before any case label.
		return normalizeJSPEmbeddedJava(y.VisitScriptletContent(i.ScriptletContent()))
	}
	return ""
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

func normalizeJSPEmbeddedJava(code string) string {
	if code == "" {
		return code
	}

	out := make([]byte, 0, len(code))
	inDouble := false
	inSingle := false
	escaped := false

	for i := 0; i < len(code); i++ {
		ch := code[i]

		if inDouble || inSingle {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if inDouble && ch == '"' {
				inDouble = false
			} else if inSingle && ch == '\'' {
				inSingle = false
			}
			continue
		}

		if ch == '\\' && i+1 < len(code) && (code[i+1] == '"' || code[i+1] == '\'') {
			out = append(out, code[i+1])
			i++
			continue
		}

		out = append(out, ch)
		if ch == '"' {
			inDouble = true
		} else if ch == '\'' {
			inSingle = true
		}
	}
	return string(out)
}
