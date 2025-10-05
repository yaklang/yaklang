package loop_yaklangcode

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	resultSpec "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

// checkCodeAndFormatErrors performs static analysis and formats error messages
// Returns: errorMessages string, hasBlockingErrors bool
func checkCodeAndFormatErrors(code string) (string, bool) {
	result := static_analyzer.YaklangScriptChecking(code, "yak")
	var buf bytes.Buffer
	hasBlockingErrors := false

	for _, msg := range result {
		buf.WriteString(msg.String())
		buf.WriteString("\n")

		// Check if there are any errors (not just warnings/hints)
		if !hasBlockingErrors && msg.Severity == resultSpec.Error {
			hasBlockingErrors = true
		}
	}

	return buf.String(), hasBlockingErrors
}
