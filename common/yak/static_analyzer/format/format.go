package format

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	resultSpec "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

const issueSeparator = "------------------------"

// CheckAndFormat runs static analysis and returns formatted messages for AI feedback or copy.
func CheckAndFormat(code string, opts ...Option) (formatted string, hasBlockingErrors bool, results []*resultSpec.StaticAnalyzeResult) {
	cfg := applyOptions(opts)
	results = static_analyzer.YaklangScriptChecking(code, cfg.PluginType)
	if len(results) == 0 {
		return "", false, nil
	}
	formatted, hasBlockingErrors = FormatResults(code, results, opts...)
	return formatted, hasBlockingErrors, results
}

// FormatResults formats existing static analyze results.
func FormatResults(code string, results []*resultSpec.StaticAnalyzeResult, opts ...Option) (string, bool) {
	cfg := applyOptions(opts)
	if len(results) == 0 {
		return "", false
	}

	me := memedit.NewMemEditor(code)
	selected, haveMore := selectIssues(results, cfg.MaxIssues)

	var buf bytes.Buffer
	hasBlockingErrors := false
	for _, msg := range selected {
		blocking, chunk := formatSingleIssue(me, msg, cfg)
		if blocking {
			hasBlockingErrors = true
		}
		buf.WriteString(chunk)
	}

	if haveMore && cfg.TruncateMoreMessage != "" {
		buf.WriteString(issueSeparator)
		buf.WriteString(cfg.TruncateMoreMessage)
	}

	out := buf.String()
	if out != "" && consts.GetYakVersion() == "dev" {
		fmt.Println("==========================================================")
		fmt.Println("Check Yaklang Static Analysis Errors Output (Development Version):")
		fmt.Println(out)
		fmt.Println("==========================================================")
	}
	return out, hasBlockingErrors
}

// FormatSingleForCopy formats one issue for clipboard copy in the plugin editor.
func FormatSingleForCopy(code string, msg *resultSpec.StaticAnalyzeResult, opts ...Option) string {
	if msg == nil {
		return ""
	}
	cfg := applyOptions(opts)
	me := memedit.NewMemEditor(code)
	_, chunk := formatSingleIssue(me, msg, cfg)
	return strings.TrimSpace(chunk)
}

// HasBlockingErrors reports whether any result is an Error severity.
func HasBlockingErrors(results []*resultSpec.StaticAnalyzeResult) bool {
	for _, res := range results {
		if res != nil && res.Severity == resultSpec.Error {
			return true
		}
	}
	return false
}

func selectIssues(results []*resultSpec.StaticAnalyzeResult, maxIssues int) (selected []*resultSpec.StaticAnalyzeResult, haveMore bool) {
	var compilerErrors []*resultSpec.StaticAnalyzeResult
	var linkErrors []*resultSpec.StaticAnalyzeResult
	for _, res := range results {
		if res == nil {
			continue
		}
		if res.From == "compiler" && res.Severity == resultSpec.Error {
			compilerErrors = append(compilerErrors, res)
		} else {
			linkErrors = append(linkErrors, res)
		}
	}

	if len(compilerErrors) > 0 {
		selected = sortCompilerErrors(compilerErrors)
	} else {
		selected = sortLinkErrors(linkErrors)
	}

	if maxIssues > 0 && len(selected) > maxIssues {
		return selected[:maxIssues], true
	}
	return selected, false
}

func sortCompilerErrors(result []*resultSpec.StaticAnalyzeResult) []*resultSpec.StaticAnalyzeResult {
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartLineNumber != result[j].StartLineNumber {
			return result[i].StartLineNumber < result[j].StartLineNumber
		}
		return result[i].StartColumn < result[j].StartColumn
	})
	return result
}

func sortLinkErrors(result []*resultSpec.StaticAnalyzeResult) []*resultSpec.StaticAnalyzeResult {
	sort.Slice(result, func(i, j int) bool {
		if result[i].Severity != result[j].Severity {
			if result[i].Severity == resultSpec.Error {
				return true
			}
			if result[j].Severity == resultSpec.Error {
				return false
			}
		}
		if result[i].StartLineNumber != result[j].StartLineNumber {
			return result[i].StartLineNumber < result[j].StartLineNumber
		}
		return result[i].StartColumn < result[j].StartColumn
	})
	return result
}

func formatSingleIssue(me *memedit.MemEditor, msg *resultSpec.StaticAnalyzeResult, cfg Options) (blocking bool, chunk string) {
	if msg == nil {
		return false, ""
	}
	if msg.Severity == resultSpec.Error {
		blocking = true
	}

	display := *msg
	if cfg.LineBase > 0 {
		display.StartLineNumber += int64(cfg.LineBase)
		display.EndLineNumber += int64(cfg.LineBase)
	}

	var buf bytes.Buffer
	buf.WriteString(display.String())
	buf.WriteByte('\n')

	if cfg.IncludeHints {
		if hint := intelligentErrorHint(msg, me); hint != "" {
			if cfg.HintLabel != "" {
				buf.WriteString("\n")
				buf.WriteString(cfg.HintLabel)
			} else {
				buf.WriteString("\n")
			}
			buf.WriteString(hint)
			buf.WriteString("\n\n")
		}
	}

	if cfg.IncludeCodeContext && msg.StartLineNumber >= 0 && msg.EndLineNumber >= 0 && msg.EndLineNumber >= msg.StartLineNumber {
		markedErr := me.GetTextContextWithPrompt(
			memedit.NewRange(
				memedit.NewPosition(int(msg.StartLineNumber), int(msg.StartColumn)),
				memedit.NewPosition(int(msg.EndLineNumber), int(msg.EndColumn)),
			),
			3, display.String(),
		)
		if markedErr != "" {
			buf.WriteString(markedErr)
		}
	}
	buf.WriteString(issueSeparator)
	return blocking, buf.String()
}

func intelligentErrorHint(msg *resultSpec.StaticAnalyzeResult, me *memedit.MemEditor) string {
	if msg == nil || msg.Severity != resultSpec.Error {
		return ""
	}

	lineContent := ""
	if me != nil && msg.StartLineNumber > 0 {
		line, err := me.GetLine(int(msg.StartLineNumber))
		if err == nil {
			lineContent = strings.TrimSpace(line)
		}
	}
	return LookupCompilerErrorHintForMessage(msg.Message, lineContent)
}

// LookupCompilerErrorHintForMessage resolves AI hints from a raw analyzer message.
func LookupCompilerErrorHintForMessage(message, lineContent string) string {
	coreMessage := ExtractCoreCompilerMessage(message)
	return lookupCompilerErrorHint(coreMessage, lineContent)
}
