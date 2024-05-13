package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCallArgument_AsVariable(t *testing.T) {

	check := func(t *testing.T, sf string, expect map[string][]string) {
		code := `
		a = 1
		b = 2
		c = 3
		f(a, b, c)
		`
		ssatest.CheckSyntaxFlow(t, code, sf, expect)
	}

	t.Run("test call self", func(t *testing.T) {
		check(t,
			`f() as $call`, map[string][]string{
				"call": {"Undefined-f(1,2,3)"},
			})
	})

	t.Run("test all argument ", func(t *testing.T) {
		check(t,
			`f(* as $a)`,
			map[string][]string{
				"a": {"1", "2", "3"},
			},
		)
	})

	t.Run("test first argument as variable", func(t *testing.T) {
		check(t,
			`f(* as $a,,)`,
			map[string][]string{
				"a": {"1"},
			},
		)
	})

	t.Run("first  argument ignore other", func(t *testing.T) {
		check(t,
			`f(* as $a, )`,
			map[string][]string{
				"a": {"1"},
			},
		)
	})

	t.Run("test first and second argument ", func(t *testing.T) {
		check(t, `f(* as $a, * as $b)`, map[string][]string{
			"a": {"1"},
			"b": {"2"},
		})
	})

	t.Run("test first and third argument ", func(t *testing.T) {
		check(t, `f(* as $a, ,* as $c)`, map[string][]string{
			"a": {"1"},
			"c": {"3"},
		})
	})

}
