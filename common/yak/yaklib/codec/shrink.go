package codec

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func ShrinkStringDefault(r any) string {
	return ShrinkString(r, 64)
}

func ShrinkString(r any, size int) string {
	return shrinkStringWithMultiLine(r, size, false)
}

func ShrinkTextBlock(r any, size int) string {
	return shrinkStringWithMultiLine(r, size, true)
}

func shrinkStringWithMultiLine(r any, size int, multiline bool) string {
	if size <= 6 {
		size = 10
	}

	half := size / 2

	verbose := AnyToString(r)
	verbose = strings.TrimSpace(verbose)
	prefixEnd, runeCount := 0, 0
	for offset := 0; offset < len(verbose) && runeCount <= size; {
		_, width := utf8.DecodeRuneInString(verbose[offset:])
		offset += width
		runeCount++
		if runeCount == half {
			prefixEnd = offset
		}
	}
	if runeCount > size {
		suffixStart := len(verbose)
		for count := 0; count < half; count++ {
			_, width := utf8.DecodeLastRuneInString(verbose[:suffixStart])
			suffixStart -= width
		}

		var builder strings.Builder
		builder.Grow(prefixEnd + (len(verbose) - suffixStart) + len("..."))
		writeRuneSlice(&builder, verbose[:prefixEnd])
		builder.WriteString("...")
		writeRuneSlice(&builder, verbose[suffixStart:])
		verbose = builder.String()
	}
	if !multiline {
		verbose = strconv.Quote(verbose)
		verbose = verbose[1:]
		verbose = verbose[:len(verbose)-1]
		verbose = strings.ReplaceAll(verbose, `\r`, " ")
		verbose = strings.ReplaceAll(verbose, `\n`, " ")
		verbose = strings.ReplaceAll(verbose, `\t`, " ")
		verbose = strings.ReplaceAll(verbose, `\"`, "\"")
	}
	return verbose
}

// writeRuneSlice matches the conversion performed by string([]rune(value)),
// including replacing each invalid UTF-8 byte with utf8.RuneError.
func writeRuneSlice(builder *strings.Builder, value string) {
	if utf8.ValidString(value) {
		builder.WriteString(value)
		return
	}
	for len(value) > 0 {
		r, width := utf8.DecodeRuneInString(value)
		builder.WriteRune(r)
		value = value[width:]
	}
}
