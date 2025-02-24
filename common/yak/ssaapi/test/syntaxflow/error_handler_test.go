package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSkipExpr(t *testing.T) {
	t.Run("simple positive", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t,
			`f(1)`,
			`f( * as $i)`,
			map[string][]string{
				"i": {"1"},
			},
		)
	})

	t.Run("simple, negative, just skip ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f(1)
		`, `
		bbbb( * as $i )
		f( * as $b)
		`, map[string][]string{
			"b": {"1"},
		},
		)
	})

	t.Run("simple, not found got empty", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f(1)
		`, `
		f ( * as $para_f )
		b( * as $para_b ) // this value not exist 
		$para_f + $para_b as $para1
		$para_b + $para_f as $para2
		`, map[string][]string{
			"para1": {"1"},
			"para2": {"1"},
		},
		)
	})

}
