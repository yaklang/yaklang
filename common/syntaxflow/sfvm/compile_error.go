package sfvm

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// formatSyntaxFlowParseError turns ANTLR parser errors into a message that
// names the problem and points at it.
//
// The shared handler formats errors for source files, where the offending
// token dump (`symbol: [@36,110:116='strcmp*',<88>,4:17]`) is useful for
// debugging a lexer. In a rule, the token index and stream offsets are noise:
// what the author needs is the rule line, the column, and what was expected.
func formatSyntaxFlowParseError(text string, errs antlr4util.SourceCodeErrors) error {
	if len(errs) == 0 {
		return utils.Errorf("SyntaxFlow compile error: unknown parse failure")
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.WriteString("SyntaxFlow compile error:")
	seen := make(map[string]struct{}, len(errs))
	for _, e := range errs {
		if e == nil || e.StartPos == nil {
			continue
		}
		line := e.StartPos.GetLine()
		col := e.StartPos.GetColumn()
		key := e.Message
		if _, dup := seen[key]; dup && len(errs) > 1 {
			continue
		}
		seen[key] = struct{}{}
		b.WriteString("\n  ")
		b.WriteString(e.Message)
		if line > 0 {
			b.WriteString("\n")
			b.WriteString(sourceExcerpt(lines, line, col))
		}
		if hint := syntaxFlowErrorHint(text, lines, line, col, e.Message); hint != "" {
			b.WriteString("\n  hint: ")
			b.WriteString(hint)
		}
	}
	return utils.Error(b.String())
}

// sourceExcerpt renders the offending line with a caret under the column.
func sourceExcerpt(lines []string, line, col int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	src := strings.TrimRight(lines[line-1], "\r")
	const gutterWidth = 5
	gutter := strings.Repeat(" ", gutterWidth-len(strconv.Itoa(line))) + strconv.Itoa(line) + " | "
	caretPad := strings.Repeat(" ", gutterWidth+3)
	if col > 0 {
		caretPad += strings.Repeat(" ", col)
	}
	return gutter + src + "\n" + caretPad + "^"
}

// conditionKeys are the words that may be followed by ':' inside ?{...}.
var conditionKeys = map[string]struct{}{
	"opcode": {}, "have": {}, "any": {}, "version_in": {},
}

func isConditionKey(word string) bool {
	_, ok := conditionKeys[strings.ToLower(word)]
	return ok
}

// colonPosition returns the index of the ':' at or just before col, or -1.
// ANTLR reports the column of the offending token, which is the ':' itself for
// a "mismatched input ':'" error.
func colonPosition(src string, col int) int {
	if col >= 0 && col < len(src) && src[col] == ':' {
		return col
	}
	if idx := col - 1; idx >= 0 && idx < len(src) && src[idx] == ':' {
		return idx
	}
	return -1
}

// wordBefore returns the identifier that ends right before the colon at col.
func wordBefore(src string, col int) string {
	end := col
	if idx := colonPosition(src, col); idx >= 0 {
		end = idx
	}
	start := end
	for start > 0 {
		c := src[start-1]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	return src[start:end]
}

// syntaxFlowErrorHint explains the mistake the parser ran into. It is read-only
// over the rule text and the ANTLR message.
func syntaxFlowErrorHint(text string, lines []string, line, col int, msg string) string {
	if strings.Contains(msg, "token recognition error") {
		return "the rule contains a character that is not part of SyntaxFlow syntax"
	}
	src := ""
	if line >= 1 && line <= len(lines) {
		src = lines[line-1]
	}
	inCondition := strings.Contains(src, "?{") || strings.Contains(src, "?<")
	switch {
	case inCondition && colonPosition(src, col) >= 0:
		if key := wordBefore(src, col); key != "" && !isConditionKey(key) {
			return "unknown condition key '" + key + ":'; the keys are opcode:, have:, any:, version_in:"
		}
	case inCondition && strings.Contains(src, "-") && col > 0 && col <= len(src) && src[col-1] == '-':
		return "a name inside ?{...} cannot contain '-' unless it is written as a glob: use *Function-strcmp*"
	case inCondition && strings.Contains(msg, "no viable alternative"):
		return "expected a condition: a name or glob (*name*), 'string', /regexp/, opcode: <name>, have: 'string', not/!/&&/||"
	case strings.Contains(msg, "no viable alternative"):
		return "expected a filter expression: a variable ($x), a name, a glob (*name*), or /regexp/"
	case strings.Contains(msg, "mismatched input") || strings.Contains(msg, "extraneous input"):
		return "this token does not belong here; check the surrounding brackets and separators"
	case strings.Contains(msg, "missing "):
		return "a token is missing at the caret position"
	}
	return ""
}
