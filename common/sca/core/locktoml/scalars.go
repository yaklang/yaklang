// Scalar helpers extracted from BurntSushi/toml v1.3.2; MIT, see LICENSE.
package locktoml

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func numHasLeadingZero(s string) bool {
	if len(s) > 1 && s[0] == '0' && !(s[1] == 'b' || s[1] == 'o' || s[1] == 'x') { // Allow 0b, 0o, 0x
		return true
	}
	if len(s) > 2 && (s[0] == '-' || s[0] == '+') && s[1] == '0' {
		return true
	}
	return false
}

// numUnderscoresOK checks whether each underscore in s is surrounded by
// characters that are not underscores.
func numUnderscoresOK(s string) bool {
	switch s {
	case "nan", "+nan", "-nan", "inf", "-inf", "+inf":
		return true
	}
	accept := false
	for _, r := range s {
		if r == '_' {
			if !accept {
				return false
			}
		}

		// isHexadecimal is a superset of all the permissable characters
		// surrounding an underscore.
		accept = isHexadecimal(r)
	}
	return accept
}

// numPeriodsOK checks whether every period in s is followed by a digit.
func numPeriodsOK(s string) bool {
	period := false
	for _, r := range s {
		if period && !isDigit(r) {
			return false
		}
		period = r == '.'
	}
	return !period
}

func stripFirstNewline(s string) string {
	if len(s) > 0 && s[0] == '\n' {
		return s[1:]
	}
	if len(s) > 1 && s[0] == '\r' && s[1] == '\n' {
		return s[2:]
	}
	return s
}

// stripEscapedNewlines removes whitespace after line-ending backslashes in
// multiline strings.
//
// A line-ending backslash is an unescaped \ followed only by whitespace until
// the next newline. After a line-ending backslash, all whitespace is removed
// until the next non-whitespace character.
func (p *parser) stripEscapedNewlines(s string) string {
	var b strings.Builder
	var i int
	for {
		ix := strings.Index(s[i:], `\`)
		if ix < 0 {
			b.WriteString(s)
			return b.String()
		}
		i += ix

		if len(s) > i+1 && s[i+1] == '\\' {
			// Escaped backslash.
			i += 2
			continue
		}
		// Scan until the next non-whitespace.
		j := i + 1
	whitespaceLoop:
		for ; j < len(s); j++ {
			switch s[j] {
			case ' ', '\t', '\r', '\n':
			default:
				break whitespaceLoop
			}
		}
		if j == i+1 {
			// Not a whitespace escape.
			i++
			continue
		}
		if !strings.Contains(s[i:j], "\n") {
			// This is not a line-ending backslash.
			// (It's a bad escape sequence, but we can let
			// replaceEscapes catch it.)
			i++
			continue
		}
		b.WriteString(s[:i])
		s = s[j:]
		i = 0
	}
}

func (p *parser) replaceEscapes(it item, str string) string {
	replaced := make([]rune, 0, len(str))
	s := []byte(str)
	r := 0
	for r < len(s) {
		if s[r] != '\\' {
			c, size := utf8.DecodeRune(s[r:])
			r += size
			replaced = append(replaced, c)
			continue
		}
		r += 1
		if r >= len(s) {
			p.bug("Escape sequence at end of string.")
			return ""
		}
		switch s[r] {
		default:
			p.bug("Expected valid escape code after \\, but got %q.", s[r])
		case ' ', '\t':
			p.panicItemf(it, "invalid escape: '\\%c'", s[r])
		case 'b':
			replaced = append(replaced, rune(0x0008))
			r += 1
		case 't':
			replaced = append(replaced, rune(0x0009))
			r += 1
		case 'n':
			replaced = append(replaced, rune(0x000A))
			r += 1
		case 'f':
			replaced = append(replaced, rune(0x000C))
			r += 1
		case 'r':
			replaced = append(replaced, rune(0x000D))
			r += 1
		case 'e':

		case '"':
			replaced = append(replaced, rune(0x0022))
			r += 1
		case '\\':
			replaced = append(replaced, rune(0x005C))
			r += 1
		case 'x':

		case 'u':
			// At this point, we know we have a Unicode escape of the form
			// `uXXXX` at [r, r+5). (Because the lexer guarantees this
			// for us.)
			escaped := p.asciiEscapeToUnicode(it, s[r+1:r+5])
			replaced = append(replaced, escaped)
			r += 5
		case 'U':
			// At this point, we know we have a Unicode escape of the form
			// `uXXXX` at [r, r+9). (Because the lexer guarantees this
			// for us.)
			escaped := p.asciiEscapeToUnicode(it, s[r+1:r+9])
			replaced = append(replaced, escaped)
			r += 9
		}
	}
	return string(replaced)
}

func (p *parser) asciiEscapeToUnicode(it item, bs []byte) rune {
	s := string(bs)
	hex, err := strconv.ParseUint(strings.ToLower(s), 16, 32)
	if err != nil {
		p.bug("Could not parse '%s' as a hexadecimal number, but the lexer claims it's OK: %s", s, err)
	}
	if !utf8.ValidRune(rune(hex)) {
		p.panicItemf(it, "Escaped character '\\u%s' is not valid UTF-8.", s)
	}
	return rune(hex)
}
