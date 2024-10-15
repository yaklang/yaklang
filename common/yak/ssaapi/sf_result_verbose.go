package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

func (r *SyntaxFlowResult) GetAlertValues() *omap.OrderedMap[string, Values] {
	if r == nil {
		return omap.NewOrderedMap(make(map[string]Values))
	}
	if r.alertVariable == nil {
		r.GetAllVariable()
	}
	ret := omap.NewOrderedMap(make(map[string]Values))
	for _, name := range r.alertVariable {
		if vs := r.GetValues(name); vs != nil && len(vs) > 0 {
			ret.Set(name, vs)
		}
	}
	return ret
}

func (r *SyntaxFlowResult) Dump(showCode bool) string {
	indent := 0
	var buf bytes.Buffer

	lastLine := ""
	line := func(i string, items ...any) {
		if i == "" {
			return
		}

		if i != "" && lastLine != "" {
			results := buf.String()
			lastIdx := strings.LastIndex(results, lastLine)
			if lastIdx > 0 {
				buf.Reset()
				buf.WriteString(results[:lastIdx])
				newLine := strings.Replace(lastLine, "└", "├", 1)
				buf.WriteString(newLine)
			}
		}

		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(i, items...)
		} else {
			msg = i
		}

		lines := strings.Split(msg, "\n")

		for i := 0; i < len(lines); i++ {
			newBuf := bytes.NewBufferString("")
			if indent > 0 {
				var prefix = "*─" // "├─"
				if i == 0 {
					prefix = "├─"
				} else if i == len(lines)-1 {
					prefix = "└─"
				} else {
					prefix = "│ "
				}
				newBuf.WriteString(strings.Repeat("  ", indent) + prefix + " ")
			}
			newBuf.WriteString(strings.TrimRight(lines[i], "\r\n"))
			newBuf.WriteString("\n")
			if i == len(lines)-1 {
				lastLine = newBuf.String()
			}
			buf.WriteString(newBuf.String())
		}
	}
	increase := func() {
		indent++
	}
	decrease := func() {
		indent--
	}

	rule := r.rule
	line("RULE: %v", rule.RuleName)
	increase()
	line("title: %v", rule.Title)
	if rule.TitleZh != "" {
		line("title zh: %v", rule.TitleZh)
	}
	vals := r.GetAlertValues()
	if vals.Len() > 0 {
		line("ALERT RESULTS (%v):", vals.Len())
		increase()
		vals.ForEach(func(name string, vs Values) bool {
			line("ALERT: %v", name)
			increase()
			m := map[int64]any{}
			vs.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}

				_, existed := m[val.GetId()]
				if existed {
					return nil
				} else {
					m[val.GetId()] = true
				}

				line("VALUE: %v", val)
				increase()
				if extra, ok := r.GetAlertEx(name); extra != nil && ok {
					general := utils.InterfaceToGeneralMap(extra.ExtraInfo)
					haveMsg := false
					if extra.Msg != "" {
						line("Message: %v", extra.Msg)
						haveMsg = true
					}
					msg := utils.MapGetStringByManyFields(general, "msg", "message", "content")
					if msg != "" && !haveMsg {
						line("Message: %v", msg)
					}
					cve := utils.MapGetStringByManyFields(general, "cve", "Cve", "CVE")
					if cve != "" {
						line("CVE: %v", cve)
					}
					cwe := utils.MapGetStringByManyFields(general, "cwe", "Cwe", "CWE")
					if cwe != "" {
						line("CWE: %v", cwe)
					}

					if extra.Level != "" {
						line("Level: %v", extra.Level)
					}

				}
				line("ID: %v", val.GetId())
				line("Filename: %v", val.GetRange().GetEditor().GetFormatedUrl())
				if strings.Contains(rule.Title, "SCA:") {
					line("Reason: SCA: 根据依赖版本检查漏洞")
				} else {
					if !strings.Contains(val.GetSSAValue().String(), "\n") {
						line("Fixed Point(不动点)：%v", val.GetSSAValue().String())
					}
					if showCode {
						line("Source Code: \n%v", val.GetRange().GetTextContext(3))
					}
				}
				decrease()
				return nil
			})
			decrease()
			return true
		})
		decrease()
	}
	return buf.String()
}
