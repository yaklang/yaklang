package yakast

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakfmt"
)

// GetFormattedCode is a lazy compatibility API. Compilation never builds
// formatted text or retains a syntax tree for this method. Explicit callers
// reparse the source through the independent formatter and cache its result.
func (y *YakCompiler) GetFormattedCode() string {
	if y.formatInput == nil {
		return ""
	}
	if !y.formatReady {
		source := y.formatInput.GetText(0, y.formatInput.Size()-1)
		formatted, _ := yakfmt.Format(source)
		y.formattedCode = strings.TrimSuffix(formatted, "\n")
		y.formatReady = true
	}
	return y.formattedCode
}
