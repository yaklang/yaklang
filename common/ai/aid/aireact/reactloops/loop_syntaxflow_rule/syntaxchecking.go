package loop_syntaxflow_rule

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// checkSyntaxFlowAndFormatErrors performs SyntaxFlow rule syntax validation via sfvm.Compile
// Returns: errorMessages string, hasBlockingErrors bool
func checkSyntaxFlowAndFormatErrors(content string) (string, bool) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	_, err := vm.Compile(content)
	if err == nil {
		return "", false
	}

	me := memedit.NewMemEditor(content)
	var buf bytes.Buffer
	hasBlockingErrors := true // Syntax errors are always blocking for SF rules

	// 所有错误输出必须以 SyntaxFlow 标识开头，便于识别错误来源
	const syntaxFlowPrefix = "SyntaxFlow 编译错误: "

	errs := vm.GetCompileErrors()
	if len(errs) == 0 {
		// Fallback: no structured errors, use raw error message
		buf.WriteString(syntaxFlowPrefix)
		buf.WriteString(err.Error())
		buf.WriteString("\n------------------------")
		return buf.String(), hasBlockingErrors
	}

	// Sort errors by line then column
	sort.Slice(errs, func(i, j int) bool {
		si, sj := errs[i], errs[j]
		if si == nil || sj == nil {
			return false
		}
		lineI, lineJ := 0, 0
		if si.StartPos != nil {
			lineI = si.StartPos.GetLine()
		}
		if sj.StartPos != nil {
			lineJ = sj.StartPos.GetLine()
		}
		if lineI != lineJ {
			return lineI < lineJ
		}
		colI, colJ := 0, 0
		if si.StartPos != nil {
			colI = si.StartPos.GetColumn()
		}
		if sj.StartPos != nil {
			colJ = sj.StartPos.GetColumn()
		}
		return colI < colJ
	})

	// Limit to first 3 errors for clarity
	maxShow := 3
	if len(errs) < maxShow {
		maxShow = len(errs)
	}
	buf.WriteString(syntaxFlowPrefix)
	buf.WriteString("\n")
	// heredoc 结束符错误时输出明确错误类型
	errTextPreview := ""
	if len(errs) > 0 && errs[0] != nil {
		errTextPreview = errs[0].Error()
	}
	if strings.Contains(content, "<<<") && strings.Contains(errTextPreview, "mismatched input ':'") && strings.Contains(errTextPreview, "expecting <EOF>") {
		buf.WriteString("【错误类型】heredoc 结束符格式错误\n【原因】结束标识符有前导空格。【修复】结束符须行首无空格。\n------------------------\n")
	}
	for i := 0; i < maxShow; i++ {
		e := errs[i]
		if e == nil {
			continue
		}
		buf.WriteString(e.Error() + "\n")
		// Add context around error location
		if e.StartPos != nil && e.EndPos != nil {
			startLine := e.StartPos.GetLine()
			startCol := e.StartPos.GetColumn()
			endLine := e.EndPos.GetLine()
			endCol := e.EndPos.GetColumn()
			if startLine >= 0 && endLine >= 0 {
				markedErr := me.GetTextContextWithPrompt(
					memedit.NewRange(
						memedit.NewPosition(startLine, startCol),
						memedit.NewPosition(endLine, endCol),
					),
					3, e.Error(),
				)
				if markedErr != "" {
					buf.WriteString(markedErr)
				}
			}
		}
		buf.WriteString("------------------------\n")
	}

	if len(errs) > maxShow {
		buf.WriteString("------------------------\n")
		buf.WriteString(fmt.Sprintf("还有 %d 个错误，建议先修复以上关键问题\n", len(errs)-maxShow))
	}

	// 当错误疑似特定格式问题时，附加可操作建议
	errText := buf.String()
	if strings.Contains(content, "desc(") && (strings.Contains(errText, "missing ')'") || strings.Contains(errText, "mismatched input ','")) {
		buf.WriteString("------------------------\n")
		buf.WriteString("【desc 格式提示】若错误位于 desc 块内：字段必须为 fieldName: value（冒号不可省略），字段间用换行分隔、禁止用逗号。参考 golang-template-ssti.sf 的 desc 写法。\n")
	}
	// heredoc 结束符错误
	if strings.Contains(content, "<<<") && strings.Contains(errText, "mismatched input ':'") && strings.Contains(errText, "expecting <EOF>") {
		buf.WriteString("------------------------\n")
		buf.WriteString("【heredoc 结束符错误】heredoc 结束标识符必须**行首无空格**。错误：`    TEXT`。正确：换行后紧跟 `TEXT` 无空格。参考 golang-reflected-xss-gin-context.sf。\n")
	}

	return strings.TrimSpace(buf.String()), hasBlockingErrors
}
