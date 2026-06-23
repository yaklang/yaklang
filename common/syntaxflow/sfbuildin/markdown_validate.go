package sfbuildin

import (
	"fmt"
	"strings"
)

var (
	corruptedPathSeparatorLiterals = []string{
		`contains("\"")`,
		`startsWith("\"")`,
		`endsWith("\"")`,
	}
)

// validateMarkdownText checks description/solution markdown fields for common formatting issues.
func validateMarkdownText(field string) []string {
	if strings.TrimSpace(field) == "" {
		return nil
	}
	var issues []string
	if containsLiteralNewlineOutsideCodeFences(field) {
		issues = append(issues, "contains literal \\n instead of real newlines")
	}
	if msg := validateCodeFencesAtLineStart(field); msg != "" {
		issues = append(issues, msg)
	}
	for _, msg := range validateCodeFenceContent(field) {
		issues = append(issues, msg)
	}
	return issues
}

func containsLiteralNewlineOutsideCodeFences(content string) bool {
	outside := contentOutsideCodeFences(content)
	for i := 0; i+1 < len(outside); i++ {
		if outside[i] != '\\' || outside[i+1] != 'n' {
			continue
		}
		// Allow CRLF escape notation (\r\n) in prose.
		if i >= 2 && outside[i-2] == '\\' && outside[i-1] == 'r' {
			continue
		}
		return true
	}
	return false
}

func contentOutsideCodeFences(content string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			b.WriteString(stripQuotedStrings(stripInlineCode(line)))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func stripInlineCode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '`' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && s[j] != '`' {
			j++
		}
		if j < len(s) {
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func stripQuotedStrings(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch s[i] {
		case '"', '\'':
			quote := s[i]
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == quote {
					i++
					break
				}
				i++
			}
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

func validateCodeFencesAtLineStart(content string) string {
	for i, line := range strings.Split(content, "\n") {
		for idx := 0; idx < len(line); {
			pos := strings.Index(line[idx:], "```")
			if pos < 0 {
				break
			}
			absPos := idx + pos
			before := line[:absPos]
			if strings.TrimSpace(before) != "" {
				return fmt.Sprintf("line %d: code fence ``` must be on its own line (optionally with a language tag)", i+1)
			}
			idx = absPos + 3
		}
	}
	return ""
}

func validateCodeFenceContent(content string) []string {
	var issues []string
	inFence := false
	inString := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				inFence = true
				inString = false
			} else {
				inFence = false
				inString = false
			}
			continue
		}
		if !inFence {
			continue
		}
		if hasCorruptedPathSeparatorLiteral(line) {
			issues = append(issues, fmt.Sprintf("line %d: corrupted path separator string literal in .contains/.startsWith (use \\\\ instead of \\\")", i+1))
		}
		if lineHasCorruptedEscapedQuotes(line, &inString) {
			issues = append(issues, fmt.Sprintf("line %d: JSON-style escaped quote (\\\") in code block; use normal \" quotes", i+1))
		}
	}
	return issues
}

func hasCorruptedPathSeparatorLiteral(line string) bool {
	for _, bad := range corruptedPathSeparatorLiterals {
		if strings.Contains(line, bad) {
			return true
		}
	}
	return false
}

// lineHasCorruptedEscapedQuotes detects heredoc corruption where source string
// delimiters were JSON-escaped (e.g. getParameter(\"name\") instead of ("name")).
// Escaped quotes inside normal "..." strings (HTML attributes, JSON literals) are ignored.
// inString carries double-quoted string state across lines within a code fence.
func lineHasCorruptedEscapedQuotes(line string, inString *bool) bool {
	corrupted := false
	for i := 0; i < len(line); i++ {
		if *inString {
			if line[i] == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if line[i] == '"' {
				*inString = false
			}
			continue
		}
		if line[i] == '"' {
			*inString = true
			continue
		}
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '"' {
			if i+2 < len(line) && isSourceStringStartChar(line[i+2]) {
				corrupted = true
			}
		}
	}
	return corrupted
}

func isSourceStringStartChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '/' || c == ':' || c == '`' || c == '$' || c == '@'
}
