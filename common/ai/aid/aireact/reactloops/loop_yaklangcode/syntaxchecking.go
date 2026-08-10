package loop_yaklangcode

import (
	"github.com/yaklang/yaklang/common/yak/static_analyzer/format"
)

// checkCodeAndFormatErrors performs static analysis and formats error messages.
// codeLineBase is the 0-based offset from snippet-relative lines to absolute editor lines
// (LoopVarCodeLineBase). Displayed Start/End line numbers are shifted by this base so they
// match CurrentCodeWithLineNumber; memedit context still uses relative positions.
// Returns: errorMessages string, hasBlockingErrors bool
func checkCodeAndFormatErrors(code string, codeLineBase ...int) (string, bool) {
	lineBase := 0
	if len(codeLineBase) > 0 && codeLineBase[0] > 0 {
		lineBase = codeLineBase[0]
	}
	msg, blocking, _ := format.CheckAndFormat(code, format.YakRunnerDefaults(lineBase)...)
	return msg, blocking
}
