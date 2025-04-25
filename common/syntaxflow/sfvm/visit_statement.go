package sfvm

import (
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/schema"

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

	extraDesc := NewExtraDesc()
	haveDesc := false
	for _, item := range i.DescriptionItems().(*sf.DescriptionItemsContext).AllDescriptionItem() {
		ret, ok := item.(*sf.DescriptionItemContext)
		if !ok || ret.Comment() != nil { // skip comment
			continue
		}
		key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
		value := ""
		if valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext); ok && valueItem != nil {
			if valueItem.HereDoc() != nil {
				value = y.VisitHereDoc(valueItem.HereDoc())
			} else if valueItem.StringLiteral() != nil {
				value = mustUnquoteSyntaxFlowString(valueItem.StringLiteral().GetText())
			} else {
				value = valueItem.GetText()
			}
		}

		if value != "" {
			switch keyLower := strings.ToLower(key); keyLower {
			case "title":
				y.rule.Title = value
			case "title_zh":
				y.rule.TitleZh = value
			case "description", "desc", "note":
				y.rule.Description = value
			case "type", "purpose":
				y.rule.Purpose = schema.ValidPurpose(value)
			case "lib", "allow_include", "as_library", "as_lib", "library_name":
				y.rule.AllowIncluded = !strings.EqualFold(value, "")
				if y.rule.AllowIncluded {
					y.rule.IncludedName = value
					y.rule.Title = value
				}
			case "level", "severity", "sev":
				y.rule.Severity = schema.ValidSeverityType(value)
			case "language", "lang":
				y.rule.Language = value
			case "cve":
				y.rule.CVE = value
			case "risk_type", "risk":
				y.rule.RiskType = value
			case "solution", "fix":
				y.rule.Solution = value
			case "rule_id", "id":
				y.rule.RuleId = value
			default:
				haveDesc = true
				if strings.Contains(keyLower, "://") {
					urlIns, _ := url.Parse(keyLower)
					if urlIns != nil {
						switch ret := urlIns.Scheme; ret {
						case "file", "fs", "filesystem":
							if strings.HasPrefix(keyLower, ret+"://") {
								filename := strings.TrimPrefix(keyLower, ret+"://")
								extraDesc.verifyFilesystem[filename] = value
								continue
							}
						case "safe-file", "safefile", "safe-fs", "safefs", "safe-filesystem", "safefilesystem", "negative-file", "negativefs", "nfs":
							if strings.HasPrefix(keyLower, ret+"://") {
								filename := strings.TrimPrefix(keyLower, ret+"://")
								extraDesc.negativeFilesystem[filename] = value
								continue
							}
						}
					}
				}
				extraDesc.rawDesc[key] = value
			}
		}
		if haveDesc {
			y.verifyFsInfo = append(y.verifyFsInfo, extraDesc)
		}
		y.EmitAddDescription(key, value)
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
	ref := i.RefVariable().GetText()
	ref = strings.TrimLeft(ref, "$")
	extra := y.rule.AlertDesc[ref]
	if extra == nil {
		extra = &schema.SyntaxFlowDescInfo{}
		y.rule.AlertDesc[ref] = extra
	}
	if extra.ExtraInfo == nil {
		extra.ExtraInfo = make(map[string]string)
	}
	if i.DescriptionItems() != nil {
		for _, item := range i.DescriptionItems().(*sf.DescriptionItemsContext).AllDescriptionItem() {
			if ret, ok := item.(*sf.DescriptionItemContext); ok {
				key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
				value := ""
				if valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext); ok && valueItem != nil {
					if valueItem.HereDoc() != nil {
						value = y.VisitHereDoc(valueItem.HereDoc())
					} else if valueItem.StringLiteral() != nil {
						value = mustUnquoteSyntaxFlowString(valueItem.StringLiteral().GetText())
					} else {
						value = valueItem.GetText()
					}
				}
				if value != "" {
					switch keyLower := strings.ToLower(key); keyLower {
					case "title":
						extra.Title = value
					case "title_zh":
						extra.TitleZh = value
					case "description", "desc", "note":
						extra.Description = value
					case "type", "purpose":
						extra.Purpose = schema.ValidPurpose(value)
					case "level", "severity", "sev":
						extra.Severity = schema.ValidSeverityType(value)
					case "message", "msg":
						extra.Msg = value
					case "cve":
						extra.CVE = value
					case "risk_type", "risk":
						extra.RiskType = value
					default:
						extra.ExtraInfo[key] = value
					}
				}
			}
		}
	}
	if i.StringLiteral() != nil {
		text := i.StringLiteral().GetText()
		forString := mustUnquoteSyntaxFlowString(text)
		extra.Msg = forString
		extra.OnlyMsg = true
	}

	y.EmitAlert(ref)
}
