package preprocess

import "strings"

// skipBackslashNewline consumes a backslash line continuation at src[i].
func skipBackslashNewline(src string, i int) int {
	if i >= len(src) || src[i] != '\\' {
		return i
	}
	k := i + 1
	for k < len(src) && (src[k] == ' ' || src[k] == '\t') {
		k++
	}
	if k < len(src) && src[k] == '\r' {
		if k+1 < len(src) && src[k+1] == '\n' {
			return k + 2
		}
		return k + 1
	}
	if k < len(src) && src[k] == '\n' {
		return k + 1
	}
	return i
}

func collapsePreprocessorContinuations(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		next := skipBackslashNewline(src, i)
		if next != i {
			i = next
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}
