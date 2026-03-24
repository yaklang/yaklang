package codec

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"
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
	if len(i) == 0 {
		return i
	}

	var buf bytes.Buffer
	for index := 0; index < len(i); {
		if decoded, size, ok := decodeJSONUnicodeAt(i, index); ok {
			buf.WriteString(decoded)
			index += size
			continue
		}
		buf.WriteByte(i[index])
		index++
	}
	return buf.String()
}

func decodeJSONUnicodeAt(s string, index int) (decoded string, size int, ok bool) {
	if index >= len(s) || s[index] != '\\' {
		return "", 0, false
	}

	if index+2 < len(s) && s[index+1] == '\\' {
		switch s[index+2] {
		case 'u':
			if index+7 <= len(s) && parseJSONUnicodeRune(s[index+3:index+7]) {
				return s[index+1 : index+7], 7, true
			}
		case 'U':
			if index+11 <= len(s) && parseJSONUnicodeRune(s[index+3:index+11]) {
				return s[index+1 : index+11], 11, true
			}
		}
	}

	if index+1 >= len(s) {
		return "", 0, false
	}

	switch s[index+1] {
	case 'u':
		if index+6 > len(s) {
			return "", 0, false
		}
		r, ok := decodeJSONUnicodeRune(s[index+2 : index+6])
		if !ok {
			return "", 0, false
		}
		return string(r), 6, true
	case 'U':
		if index+10 > len(s) {
			return "", 0, false
		}
		r, ok := decodeJSONUnicodeRune(s[index+2 : index+10])
		if !ok {
			return "", 0, false
		}
		return string(r), 10, true
	default:
		return "", 0, false
	}
}

func parseJSONUnicodeRune(hex string) bool {
	_, ok := decodeJSONUnicodeRune(hex)
	return ok
}

func decodeJSONUnicodeRune(hex string) (rune, bool) {
	n, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return 0, false
	}
	r := rune(n)
	if !utf8.ValidRune(r) {
		return 0, false
	}
	return r, true
}
