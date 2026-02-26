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

	errs := vm.GetCompileErrors()
	if len(errs) == 0 {
		// Fallback: no structured errors, use raw error message
		buf.WriteString(fmt.Sprintf("SyntaxFlow 编译错误: %s\n", err.Error()))
		buf.WriteString("------------------------")
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

	return strings.TrimSpace(buf.String()), hasBlockingErrors
}
