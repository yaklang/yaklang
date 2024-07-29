package yaklib

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var jsonExtractRe2 = regexp2.MustCompile(`(\{(?:(?>[^{}"'\/]+)|(?>"(?:(?>[^\\"]+)|\\.)*")|(?>'(?:(?>[^\\']+)|\\.)*')|(?>\/\/.*\n)|(?>\/\*.*?\*\/))*\})`, regexp2.ECMAScript|regexp2.Multiline)

func extractAllJson(raw string) []string {
	originRaw := raw
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

func TestSmoking(t *testing.T) {
	t.Run("compile", func(t *testing.T) {
		re, err := re2Compile(`((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))(%.+)?`)
		require.NotNil(t, re)
		require.NoError(t, err)
	})
	t.Run("find", func(t *testing.T) {
		require.Equal(t, "a", re2Find("abc", "a"))
	})
	t.Run("findAll", func(t *testing.T) {
		require.ElementsMatch(t, re2FindAll("abc", "a"), []string{"a"})
	})
	t.Run("findSubmatch", func(t *testing.T) {
		require.ElementsMatch(t, re2FindSubmatch("abc", "(a)"), []string{"a", "a"})
	})
	t.Run("findSubmatchAll", func(t *testing.T) {
		require.ElementsMatch(t, re2FindSubmatchAll("abc", "(a)"), [][]string{{"a", "a"}})
	})
	t.Run("replaceAll", func(t *testing.T) {
		require.Equal(t, re2ReplaceAll("abc", "a", "b"), "bbc")
	})
	t.Run("replaceAllFunc", func(t *testing.T) {
		require.Equal(t, re2ReplaceAllFunc("abc", "a", func(s string) string {
			return "b"
		}), "bbc")
	})
	t.Run("ExtractGroups", func(t *testing.T) {
		require.Equal(t, re2ExtractGroups("abc", "(a)(b)"), map[string]string{
			"__all__": "ab",
			"0":       "ab",
			"1":       "a",
			"2":       "b",
		})
	})

	t.Run("ExtractGroupsAll", func(t *testing.T) {
		require.Equal(t, re2ExtractGroupsAll("abc", "(a)(b)"), []map[string]string{
			{
				"__all__": "ab",
				"0":       "ab",
				"1":       "a",
				"2":       "b",
			},
		})
	})
}
