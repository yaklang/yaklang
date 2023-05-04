package yaklib

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

var jsonExtractRe2 = regexp2.MustCompile(`(\{(?:(?>[^{}"'\/]+)|(?>"(?:(?>[^\\"]+)|\\.)*")|(?>'(?:(?>[^\\']+)|\\.)*')|(?>\/\/.*\n)|(?>\/\*.*?\*\/))*\})`, regexp2.ECMAScript|regexp2.Multiline)

func extractAllJson(raw string) []string {
	var originRaw = raw
	var lastResultMd5 string

	_ = originRaw
	var results []string
	_ = results
	for i := 0; i < 10; i++ {
		if lastResultMd5 == codec.Md5(raw) {
			// 结果是否稳定
			break
		}

		match, err := jsonExtractRe2.FindStringMatch(raw)
		if err != nil {
			continue
		}
		if match == nil {
			continue
		}

		for _, group := range match.Groups() {
			spew.Dump(group)
		}

		lastResultMd5 = codec.Md5(raw)
		raw, err = jsonExtractRe2.Replace(raw, "null", 0, -1)
		if err != nil {
			log.Errorf("replace failed: %s", err)
			break
		}

		if raw == "" {
			break
		}
	}
	return nil
}

func TestRegexpES(t *testing.T) {
	extractAllJson(`
{
    "name": "John Doe",
    "email": "johndoe@example.com",
    "user": {
        "username": "johndoe",
        "password": "abc123"
    }
}`)
}
