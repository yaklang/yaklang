package rule

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

var sRe = regexp.MustCompile(`(?i)\|(?P<single>[0-9a-f][0-9a-f])( (?P<after>[0-9a-f][0-9a-f]))*\|`)

func unquoteAndParseHex(s string) string {
	rawStr, err := strconv.Unquote(s)
	if err != nil {
		return strings.Trim(strings.TrimSpace(s), `"`)
	}
	return sRe.ReplaceAllStringFunc(rawStr, func(origin string) string {
		origin = strings.Trim(origin, " |")
		origin = strings.ReplaceAll(origin, " ", "")
		var originBytes, _ = codec.DecodeHex(origin)
		return string(originBytes)
	})
}

var unquoteReplacer = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\;`, `;`)

func unquoteString(s string) string {
	if !(strings.HasSuffix(s, `"`) && strings.HasPrefix(s, `"`)) {
		return s
	}
	s = strings.Trim(s, `"`)
	var tmp string
	tmp = unquoteReplacer.Replace(s)
	for tmp != s {
		s = tmp
		tmp = unquoteReplacer.Replace(s)
	}
	return tmp
}

// unquotePCREString removes the rule-level quotes around a pcre value without
// collapsing regular-expression backslashes. Suricata passes the PCRE body to
// the regex engine mostly verbatim: ET rules rely on "\\x" matching a literal
// backslash followed by x and on "\\" matching a literal backslash. Repeatedly
// unescaping those pairs changes character classes and grouping, so only an
// escaped quote is translated here.
func unquotePCREString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	var builder strings.Builder
	builder.Grow(len(s))
	for index := 0; index < len(s); index++ {
		if s[index] != '\\' || index+1 >= len(s) {
			builder.WriteByte(s[index])
			continue
		}
		switch {
		case s[index+1] == '"':
			builder.WriteByte('"')
			index++
		default:
			builder.WriteByte(s[index])
		}
	}
	return builder.String()
}

func atoi(i string) int {
	parsed, _ := strconv.Atoi(i)
	return parsed
}

func parseOptionalInt(i string) (*int, error) {
	if i == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(i)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalIntOrVar(i string) (*int, string, error) {
	i = strings.TrimSpace(i)
	if i == "" {
		return nil, "", nil
	}
	parsed, err := strconv.Atoi(i)
	if err == nil {
		return &parsed, "", nil
	}
	if isVariableName(i) {
		return nil, i, nil
	}
	return nil, "", err
}

func isVariableName(value string) bool {
	if value == "" {
		return false
	}
	for index, ch := range value {
		if index == 0 {
			if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
				return false
			}
			continue
		}
		if !(ch == '_' || ch == '.' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

func getHighPort() uint16 {
	return uint16(55000 + rand.Intn(65535-55000))
}
