package codec

import (
	"strconv"
	"strings"
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
	runes := []rune(verbose)
	if len(runes) > size {
		runes = append(runes[:half], append([]rune("..."), runes[len(runes)-half:]...)...)
		verbose = string(runes)
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
