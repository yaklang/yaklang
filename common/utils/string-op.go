package utils

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// SplitAndTrim 将字符串s按照sep分割成字符串切片，并且去除每个字符串的前后空白字符
// Example:
// ```
// str.SplitAndTrim(" hello yak ", " ") // ["hello", "yak"]
// ```
func PrettifyListFromStringSplited(Raw string, sep string) (targets []string) {
	targetsRaw := strings.Split(Raw, sep)
	for _, tRaw := range targetsRaw {
		r := strings.TrimSpace(tRaw)
		if len(r) > 0 {
			targets = append(targets, r)
		}
	}
	return
}

func PrettifyShrinkJoin(sep string, s ...string) string {
	var buf bytes.Buffer
	count := 0
	existedHashMap := make(map[string]struct{})
	for _, element := range s {
		for _, i := range PrettifyListFromStringSplited(element, sep) {
			if i == "" {
				continue
			}

			_, ok := existedHashMap[i]
			if ok {
				continue
			}
			existedHashMap[i] = struct{}{}
			if count == 0 {
				buf.WriteString(i)
			} else {
				buf.WriteString(sep + i)
			}
			count++
		}
	}
	return buf.String()
}

func PrettifyJoin(sep string, s ...string) string {
	var buf bytes.Buffer
	count := 0
	for _, i := range s {
		if i == "" {
			continue
		}

		if count == 0 {
			buf.WriteString(i)
		} else {
			buf.WriteString(sep + i)
		}
		count++
	}
	return buf.String()
}

// PrettifyListFromStringSplitEx split string using given sep if no sep given sep = []string{",", "|"}
func PrettifyListFromStringSplitEx(Raw string, sep ...string) (targets []string) {
	if len(sep) <= 0 {
		sep = []string{",", "|"}
	}
	patternStr := ""
	for _, v := range sep {
		if len(v) > 0 {
			patternStr += regexp.QuoteMeta(string(v[0])) + "|"
		}
	}
	if len(patternStr) > 0 {
		patternStr = patternStr[:len(patternStr)-1]
	}

	var targetsRaw []string
	re, err := regexp.Compile(patternStr)
	if err != nil {
		log.Warn(err)
		return targetsRaw
	}
	targetsRaw = re.Split(Raw, -1)
	for _, tRaw := range targetsRaw {
		r := strings.TrimSpace(tRaw)
		if len(r) > 0 {
			targets = append(targets, r)
		}
	}
	return
}

func ToLowerAndStrip(s string) string {
	return StringLowerAndTrimSpace(s)
}

// StringSliceContains 判断字符串切片s中是否包含raw，对于非字符串的切片，会尝试将其元素转换为字符串再判断是否包含
// Example:
// ```
// str.StringSliceContains(["hello", "yak"], "yak") // true
// str.StringSliceContains([1, 2, 3], "4") // false
// ```
func StringSliceContain(s interface{}, raw string) (result bool) {
	defer func() {
		if err := recover(); err != nil {
			return
		}
	}()
	haveResult := false
	switch ret := s.(type) {
	case []string:
		for _, i := range ret {
			if i == raw {
				return true
			}
		}
		return false
	}
	funk.ForEach(s, func(i interface{}) {
		if haveResult {
			return
		}
		if InterfaceToString(i) == raw {
			haveResult = true
		}
	})
	return haveResult
}

// StringContainsAnyOfSubString 判断字符串s中是否包含subs中的任意一个子串
// Example:
// ```
// str.StringContainsAnyOfSubString("hello yak", ["yak", "world"]) // true
// ```
func StringContainsAnyOfSubString(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func StringContainsAllOfSubString(s string, subs []string) bool {
	if len(subs) <= 0 {
		return false
	}
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func IStringContainsAnyOfSubString(s string, subs []string) bool {
	for _, sub := range subs {
		if IContains(s, sub) {
			return true
		}
	}
	return false
}

func ConvertToStringSlice(raw ...interface{}) (r []string) {
	for _, e := range raw {
		r = append(r, fmt.Sprintf("%v", e))
	}
	return
}

func ChanStringToSlice(c chan string) (result []string) {
	for l := range c {
		result = append(result, l)
	}
	return
}

var cStyleCharPRegexp, _ = regexp.Compile(`\\((x[0-9abcdef]{2})|([0-9]{1,3}))`)

func ParseCStyleBinaryRawToBytes(raw []byte) []byte {
	// like "\\x12" => "\x12"
	return cStyleCharPRegexp.ReplaceAllFunc(raw, func(i []byte) []byte {
		if bytes.HasPrefix(i, []byte("\\x")) {
			if len(i) == 4 {
				rawChar := string(i[2:])
				charInt, err := strconv.ParseInt("0x"+string(rawChar), 0, 16)
				if err != nil {
					return i
				}
				return []byte{byte(charInt)}
			} else {
				return i
			}
		} else if bytes.HasPrefix(raw, []byte("\\")) {
			if len(i) > 1 && len(i) <= 4 {
				rawChar := string(i[1:])
				charInt, err := strconv.ParseInt(string(rawChar), 10, 8)
				if err != nil {
					return i
				}
				return []byte{byte(charInt)}
			} else {
				return i
			}
		}
		return i
	})
}

var (
	GbkToUtf8 = codec.GbkToUtf8
	Utf8ToGbk = codec.Utf8ToGbk
)

func ParseStringToVisible(raw interface{}) string {
	s := InterfaceToString(raw)
	s = strings.TrimSpace(s)
	s = EscapeInvalidUTF8Byte([]byte(s))
	// s = strings.ReplaceAll(s, "\x20", "\\x20")
	s = strings.ReplaceAll(s, "\x0b", "\\v")
	r, err := regexp.Compile(`\s`)
	if err != nil {
		return s
	}
	return r.ReplaceAllStringFunc(s, func(s string) string {
		result := strconv.Quote(s)
		for strings.HasPrefix(result, "\"") {
			result = result[1:]
		}
		for strings.HasSuffix(result, "\"") {
			result = result[:len(result)-1]
		}
		return result
	})
}

func EscapeInvalidUTF8Byte(s []byte) string {
	// 这个操作返回的结果和原始字符串是非等价的
	var builder strings.Builder
	builder.Grow(len(s) + 20)
	start := 0
	for {
		r, size := utf8.DecodeRune(s[start:])
		if r == utf8.RuneError {
			// 说明是空的
			if size == 0 {
				break
			} else {
				// 不是 rune
				builder.WriteString("\\x")
				builder.WriteString(hex.EncodeToString([]byte{s[start]}))
			}
		} else {
			// 不是换行之类的控制字符
			if unicode.IsControl(r) && !unicode.IsSpace(r) {
				builder.WriteString("\\x")
				builder.WriteString(hex.EncodeToString([]byte{byte(r)}))
			} else {
				// 正常字符
				builder.WriteRune(r)
			}
		}
		start += size
	}
	return builder.String()
}

var GBKSafeString = codec.GBKSafeString

func LastLine(s []byte) []byte {
	s = bytes.TrimSpace(s)
	scanner := bufio.NewScanner(bytes.NewReader(s))
	scanner.Split(bufio.ScanLines)

	lastLine := s
	for scanner.Scan() {
		lastLine = scanner.Bytes()
	}

	return lastLine
}

func RemoveUnprintableChars(raw string) string {
	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanBytes)

	buf := bytes.NewBufferString("")
	for scanner.Scan() {
		c := scanner.Bytes()[0]

		if c <= 0x7e && c >= 0x20 {
			buf.WriteByte(c)
		} else {
			buf.WriteString(`\x` + fmt.Sprintf("%02x", c))
		}
	}

	return buf.String()
}

func RemoveUnprintableCharsWithReplace(raw string, handle func(i byte) string) string {
	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanBytes)

	var r []byte
	for scanner.Scan() {
		c := scanner.Bytes()[0]

		if c <= 0x7e && c >= 0x20 {
			r = append(r, c)
		} else {
			r = append(r, []byte(handle(c))...)
		}
	}

	return string(r)
}

func RemoveUnprintableCharsWithReplaceItem(raw string) string {
	return RemoveUnprintableCharsWithReplace(raw, func(i byte) string {
		return fmt.Sprintf("__HEX_%v__", codec.EncodeToHex([]byte{i}))
	})
}

func RemoveRepeatedWithStringSlice(slice []string) []string {
	r := map[string]interface{}{}
	for _, s := range slice {
		r[s] = 1
	}

	var r2 []string
	for k := range r {
		r2 = append(r2, k)
	}
	return r2
}

var titleRegexp = regexp.MustCompile(`(?is)\<title\>(.*?)\</?title\>`)

func ExtractTitleFromHTMLTitle(s string, defaultValue string) string {
	var title string
	l := titleRegexp.FindString(s)
	if len(l) > 15 {
		title = EscapeInvalidUTF8Byte([]byte(l))[7 : len(l)-8]
	}
	titleRunes := []rune(title)
	if len(titleRunes) > 128 {
		title = string(titleRunes[0:128]) + "..."
	}

	if title == "" {
		return defaultValue
	}

	return title
}
