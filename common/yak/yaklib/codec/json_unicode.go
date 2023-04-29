package codec

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

func JsonUnicodeEncode(i string) string {
	s := []rune(i)
	var buf bytes.Buffer
	for _, i := range s {
		code := fmt.Sprintf("\\u%04x", i)
		buf.WriteString(code)
	}
	return buf.String()
}

func JsonUnicodeDecode(i string) string {
	s := i
	var (
		jsonUnicodeDecodeRe = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)
	)
	return jsonUnicodeDecodeRe.ReplaceAllStringFunc(s, func(found string) (ret string) {
		defer func() {
			if err := recover(); err != nil {
				ret = found
			}
		}()
		numStr := found[2:]
		n, err := strconv.ParseInt(numStr, 16, 32)
		if err != nil {
			return found
		}
		return string([]rune{rune(n)})
	})
}
