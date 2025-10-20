package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"sort"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	resultSpec "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

// checkCodeAndFormatErrors performs static analysis and formats error messages
// Returns: errorMessages string, hasBlockingErrors bool
func checkCodeAndFormatErrors(code string) (string, bool) {
	result := static_analyzer.YaklangScriptChecking(code, "yak")
	if len(result) <= 0 {
		return "", false
	}

	me := memedit.NewMemEditor(code)

	var buf bytes.Buffer
	hasBlockingErrors := false

	var compilerErrors []*resultSpec.StaticAnalyzeResult
	var linkErrors []*resultSpec.StaticAnalyzeResult
	for _, res := range result {
		if res.From == "compiler" && res.Severity == resultSpec.Error {
			compilerErrors = append(compilerErrors, res)
		} else {
			linkErrors = append(linkErrors, res)
		}
	}

	haveMore := false
	if len(compilerErrors) > 0 {
		// 专注解决一个错误
		result = compilerErrors
		sort.Slice(result, func(i, j int) bool {
			// Then by line number
			if result[i].StartLineNumber != result[j].StartLineNumber {
				return result[i].StartLineNumber < result[j].StartLineNumber
			}
			// Finally by column
			return result[i].StartColumn < result[j].StartColumn
		})

		if len(result) > 2 {
			haveMore = true
			result = result[:2]
		}
	} else {
		result = linkErrors
		sort.Slice(result, func(i, j int) bool {
			// First by severity (errors before others)
			if result[i].Severity != result[j].Severity {
				if result[i].Severity == resultSpec.Error {
					return true
				}
				if result[j].Severity == resultSpec.Error {
					return false
				}
			}
			// Then by line number
			if result[i].StartLineNumber != result[j].StartLineNumber {
				return result[i].StartLineNumber < result[j].StartLineNumber
			}
			// Finally by column
			return result[i].StartColumn < result[j].StartColumn
		})

		if len(result) > 2 {
			haveMore = true
			result = result[:2]
		}
	}

	for _, msg := range result {
		buf.WriteString(msg.String() + "\n")
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
			}
		}
		buf.WriteString("------------------------")

		// Check if there are any errors (not just warnings/hints)
		if !hasBlockingErrors && msg.Severity == resultSpec.Error {
			hasBlockingErrors = true
		}
	}

	if haveMore {
		buf.WriteString("------------------------")
		buf.WriteString("There are other errors, it's better to fix the critical issues above first before fixing others")
	}

	if buf.Len() > 0 {
		if consts.GetYakVersion() == "dev" && buf.String() != "" {
			fmt.Println("==========================================================")
			fmt.Println("Check Yaklang Static Analysis Errors Output (Development Version):")
			fmt.Println(buf.String())
			fmt.Println("==========================================================")
		}
	}
	return buf.String(), hasBlockingErrors
}
