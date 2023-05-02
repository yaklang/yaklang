package suricata

import (
	"regexp"
	"strconv"
	"strings"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var sRe = regexp.MustCompile(`(?i)\|(?P<single>[0-9a-f][0-9a-f])( (?P<after>[0-9a-f][0-9a-f]))*\|`)

func UnquoteString(s string) string {
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
