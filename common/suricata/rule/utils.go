package rule

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func atoi(i string) int {
	parsed, _ := strconv.Atoi(i)
	return parsed
}

func atoistar(i string) *int {
	if i == "" {
		return nil
	}
	parsed, _ := strconv.Atoi(i)
	return &parsed
}
