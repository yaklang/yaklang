// Package cliteral decodes C-family literal tokens.
//
// Both the C frontend (c2ssa) and the C++ frontend (cpp2ssa) accept the same
// integer, floating, character and string literal shapes. C++14 adds digit
// separators and user-defined suffixes, so the decoders here accept the union
// while keeping the C behaviour for every input C accepted before.
package cliteral

import (
	"strconv"
	"strings"
)

// IsInt reports whether text looks like an integer literal, with or without a
// C/C++ integer suffix.
func IsInt(text string) bool {
	body := trimIntSuffix(text)
	if body == "" {
		return false
	}
	if hasPrefixFold(body, "0x") {
		return isDigits(body[2:], 16)
	}
	if hasPrefixFold(body, "0b") {
		return isDigits(body[2:], 2)
	}
	return isDigits(body, 10) || isDigits(body, 8)
}

// ParseInt decodes an integer literal. Digit separators (') and the u/U/l/L
// suffix run are accepted; the base is chosen by prefix as in C.
func ParseInt(text string) (int64, error) {
	body := trimIntSuffix(strings.ReplaceAll(text, "'", ""))
	if hasPrefixFold(body, "0x") {
		return strconv.ParseInt(body[2:], 16, 64)
	}
	if hasPrefixFold(body, "0b") {
		return strconv.ParseInt(body[2:], 2, 64)
	}
	if strings.HasPrefix(body, "0") && len(body) > 1 {
		return strconv.ParseInt(body[1:], 8, 64)
	}
	return strconv.ParseInt(body, 10, 64)
}

// IsFloat reports whether text looks like a floating-point literal.
func IsFloat(text string) bool {
	return strings.Contains(text, ".") || strings.ContainsAny(text, "eE")
}

// ParseFloat decodes a floating-point literal, accepting the f/F/l/L suffix and
// digit separators.
func ParseFloat(text string) (float64, error) {
	body := strings.ReplaceAll(text, "'", "")
	for len(body) > 0 {
		last := body[len(body)-1]
		if last == 'f' || last == 'F' || last == 'l' || last == 'L' {
			body = body[:len(body)-1]
			continue
		}
		break
	}
	return strconv.ParseFloat(body, 64)
}

// ParseChar decodes a character literal and returns its code point. Encoding
// prefixes (u, U, L) are skipped, and the common escapes are resolved.
func ParseChar(text string) int32 {
	body := trimLiteralPrefix(strings.TrimSpace(text))
	if len(body) < 2 || body[0] != '\'' {
		return 0
	}
	end := strings.LastIndexByte(body, '\'')
	if end <= 0 {
		return 0
	}
	inner := body[1:end]
	runes := []rune(inner)
	if len(runes) == 0 {
		return 0
	}
	if runes[0] != '\\' {
		return runes[0]
	}
	if len(runes) == 1 {
		return '\\'
	}
	switch runes[1] {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'v':
		return '\v'
	case '0':
		return 0
	case '\\':
		return '\\'
	case '\'':
		return '\''
	case '"':
		return '"'
	case 'x':
		if value, err := strconv.ParseInt(string(runes[2:]), 16, 32); err == nil {
			return int32(value)
		}
	}
	return 0
}

// ParseString decodes a string literal body: surrounding quotes of every
// encoding prefix are removed and the common escapes are resolved. ok is false
// when text is not a quoted string, so callers can keep the raw macro token.
func ParseString(text string) (string, bool) {
	body := trimLiteralPrefix(strings.TrimSpace(text))
	// Raw string literals: R"delim(...)delim" keeps the payload between the
	// parentheses; the delimiter itself is not part of the value.
	if strings.HasPrefix(body, "R\"") {
		if start := strings.Index(body, "("); start >= 0 {
			if end := strings.LastIndex(body, ")"); end > start {
				return body[start+1 : end], true
			}
		}
	}
	if len(body) < 2 || body[0] != '"' {
		return "", false
	}
	unquoted, err := strconv.Unquote(body)
	if err == nil {
		return unquoted, true
	}
	return body[1 : len(body)-1], true
}

func trimIntSuffix(text string) string {
	i := len(text)
	for i > 0 {
		switch text[i-1] {
		case 'u', 'U', 'l', 'L':
			i--
		default:
			return text[:i]
		}
	}
	return text[:i]
}

func isDigits(text string, base int) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r == '\'' {
			continue
		}
		if digitValue(r) < 0 || digitValue(r) >= base {
			return false
		}
	}
	return true
}

func digitValue(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return -1
}

func hasPrefixFold(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

// trimLiteralPrefix removes the C++ literal encoding prefix: L, u, U, u8.
func trimLiteralPrefix(text string) string {
	for len(text) > 1 {
		switch text[0] {
		case 'L', 'U':
			text = text[1:]
			continue
		case 'u':
			text = text[1:]
			if len(text) > 1 && text[0] == '8' {
				text = text[1:]
			}
			continue
		}
		break
	}
	return text
}
