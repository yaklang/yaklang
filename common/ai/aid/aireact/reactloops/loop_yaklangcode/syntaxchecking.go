package loop_yaklangcode

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	resultSpec "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

// checkCodeAndFormatErrors performs static analysis and formats error messages
// Returns: errorMessages string, hasBlockingErrors bool
func checkCodeAndFormatErrors(code string) (string, bool) {
	result := static_analyzer.YaklangScriptChecking(code, "yak")

	me := memedit.NewMemEditor(code)

	var buf bytes.Buffer
	hasBlockingErrors := false

	for _, msg := range result {
		if msg.StartLineNumber >= 0 && msg.EndLineNumber >= 0 && msg.EndLineNumber >= msg.StartLineNumber {
			markedErr := me.GetTextContextWithPrompt(
				memedit.NewRange(
					memedit.NewPosition(int(msg.StartLineNumber), int(msg.StartColumn)),
					memedit.NewPosition(int(msg.EndLineNumber), int(msg.EndColumn)),
				),
				3, msg.String(),
			)
			if markedErr != "" {
				buf.WriteString(markedErr)
				buf.WriteString("---")
			}
		} else {
			buf.WriteString(msg.String() + "\n")
		}

		// Check if there are any errors (not just warnings/hints)
		if !hasBlockingErrors && msg.Severity == resultSpec.Error {
			hasBlockingErrors = true
		}
	}

	return buf.String(), hasBlockingErrors
}
