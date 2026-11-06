package pcre

import "strings"

// normalizeLegacyCharacterClassRanges keeps legacy yaklang rules working when
// they use a class escape as a range endpoint, for example "[\d\w-_]". PCRE1
// and Go's regexp accepted that spelling; PCRE2 rejects the ambiguous range.
// Treat the hyphen as a literal in that case, which is what the rule intends.
func normalizeLegacyCharacterClassRanges(expression string) string {
	if !strings.Contains(expression, "[") || !strings.Contains(expression, "-") {
		return expression
	}
	var builder strings.Builder
	builder.Grow(len(expression))
	inClass := false
	classFirst := false
	previousClassEscape := false
	for index := 0; index < len(expression); {
		current := expression[index]
		if !inClass {
			builder.WriteByte(current)
			if current == '\\' && index+1 < len(expression) {
				builder.WriteByte(expression[index+1])
				index += 2
				continue
			}
			if current == '[' {
				inClass = true
				classFirst = true
			}
			index++
			continue
		}
		if current == ']' && !classFirst {
			builder.WriteByte(current)
			inClass = false
			previousClassEscape = false
			index++
			continue
		}
		if current == '^' && classFirst {
			builder.WriteByte(current)
			index++
			continue
		}
		if current == '-' && !classFirst {
			_, nextClassEscape, nextCloses := peekCharacterClassAtom(expression, index+1)
			if nextCloses || (!previousClassEscape && !nextClassEscape) {
				builder.WriteByte(current)
			} else {
				builder.WriteString(`\-`)
			}
			previousClassEscape = false
			classFirst = false
			index++
			continue
		}
		if current == '\\' {
			end, classEscape := scanCharacterClassEscape(expression, index)
			builder.WriteString(expression[index:end])
			previousClassEscape = classEscape
			classFirst = false
			index = end
			continue
		}
		builder.WriteByte(current)
		previousClassEscape = false
		classFirst = false
		index++
	}
	return builder.String()
}

func peekCharacterClassAtom(expression string, index int) (end int, classEscape, closesClass bool) {
	if index >= len(expression) {
		return index, false, true
	}
	if expression[index] == ']' {
		return index + 1, false, true
	}
	if expression[index] == '\\' {
		end, classEscape = scanCharacterClassEscape(expression, index)
		return end, classEscape, false
	}
	return index + 1, false, false
}

func scanCharacterClassEscape(expression string, index int) (end int, classEscape bool) {
	if index+1 >= len(expression) {
		return len(expression), false
	}
	escaped := expression[index+1]
	end = index + 2
	switch escaped {
	case 'p', 'P':
		if end < len(expression) && expression[end] == '{' {
			for end < len(expression) && expression[end] != '}' {
				end++
			}
			if end < len(expression) {
				end++
			}
		}
		return end, true
	case 'd', 'D', 'w', 'W', 's', 'S', 'h', 'H', 'v', 'V',
		'N', 'R', 'X', 'C':
		return end, true
	default:
		return end, false
	}
}
