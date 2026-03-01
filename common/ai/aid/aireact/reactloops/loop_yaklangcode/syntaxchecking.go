package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
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

		// Add intelligent error hints for common Yaklang DSL issues
		intelligentHint := getIntelligentErrorHint(msg, me)
		if intelligentHint != "" {
			buf.WriteString("\nAI助手提示: " + intelligentHint + "\n\n")
		}

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

// ErrorPattern represents a pattern for detecting specific syntax errors
type ErrorPattern struct {
	Name        string
	ErrorGlobs  []string // Error message patterns
	LineGlobs   []string // Line content patterns
	LineRegexps []string // Line content regexps
	Hint        string
	Examples    []string // [incorrect, correct] pairs
}

// Common Yaklang DSL error patterns
var yaklangErrorPatterns = []ErrorPattern{
	{
		Name:       "FunctionParameterTypes",
		ErrorGlobs: []string{"*no viable alternative at input*", "*func(*"},
		LineRegexps: []string{
			`func\s*\([^)]*\s+(map\[|string|int|interface\{\}|\[\]|\*|chan)`,
		},
		Hint: "Yaklang DSL 中函数参数不允许有类型声明。请移除参数的类型声明。",
		Examples: []string{
			"func(result map[string]interface{})",
			"func(result)",
		},
	},
	{
		Name:       "VarTypeDeclarations",
		ErrorGlobs: []string{"*no viable alternative*", "*extraneous input*", "*mismatched input*"},
		LineRegexps: []string{
			`var\s+\w+\s+(map\[|\[\]|string|int|interface\{\}|\*|chan)`,
			`\w+\s*:=\s*(map\[|\[\]string|\[\]int)`,
		},
		Hint: "Yaklang DSL 中变量声明不需要显式类型。请使用简单的赋值语法。",
		Examples: []string{
			"var result map[string]interface{}",
			"result := {}",
		},
	},
	{
		Name:       "IncompleteStructure",
		ErrorGlobs: []string{"*mismatched input*", "*expecting <EOF>*"},
		Hint:       "语法结构不完整，可能缺少匹配的括号、花括号或分号。请检查代码块的完整性。",
	},
	{
		Name:       "ArraySliceSyntax",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineGlobs:  []string{"*[]*{*", "*[]string*", "*[]int*"},
		Hint:       "Yaklang DSL 中数组/切片语法可能与 Go 不同。请使用 Yaklang 的数组语法。",
		Examples: []string{
			`[]string{"a", "b"}`,
			`["a", "b"]`,
		},
	},
	{
		Name:       "ImportStatements",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineGlobs:  []string{"*import*"},
		Hint:       "Yaklang DSL 不需要 import 语句。所有内置库都是自动可用的。请删除 import 语句。",
	},
	{
		Name:       "PackageDeclarations",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineGlobs:  []string{"*package*"},
		Hint:       "Yaklang DSL 不需要 package 声明。请删除 package 语句，直接编写代码逻辑。",
	},
	{
		Name:       "MethodReceivers",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineRegexps: []string{
			`func\s*\([^)]+\)\s*\w+\s*\(`,
		},
		Hint: "Yaklang DSL 不支持方法接收者语法。请使用普通函数定义。",
		Examples: []string{
			"func (t *Type) Method()",
			"func Method()",
		},
	},
	{
		Name:       "GenericSyntax",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineGlobs:  []string{"*<*>*"},
		Hint:       "Yaklang DSL 不支持泛型语法。请使用具体类型或 interface{}。",
	},
	{
		Name:       "PointerSyntax",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineRegexps: []string{
			`[^"]*\*[^"]*`, // * not in string
		},
		Hint: "Yaklang DSL 中指针语法可能不同。请检查是否需要指针，或使用 Yaklang 的引用方式。",
	},
	{
		Name:       "ChannelSyntax",
		ErrorGlobs: []string{"*no viable alternative*"},
		LineGlobs:  []string{"*chan*"},
		Hint:       "Yaklang DSL 的并发模型可能与 Go 不同。请查阅 Yaklang 的并发语法文档。",
	},
}

// getIntelligentErrorHint provides intelligent hints for common Yaklang DSL syntax errors
func getIntelligentErrorHint(msg *resultSpec.StaticAnalyzeResult, me *memedit.MemEditor) string {
	if msg == nil || msg.Severity != resultSpec.Error {
		return ""
	}

	// Get the problematic line content
	lineContent := ""
	if msg.StartLineNumber > 0 {
		line, err := me.GetLine(int(msg.StartLineNumber))
		if err == nil {
			lineContent = strings.TrimSpace(line)
		}
	}

	errorMessage := msg.Message

	// Check each pattern
	for _, pattern := range yaklangErrorPatterns {
		if matchesErrorPattern(pattern, errorMessage, lineContent) {
			return formatErrorHint(pattern)
		}
	}

	return ""
}

// matchesErrorPattern checks if an error matches a specific pattern
func matchesErrorPattern(pattern ErrorPattern, errorMessage, lineContent string) bool {
	// Check error message patterns
	if len(pattern.ErrorGlobs) > 0 {
		matched := false
		for _, glob := range pattern.ErrorGlobs {
			// Use safe glob matching to avoid panic
			if safeGlobMatch(errorMessage, glob) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check line content patterns (globs)
	if len(pattern.LineGlobs) > 0 {
		matched := false
		for _, glob := range pattern.LineGlobs {
			// Use safe glob matching to avoid panic
			if safeGlobMatch(lineContent, glob) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check line content patterns (regexps)
	if len(pattern.LineRegexps) > 0 {
		if !utils.MatchAnyOfRegexp(lineContent, pattern.LineRegexps...) {
			return false
		}
	}

	return true
}

// safeGlobMatch performs glob matching with error handling to avoid panics
func safeGlobMatch(text, pattern string) bool {
	defer func() {
		if r := recover(); r != nil {
			// If glob compilation fails, fall back to substring matching
			return
		}
	}()

	// Try utils.MatchAnyOfGlob first
	return utils.MatchAnyOfGlob(text, pattern)
}

// formatErrorHint formats the error hint with examples
func formatErrorHint(pattern ErrorPattern) string {
	hint := pattern.Hint

	if len(pattern.Examples) >= 2 {
		hint += "\n错误: " + pattern.Examples[0]
		hint += "\n正确: " + pattern.Examples[1]
	}

	return hint
}
