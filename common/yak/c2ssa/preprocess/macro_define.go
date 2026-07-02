package preprocess

import (
	"fmt"
	"strings"
	"unicode"
)

const vaArgsName = "__VA_ARGS__"

type functionMacro struct {
	params   []string
	variadic bool
	body     string
}

type collectResult struct {
	tables macroTables
	output string
}

// scanMacroTablesFromSource collects function/object macros without modifying src.
func scanMacroTablesFromSource(src string) macroTables {
	tables := newMacroTables()
	logical := joinLogicalLines(src)
	for _, line := range logical {
		applyDirectiveToTables(line, tables, false)
	}
	return tables
}

func applyDirectiveToTables(line string, tables macroTables, stripDefines bool) (removedDefine bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "#") {
		return false
	}
	directive := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	if directive == "" {
		return false
	}
	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "define":
		if name, fm, ok := parseFunctionDefineSafe(directive); ok {
			tables.function[name] = fm
			return stripDefines
		}
		if name, body, ok := parseObjectDefineSafe(directive); ok {
			tables.object[name] = body
			return stripDefines
		}
	case "undef":
		if len(parts) >= 2 {
			name := parts[1]
			delete(tables.function, name)
			delete(tables.object, name)
		}
	}
	return false
}

// collectFunctionMacros parses #define / #undef in src, merges base tables, and strips collected defines.
func collectFunctionMacros(src string, base macroTables) collectResult {
	res := collectResult{tables: cloneMacroTables(base)}
	logical := joinLogicalLines(src)
	var outLines []string

	for _, line := range logical {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasPrefix(trimmed, "#") {
			outLines = append(outLines, line)
			continue
		}
		directive := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if directive == "" {
			outLines = append(outLines, line)
			continue
		}
		parts := strings.Fields(directive)
		if len(parts) == 0 {
			outLines = append(outLines, line)
			continue
		}
		switch parts[0] {
		case "define":
			if removed := applyDirectiveToTables(line, res.tables, true); removed {
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			if len(parts) >= 2 {
				name := parts[1]
				delete(res.tables.function, name)
				delete(res.tables.object, name)
			}
			outLines = append(outLines, line)
		default:
			outLines = append(outLines, line)
		}
	}
	res.output = strings.Join(outLines, "\n")
	return res
}

func parseFunctionDefineSafe(directive string) (name string, fm functionMacro, ok bool) {
	name, fm, ok, err := parseFunctionDefine(directive)
	if err != nil || !ok {
		return "", functionMacro{}, false
	}
	return name, fm, true
}

func parseObjectDefineSafe(directive string) (name, body string, ok bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(directive, "define"))
	if rest == "" {
		return "", "", false
	}
	i := 0
	for i < len(rest) && unicode.IsSpace(rune(rest[i])) {
		i++
	}
	start := i
	for i < len(rest) {
		r := rune(rest[i])
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			i++
			continue
		}
		break
	}
	if i == start {
		return "", "", false
	}
	name = rest[start:i]
	for i < len(rest) && unicode.IsSpace(rune(rest[i])) {
		i++
	}
	if i >= len(rest) {
		return "", "", false
	}
	if rest[i] == '(' && findMatchingParen(rest, i) < 0 {
		return "", "", false
	}
	body = strings.TrimSpace(rest[i:])
	if body == "" {
		return "", "", false
	}
	return name, body, true
}

func parseFunctionDefine(directive string) (name string, fm functionMacro, ok bool, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(directive, "define"))
	if rest == "" {
		return "", fm, false, nil
	}
	i := 0
	for i < len(rest) && unicode.IsSpace(rune(rest[i])) {
		i++
	}
	start := i
	for i < len(rest) {
		r := rune(rest[i])
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			i++
			continue
		}
		break
	}
	if i == start {
		return "", fm, false, nil
	}
	name = rest[start:i]
	for i < len(rest) && unicode.IsSpace(rune(rest[i])) {
		i++
	}
	if i >= len(rest) || rest[i] != '(' {
		return "", fm, false, nil
	}
	closeIdx := findMatchingParen(rest, i)
	if closeIdx < 0 {
		return "", fm, false, fmt.Errorf("unbalanced parentheses in #define %s", name)
	}
	paramText := rest[i+1 : closeIdx]
	params, variadic, err := parseMacroParams(paramText)
	if err != nil {
		return "", fm, false, err
	}
	for _, p := range params {
		if !isValidMacroParamName(p) {
			return "", fm, false, nil
		}
	}
	body := strings.TrimSpace(rest[closeIdx+1:])
	fm = functionMacro{params: params, variadic: variadic, body: body}
	return name, fm, true, nil
}

func findMatchingParen(s string, open int) int {
	if open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	for i := open; i < len(s); i++ {
		if i+1 < len(s) && s[i:i+2] == "/*" {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return -1
			}
			i += j + 3
			continue
		}
		if i+1 < len(s) && s[i:i+2] == "//" {
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				break
			}
			i += j
			continue
		}
		switch s[i] {
		case '"':
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == '"' {
					break
				}
				i++
			}
		case '\'':
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == '\'' {
					break
				}
				i++
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseMacroParams(paramText string) ([]string, bool, error) {
	paramText = strings.TrimSpace(paramText)
	if paramText == "" {
		return nil, false, nil
	}
	raw := splitMacroParamNames(paramText)
	var params []string
	variadic := false
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "..." {
			variadic = true
			continue
		}
		if strings.HasPrefix(p, "...") {
			return nil, false, fmt.Errorf("invalid variadic macro parameter: %q", p)
		}
		if strings.HasSuffix(p, "...") {
			variadic = true
			p = strings.TrimSpace(strings.TrimSuffix(p, "..."))
		}
		if p == "" {
			continue
		}
		params = append(params, p)
	}
	return params, variadic, nil
}

func isValidMacroParamName(p string) bool {
	if p == "" {
		return false
	}
	for i, r := range p {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func splitMacroParamNames(s string) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(':
			depth++
			cur.WriteByte(c)
		case ')':
			depth--
			cur.WriteByte(c)
		case ',':
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
