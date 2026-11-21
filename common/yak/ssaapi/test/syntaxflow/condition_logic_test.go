package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestConditionLogicOrOrder locks the symmetry of `||` inside an optional
// filter. The right-hand branch used to contribute nothing when it was a
// `have:` comparison, so `*x* || have:'y'` returned fewer values than
// `have:'y' || *x*`.
func TestConditionLogicOrOrder(t *testing.T) {
	code := `
https_url = "https://example.test"
strncmp_buf = "strncmp"
other = "zzz"
`
	globs := `*?{*strncmp*} as $m
print $m`
	_ = globs

	t.Run("glob first", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			* as $all
			$all?{*strncmp* || have: "https"} as $m
			print $m
		`, map[string][]string{
			"m": {`"https://example.test"`, `"strncmp"`},
		})
	})

	t.Run("have first", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			* as $all
			$all?{have: "https" || *strncmp*} as $m
			print $m
		`, map[string][]string{
			"m": {`"https://example.test"`, `"strncmp"`},
		})
	})
}