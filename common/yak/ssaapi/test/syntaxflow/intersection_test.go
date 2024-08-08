package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestIntersection(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 1

		b1 = a 
		b2 = 2
		`, `
		a as $a // 1 
		b* as $b // 1, 2
		$b & $a as $target // 1
		`, map[string][]string{
			"target": {"1"},
		})
	})

}
