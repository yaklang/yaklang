package preprocess

import (
	"strings"
	"unicode"
)

// headerStmtAcc buffers a header declaration until it is complete so we can
// keep typedef/struct/union/enum and drop library function prototypes/bodies.
type headerStmtAcc struct {
	lines []string
	brace int
}

func (a *headerStmtAcc) feed(line string) (done bool, lines []string, keep bool) {
	if a.empty() && isSkippableHeaderLine(line) {
		return false, nil, false
	}
	a.lines = append(a.lines, line)
	a.brace += netBraces(line)
	if a.brace < 0 {
		a.brace = 0
	}
	if a.brace > 0 {
		return false, nil, false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false, nil, false
	}
	if !strings.Contains(trimmed, ";") && !strings.HasSuffix(trimmed, "}") && !strings.HasSuffix(trimmed, "};") {
		return false, nil, false
	}
	return a.flush()
}

func (a *headerStmtAcc) empty() bool {
	return len(a.lines) == 0
}

func (a *headerStmtAcc) flush() (done bool, lines []string, keep bool) {
	if a.empty() {
		return false, nil, false
	}
	stmt := strings.Join(a.lines, "\n")
	lines = a.lines
	a.lines = nil
	a.brace = 0
	return true, lines, isHeaderTypeStmt(stmt)
}

func isSkippableHeaderLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "//") {
		return true
	}
	if strings.HasPrefix(s, "/*") && strings.HasSuffix(s, "*/") && !strings.Contains(s[2:len(s)-2], "*/") {
		return true
	}
	return false
}

func isHeaderTypeStmt(stmt string) bool {
	s := skipLeadingExt(strings.TrimSpace(stmt))
	if s == "" {
		return false
	}
	tok, rest := splitFirstIdent(s)
	switch tok {
	case "typedef":
		return true
	case "struct", "union", "enum":
		rest = skipLeadingExt(strings.TrimSpace(rest))
		if id, rest2 := splitFirstIdent(rest); id != "" {
			rest = strings.TrimSpace(rest2)
		}
		if rest == "" {
			return false
		}
		return rest[0] == '{' || rest[0] == ';'
	default:
		return false
	}
}

func splitFirstIdent(s string) (ident, rest string) {
	s = strings.TrimSpace(s)
	if s == "" || !isIdentStart(rune(s[0])) {
		return "", s
	}
	j := 1
	for j < len(s) && isIdentPart(rune(s[j])) {
		j++
	}
	return s[:j], s[j:]
}

func skipLeadingExt(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case strings.HasPrefix(s, "__extension__"):
			s = strings.TrimSpace(s[len("__extension__"):])
		case strings.HasPrefix(s, "__attribute__"):
			s = skipBalanced(strings.TrimSpace(s[len("__attribute__"):]), '(', ')')
		case strings.HasPrefix(s, "__declspec"):
			s = skipBalanced(strings.TrimSpace(s[len("__declspec"):]), '(', ')')
		default:
			return s
		}
	}
}

func skipBalanced(s string, open, close byte) string {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != open {
		return s
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[i+1:])
			}
		}
	}
	return ""
}

func netBraces(line string) int {
	n := 0
	inStr, inChr, esc := false, false, false
	for _, r := range line {
		if inStr {
			if esc {
				esc = false
				continue
			}
			if r == '\\' {
				esc = true
				continue
			}
			if r == '"' {
				inStr = false
			}
			continue
		}
		if inChr {
			if esc {
				esc = false
				continue
			}
			if r == '\\' {
				esc = true
				continue
			}
			if r == '\'' {
				inChr = false
			}
			continue
		}
		switch r {
		case '"':
			inStr = true
		case '\'':
			inChr = true
		case '{':
			n++
		case '}':
			n--
		}
	}
	return n
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
