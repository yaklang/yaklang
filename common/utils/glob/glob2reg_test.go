package glob_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func TestGlob2Reg(t *testing.T) {
	for globStr, wantReg := range map[string]string{
		"1":            "^1$",
		"1*":           "^1.*$",
		"*1*":          "^.*1.*$",
		"foo/bar":      "^foo\\/bar$",
		"foo/**/bar":   "^foo\\/.*\\/bar$",
		"foo?bar":      "^foo.bar$",
		"foo[abc]bar":  "^foo[abc]bar$",
		"foo[a-c]bar":  "^foo[a-c]bar$",
		"foo{bar,baz}": "^foo(bar|baz)$",
	} {
		gotReg := glob.Glob2Regex(globStr)
		require.Equal(t, wantReg, gotReg)
	}
}
