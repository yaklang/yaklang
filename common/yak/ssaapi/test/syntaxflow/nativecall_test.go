package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestGetPredecessors(t *testing.T) {
	t.Run("test normal predecessors", func(t *testing.T) {
		code := `
a = 111
a.b = 222 
	`

		ssatest.CheckSyntaxFlow(t, code, `
a.b as $b 
$b<getPredecessors> as $ret
	`, map[string][]string{
			"b":   {"222"},
			"ret": {"111"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))

	})

	t.Run("test empty predecessors", func(t *testing.T) {

		code := `
a = 11
	`

		ssatest.CheckSyntaxFlow(t, code, `
a as $a
$a<getPredecessors> as $ret
	`, map[string][]string{
			"a":   {"11"},
			"ret": {},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("test dataflow predecessors", func(t *testing.T) {

		code := `
func foo(x) {
	return x + 1
}
a = 2
b = foo(a)
	`

		ssatest.CheckSyntaxFlow(t, code, `
b #-> as $top_b
$top_b<getPredecessors> as $b
	`, map[string][]string{
			"top_b": {"1", "2"},
			"b":     {"Function-foo(2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
func TestGetRoot(t *testing.T) {
	code := `a.b().c.d();`
	ssatest.CheckSyntaxFlow(t, code, `.d<root> as $sink`, map[string][]string{
		"sink": {"Undefined-a"},
	})
}
