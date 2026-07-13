package preprocess

import (
	"strings"
	"unicode/utf8"
)

type macroScanState struct {
	inBlockComment bool
}

// joinLogicalLines merges physical lines connected by trailing backslash continuations.
func joinLogicalLines(src string) []string {
	src = normalizeLineEndings(src)
	var lines []string
	var b strings.Builder
	physical := strings.Split(src, "\n")
	for i, line := range physical {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, `\`) {
			b.WriteString(trimmed[:len(trimmed)-1])
			continue
		}
		b.WriteString(line)
		lines = append(lines, b.String())
		b.Reset()
		if i == len(physical)-1 && strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) {
			// dangling continuation at EOF
		}
	}
	if b.Len() > 0 {
		lines = append(lines, b.String())
	}
	return lines
}

// tokenizeMacroSource lexes src into preprocessor tokens, skipping comments.
func tokenizeMacroSource(src string) []macroToken {
	var st macroScanState
	return tokenizeMacroSourceWithState(src, &st)
}

func tokenizeMacroSourceWithState(src string, st *macroScanState) []macroToken {
	var out []macroToken
	i := 0
	for i < len(src) {
		if st.inBlockComment {
			j := strings.Index(src[i:], "*/")
			if j < 0 {
				out = append(out, macroToken{macroTokComment, src[i:]})
				return out
			}
			out = append(out, macroToken{macroTokComment, src[i : i+j+2]})
			i += j + 2
			st.inBlockComment = false
			continue
		}
		if i+1 < len(src) && src[i:i+2] == "/*" {
			start := i
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				st.inBlockComment = true
				out = append(out, macroToken{macroTokComment, src[start:]})
				return out
			}
			out = append(out, macroToken{macroTokComment, src[start : i+2+j+2]})
			i = start + 2 + j + 2
			continue
		}
		if i+1 < len(src) && src[i:i+2] == "//" {
			start := i
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				out = append(out, macroToken{macroTokComment, src[start:]})
				return out
			}
			out = append(out, macroToken{macroTokComment, src[start : i+j]})
			i += j
			continue
		}

		if next := skipBackslashNewline(src, i); next != i {
			i = next
			continue
		}

		c := src[i]
		switch c {
		case '"':
			start := i
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			out = append(out, macroToken{macroTokString, src[start:i]})
			continue
		case '\'':
			start := i
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if src[i] == '\'' {
					i++
					break
				}
				i++
			}
			out = append(out, macroToken{macroTokChar, src[start:i]})
			continue
		case '\r':
			i++
			if i < len(src) && src[i] == '\n' {
				i++
			}
			out = append(out, macroToken{macroTokNewline, "\n"})
			continue
		case '\n':
			i++
			out = append(out, macroToken{macroTokNewline, "\n"})
			continue
		case ' ', '\t', '\f', '\v':
			start := i
			for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\f' || src[i] == '\v') {
				i++
			}
			out = append(out, macroToken{macroTokWhitespace, src[start:i]})
			continue
		}

		r, size := utf8.DecodeRuneInString(src[i:])
		if isMacroIdentStart(r) {
			start := i
			i += size
			for i < len(src) {
				r2, sz := utf8.DecodeRuneInString(src[i:])
				if !isMacroIdentPart(r2) {
					break
				}
				i += sz
			}
			out = append(out, macroToken{macroTokIdent, src[start:i]})
			continue
		}
		if c >= '0' && c <= '9' {
			start := i
			i++
			for i < len(src) {
				ch := src[i]
				if (ch >= '0' && ch <= '9') || ch == '.' || ch == 'x' || ch == 'X' ||
					(ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') ||
					ch == 'u' || ch == 'U' || ch == 'l' || ch == 'L' {
					i++
					continue
				}
				break
			}
			out = append(out, macroToken{macroTokNumber, src[start:i]})
			continue
		}
		if p, n := matchMultiCharPunct(src, i); n > 0 {
			out = append(out, macroToken{macroTokPunct, p})
			i += n
			continue
		}
		out = append(out, macroToken{macroTokPunct, src[i : i+1]})
		i++
	}
	return out
}

func tokensToString(tokens []macroToken) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(t.text)
	}
	return b.String()
}

func skipWhitespaceTokens(tokens []macroToken, i int) int {
	for i < len(tokens) && tokens[i].kind == macroTokWhitespace {
		i++
	}
	return i
}
