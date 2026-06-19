package codec

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// JsonUnicodeEncode 将字符串的每个字符编码为 \uXXXX 形式的 Unicode 转义序列
// 参数:
//   - i: 待编码的字符串
//
// 返回值:
//   - \uXXXX 形式的 Unicode 转义字符串
//
// Example:
// ```
// // VARS: 把字符串编码为 \uXXXX
// result = codec.UnicodeEncode("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: \u0061\u0062\u0063
// // assert: 锁定结论(与 UnicodeDecode 往返一致)
// assert codec.UnicodeDecode(result) == "abc", "unicode encode/decode should round-trip"
// ```
func JsonUnicodeEncode(i string) string {
	s := []rune(i)
	var buf bytes.Buffer
	for _, i := range s {
		code := fmt.Sprintf("\\u%04x", i)
		buf.WriteString(code)
	}
	return buf.String()
}

// JsonUnicodeDecode 将 \uXXXX / \UXXXXXXXX 形式的 Unicode 转义序列解码为原始字符串
// 参数:
//   - i: 含 Unicode 转义序列的字符串
//
// 返回值:
//   - 解码后的原始字符串
//
// Example:
// ```
// // VARS: 解码 \uXXXX 转义序列
// result = codec.UnicodeDecode("\\u0061\\u0062\\u0063")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: abc
// // assert: 锁定结论
// assert result == "abc", "UnicodeDecode should decode escape sequences"
// ```
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
