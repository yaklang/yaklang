package sfvm

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
)

func mustUnquoteSyntaxFlowString(text string) string {
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		afterText, err := yakunquote.Unquote(text)
		if err != nil {
			text = text[1 : len(text)-1]
		} else {
			text = afterText
		}
	}
	return text
}

func (y *SyntaxFlowVisitor) VisitCheckStatement(raw sf.ICheckStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.CheckStatementContext)
	if i == nil {
		return nil
	}

	var thenString string
	var elseString string

	if i.ThenExpr() != nil {
		text := i.ThenExpr().(*sf.ThenExprContext).StringLiteral().GetText()
		thenString = mustUnquoteSyntaxFlowString(text)
	}

	if i.ElseExpr() != nil {
		text := i.ElseExpr().(*sf.ElseExprContext).StringLiteral().GetText()
		elseString = mustUnquoteSyntaxFlowString(text)
	}

	ref := i.RefVariable().GetText()
	ref = strings.TrimLeft(ref, "$")
	y.EmitCheckParam(ref, thenString, elseString)
	return nil
}

func (y *SyntaxFlowVisitor) VisitDescriptionStatement(raw sf.IDescriptionStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.DescriptionStatementContext)
	if i == nil {
		return nil
	}

	if i.DescriptionItems() == nil {
		return nil
	}

	for _, item := range i.DescriptionItems().(*sf.DescriptionItemsContext).AllDescriptionItem() {
		if ret, ok := item.(*sf.DescriptionItemContext); ok {
			results := ret.AllStringLiteral()
			if len(results) >= 2 {
				key := mustUnquoteSyntaxFlowString(results[0].GetText())
				value := mustUnquoteSyntaxFlowString(results[1].GetText())
				key = yakunquote.TryUnquote(key)
				value = yakunquote.TryUnquote(value)
				switch strings.ToLower(key) {
				case "title":
					y.title = value
				case "description", "desc", "note":
					y.description = value
				case "type", "purpose":
					y.purpose = value
				case "lib", "allow_include", "as_library", "as_lib", "library_name":
					y.allowIncluded = value
				}
				y.EmitAddDescription(key, value)
			} else {
				key := mustUnquoteSyntaxFlowString(results[0].GetText())
				key = yakunquote.TryUnquote(key)
				y.EmitAddDescription(key, "")
			}
		}
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitAlertStatement(raw sf.IAlertStatementContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*sf.AlertStatementContext)
	if i == nil {
		return
	}

	// i.RefVariable()
	ref := i.RefVariable().GetText()
	ref = strings.TrimLeft(ref, "$")

	if i.For() != nil {
		text := i.StringLiteral().GetText()
		forString := mustUnquoteSyntaxFlowString(text)
		y.EmitAlert(ref, forString)
	} else {
		y.EmitAlert(ref, "")
	}

}
